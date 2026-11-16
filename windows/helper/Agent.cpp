#include "Agent.h"

#include <windows.h>
#include <winhttp.h>

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <vector>

#pragma comment(lib, "winhttp.lib")


namespace helper {

namespace {

constexpr std::size_t RECORD_HEADER = 18;
/// The largest payload accepted: a bigger "record" is not one of the agent's (a phone sends at most maxFrameBytes).
constexpr std::uint32_t MAX_PAYLOAD = 16u * 1024u * 1024u;

std::wstring widen(const std::string& s)
{
    if (s.empty()) return std::wstring();
    int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), nullptr, 0);
    std::wstring w((size_t)n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), &w[0], n);
    return w;
}

bool readFile(const std::wstring& path, std::string* out)
{
    HANDLE f = CreateFileW(path.c_str(), GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, nullptr,
                           OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (f == INVALID_HANDLE_VALUE) return false;
    LARGE_INTEGER size = {};
    bool ok = GetFileSizeEx(f, &size) && size.QuadPart < 65536;
    if (ok)
    {
        out->assign((size_t)size.QuadPart, '\0');
        DWORD got = 0;
        ok = ReadFile(f, &(*out)[0], (DWORD)out->size(), &got, nullptr) && got == out->size();
    }
    CloseHandle(f);
    return ok;
}

/// The value after `"key":` in a flat JSON object: the text of a number or of a string without escapes. control.json is
/// written by the agent (Go's encoder), so this is all it takes; anything unexpected is "not there".
bool findValue(const std::string& json, const char* key, std::string* value, bool* is_string)
{
    std::string pattern = std::string("\"") + key + "\"";
    size_t at = json.find(pattern);
    if (at == std::string::npos) return false;
    at = json.find(':', at + pattern.size());
    if (at == std::string::npos) return false;
    at++;
    while (at < json.size() && (json[at] == ' ' || json[at] == '\t' || json[at] == '\r' || json[at] == '\n')) at++;
    if (at >= json.size()) return false;
    if (json[at] == '"')
    {
        size_t end = json.find('"', at + 1);
        if (end == std::string::npos) return false;
        std::string text = json.substr(at + 1, end - at - 1);
        if (text.find('\\') != std::string::npos) return false;
        *value = text;
        *is_string = true;
        return true;
    }
    size_t end = at;
    while (end < json.size() && ((json[end] >= '0' && json[end] <= '9') || json[end] == '-')) end++;
    if (end == at) return false;
    *value = json.substr(at, end - at);
    *is_string = false;
    return true;
}

std::uint32_t be32(const std::uint8_t* p) { return (std::uint32_t)p[0] << 24 | (std::uint32_t)p[1] << 16 | (std::uint32_t)p[2] << 8 | p[3]; }
int be16(const std::uint8_t* p) { return (int)p[0] << 8 | p[1]; }

std::string jsonEscape(const std::string& s)
{
    std::string out;
    for (unsigned char c : s)
    {
        if (c == '"' || c == '\\') { out += '\\'; out += (char)c; }
        else if (c < 0x20) { char b[8]; std::snprintf(b, sizeof b, "\\u%04x", c); out += b; }
        else out += (char)c;
    }
    return out;
}

} //namespace


bool Control::read(const std::wstring& dir, Control* out)
{
    std::string text;
    if (!readFile(dir + L"\\control.json", &text)) return false;
    std::string port, token; bool port_is_string = false, token_is_string = false;
    if (!findValue(text, "port", &port, &port_is_string) || port_is_string) return false;
    if (!findValue(text, "token", &token, &token_is_string) || !token_is_string || token.empty()) return false;
    int p = std::atoi(port.c_str());
    if (p <= 0 || p > 65535) return false;
    out->port = p;
    out->token = token;
    return true;
}


void AgentLink::start()
{
    if (m_thread.joinable()) return;
    m_quit = false;
    m_thread = std::thread([this] { run(); });
}

void AgentLink::stop()
{
    m_quit = true;
    {
        std::lock_guard<std::mutex> lock(m_mutex);
        if (m_request)
        {
            // Closing the request wakes a read that is waiting for the next frame.
            WinHttpCloseHandle((HINTERNET)m_request);
            m_request = nullptr;
        }
    }
    if (m_thread.joinable()) m_thread.join();
}

bool AgentLink::control(Control* out) const
{
    std::lock_guard<std::mutex> lock(m_mutex);
    if (!m_have_control) return false;
    *out = m_control;
    return true;
}

void AgentLink::sleepOrQuit(int ms)
{
    for (int waited = 0; waited < ms && !m_quit; waited += 50) Sleep(50);
}

void AgentLink::run()
{
    while (!m_quit)
    {
        Control control;
        if (!Control::read(m_dir, &control))
        {
            m_connected = false;
            { std::lock_guard<std::mutex> lock(m_mutex); m_have_control = false; }
            sleepOrQuit(1000);
            continue;
        }
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            m_control = control;
            m_have_control = true;
        }
        try { stream(control); } catch (...) { m_bad++; }
        m_connected = false;
        sleepOrQuit(1000);
    }
}

/// One connection: returns when it ends (the agent went away, a bad record, stop()).
bool AgentLink::stream(const Control& control)
{
    HINTERNET session = WinHttpOpen(L"phoneset-camera", WINHTTP_ACCESS_TYPE_NO_PROXY, WINHTTP_NO_PROXY_NAME,
                                    WINHTTP_NO_PROXY_BYPASS, 0);
    if (!session) return false;
    // Resolve, connect, send: quick. Receive: never, the stream is quiet for as long as no phone sends.
    WinHttpSetTimeouts(session, 3000, 3000, 3000, 0);
    HINTERNET connection = WinHttpConnect(session, L"127.0.0.1", (INTERNET_PORT)control.port, 0);
    HINTERNET request = nullptr;
    bool ok = false;
    if (connection)
    {
        std::wstring path = L"/video.frames?t=" + widen(control.token);
        request = WinHttpOpenRequest(connection, L"GET", path.c_str(), nullptr, WINHTTP_NO_REFERER,
                                     WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
    }
    if (request)
    {
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            if (m_quit) { WinHttpCloseHandle(request); request = nullptr; }
            else m_request = request;
        }
    }
    if (request &&
        WinHttpSendRequest(request, WINHTTP_NO_ADDITIONAL_HEADERS, 0, WINHTTP_NO_REQUEST_DATA, 0, 0, 0) &&
        WinHttpReceiveResponse(request, nullptr))
    {
        DWORD status = 0, size = sizeof status;
        WinHttpQueryHeaders(request, WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER, WINHTTP_HEADER_NAME_BY_INDEX,
                            &status, &size, WINHTTP_NO_HEADER_INDEX);
        if (status == 200)
        {
            m_connected = true;
            ok = true;
            std::vector<std::uint8_t> buffer;
            std::size_t pos = 0;
            std::vector<std::uint8_t> chunk(64 * 1024);
            while (!m_quit)
            {
                // Ask what is there first and read only that: WinHttpReadData waits until its whole buffer is full, which at
                // the speed of this stream (some tens of KB a second) would hold a picture back for seconds.
                DWORD available = 0;
                if (!WinHttpQueryDataAvailable(request, &available) || available == 0) break;
                DWORD got = 0;
                if (!WinHttpReadData(request, chunk.data(), (std::min)(available, (DWORD)chunk.size()), &got) || got == 0) break;
                buffer.insert(buffer.end(), chunk.begin(), chunk.begin() + got);
                bool broken = false;
                while (buffer.size() - pos >= RECORD_HEADER)
                {
                    const std::uint8_t* h = buffer.data() + pos;
                    std::uint32_t length = be32(h + 14);
                    if (length > MAX_PAYLOAD) { broken = true; break; }
                    if (buffer.size() - pos < RECORD_HEADER + length) break;
                    Record r;
                    r.seq = be32(h);
                    r.captured_ms = be32(h + 4);
                    r.codec = h[8];
                    r.key = (h[9] & 1) != 0;
                    r.rotation = (h[9] >> 1) & 3;
                    r.mirror = (h[9] & 8) != 0;
                    r.width = be16(h + 10);
                    r.height = be16(h + 12);
                    pos += RECORD_HEADER;
                    if (r.codec == Record::JPEG || r.codec == Record::H264)
                    {
                        r.payload.assign(buffer.begin() + pos, buffer.begin() + pos + length);
                        m_records++;
                        if (on_record) on_record(std::move(r));
                    }
                    else
                    {
                        m_bad++;    // a codec the helper does not decode (hevc): skipped, the framing is still good
                    }
                    pos += length;
                }
                if (broken) { m_bad++; break; }
                if (pos == buffer.size()) { buffer.clear(); pos = 0; }
                else if (pos > (1u << 20)) { buffer.erase(buffer.begin(), buffer.begin() + pos); pos = 0; }
            }
        }
    }
    m_connected = false;
    {
        std::lock_guard<std::mutex> lock(m_mutex);
        if (m_request) { WinHttpCloseHandle((HINTERNET)m_request); m_request = nullptr; }   // else stop() closed it
        else request = nullptr;
    }
    if (connection) WinHttpCloseHandle(connection);
    WinHttpCloseHandle(session);
    return ok;
}


void report(const Control& control, const std::string& line, bool flowing)
{
    HINTERNET session = WinHttpOpen(L"phoneset-camera", WINHTTP_ACCESS_TYPE_NO_PROXY, WINHTTP_NO_PROXY_NAME,
                                    WINHTTP_NO_PROXY_BYPASS, 0);
    if (!session) return;
    WinHttpSetTimeouts(session, 500, 500, 500, 1000);
    HINTERNET connection = WinHttpConnect(session, L"127.0.0.1", (INTERNET_PORT)control.port, 0);
    if (connection)
    {
        std::wstring path = L"/api/camera/report?t=" + widen(control.token);
        HINTERNET request = WinHttpOpenRequest(connection, L"POST", path.c_str(), nullptr, WINHTTP_NO_REFERER,
                                               WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
        if (request)
        {
            std::string body = std::string("{\"line\":\"") + jsonEscape(line) + "\",\"flowing\":" + (flowing ? "true" : "false") + "}";
            static const wchar_t headers[] = L"Content-Type: application/json";
            if (WinHttpSendRequest(request, headers, (DWORD)-1, (LPVOID)body.data(), (DWORD)body.size(), (DWORD)body.size(), 0))
            {
                WinHttpReceiveResponse(request, nullptr);
            }
            WinHttpCloseHandle(request);
        }
        WinHttpCloseHandle(connection);
    }
    WinHttpCloseHandle(session);
}


} //namespace helper
