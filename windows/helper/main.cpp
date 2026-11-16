// phoneset-camera.exe: the Windows twin of the macOS companion app (macos/App). It carries the phone's picture from the
// phoneset program to "phoneset Camera": reads the program's /video.frames, decodes, turns the picture upright, fits it
// into 1280 x 720 and writes it with softcam's sender API. When no picture comes for a while it lets the sender go, so
// that the camera (windows/filter) shows its slate instead of a frozen picture.
//
//   phoneset-camera.exe [--config-dir <folder of the phoneset program's control.json>]
//   phoneset-camera.exe --quit      asks the running helper to quit (it signals the event below and waits for it)
//
// No window, no console. One instance per user session (a named mutex). It never quits by itself: not when the program
// restarts, not when the phone goes away. Once a second it tells the program what is going on (POST /api/camera/report)
// and, when something happens, writes a line to camera-helper.log in the config folder (never the token).
#include <windows.h>
#include <shellapi.h>
#include <shlobj.h>
#include <mfapi.h>
#include <objbase.h>

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstdio>
#include <deque>
#include <mutex>
#include <string>
#include <thread>

#include "Agent.h"
#include "Camera.h"
#include "Compose.h"
#include "Decode.h"
#include "Types.h"

using namespace helper;

namespace {

const wchar_t MUTEX_NAME[] = L"Local\\phoneset-camera-helper";
const wchar_t QUIT_EVENT_NAME[] = L"Local\\phoneset-camera-helper-quit";

/// How many records may be waiting or being decoded before the decoder skips ahead to the newest key frame, and how many
/// before everything waiting is dropped. (The macOS helper's Bridge.swift allows 3 in flight; here a read of the stream can
/// hold a burst of records, which a limit of 3 would throw away, and every drop costs a wait for the next key frame.)
const int MAX_IN_FLIGHT = 8;
const int HARD_LIMIT = 30;
/// No picture for this long: the phone turned its camera off, or the network or the program is gone.
const long long IDLE_MS = 1500;

long long nowMs()
{
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch()).count();
}

std::wstring defaultConfigDir()
{
    PWSTR roaming = nullptr;
    std::wstring dir;
    if (SUCCEEDED(SHGetKnownFolderPath(FOLDERID_RoamingAppData, 0, nullptr, &roaming)) && roaming)
    {
        dir = std::wstring(roaming) + L"\\phoneset";
    }
    CoTaskMemFree(roaming);
    return dir;
}

/// The log: one line when something happens, kept small (the old file is kept as .1).
class Log
{
 public:
    explicit Log(std::wstring path) : m_path(std::move(path)) {}
    void line(const std::string& text)
    {
        if (m_path.empty()) return;
        WIN32_FILE_ATTRIBUTE_DATA info = {};
        if (GetFileAttributesExW(m_path.c_str(), GetFileExInfoStandard, &info) && info.nFileSizeLow > 512 * 1024)
        {
            MoveFileExW(m_path.c_str(), (m_path + L".1").c_str(), MOVEFILE_REPLACE_EXISTING);
        }
        HANDLE f = CreateFileW(m_path.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE, nullptr, OPEN_ALWAYS,
                               FILE_ATTRIBUTE_NORMAL, nullptr);
        if (f == INVALID_HANDLE_VALUE) return;
        SYSTEMTIME t;
        GetLocalTime(&t);
        char stamp[32];
        std::snprintf(stamp, sizeof stamp, "%02d:%02d:%02d ", t.wHour, t.wMinute, t.wSecond);
        std::string full = std::string(stamp) + text + "\r\n";
        DWORD written = 0;
        WriteFile(f, full.data(), (DWORD)full.size(), &written, nullptr);
        CloseHandle(f);
    }

 private:
    std::wstring m_path;
};

/// Everything the helper does between the link and the camera.
class Bridge
{
 public:
    Bridge(const std::wstring& dir, Log* log) : m_link(dir), m_log(log) {}

    void start()
    {
        m_link.on_record = [this](Record&& r) { receive(std::move(r)); };
        m_worker = std::thread([this] { work(); });
        m_sender = std::thread([this] { sendLoop(); });
        m_reporter = std::thread([this] { reportLoop(); });
        m_link.start();
    }

    void stop()
    {
        m_link.stop();
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            m_quit = true;
        }
        {
            std::lock_guard<std::mutex> lock(m_out_mutex);
            m_out_quit = true;
        }
        {
            std::lock_guard<std::mutex> lock(m_rep_mutex);
            m_rep_quit = true;
        }
        m_wake.notify_all();
        m_out_wake.notify_all();
        m_rep_wake.notify_all();
        if (m_worker.joinable()) m_worker.join();
        if (m_sender.joinable()) m_sender.join();
        if (m_reporter.joinable()) m_reporter.join();
        m_camera.release();
    }

    /// Once a second: lets the sender go when nothing came for a while, reports, logs.
    void tick()
    {
        const long long now = nowMs();
        const long long idle = now - m_last_picture.load();
        if (m_camera.active() && idle > IDLE_MS)
        {
            // The slate, not the last picture for good: the filter notices the sender is gone and shows its slate.
            m_camera.release();
            m_log->line("no picture for a while: the sender is let go, the camera shows its slate");
        }

        const long long received = m_received.exchange(0), dropped = m_dropped.exchange(0), decoded = m_decoded.exchange(0),
                        composed = m_composed.exchange(0), written = m_written.exchange(0);
        const bool agent = m_link.connected();
        const bool flowing = agent && written > 0 && idle <= IDLE_MS;
        std::string description;
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            description = m_description;
        }

        std::string text;
        if (!agent) text = "waiting for the phoneset program";
        else if (m_fail_streak >= 3)
            text = "the camera cannot be started: is another program sending to it?";
        else if (!flowing) text = "waiting for your phone to turn its camera on";
        else text = "sending your phone's picture: " + description + ", " + std::to_string(written) + " frames a second";
        // Making the sender can fail for a moment after it was let go (the filter still holds the old frame buffer): only
        // a few seconds of it in a row is a message.
        if (m_camera.active()) m_fail_streak = 0;
        else if (m_camera.createFailures() > m_failures_seen) m_fail_streak++;
        else if (received == 0) m_fail_streak = 0;
        m_failures_seen = m_camera.createFailures();

        Control control;
        if (m_link.control(&control))
        {
            // Told to the program on a thread of its own: a program that is slow to answer must not hold the beat up.
            {
                std::lock_guard<std::mutex> lock(m_rep_mutex);
                m_rep_control = control;
                m_rep_line = text;
                m_rep_flowing = flowing;
                m_rep_pending = true;
            }
            m_rep_wake.notify_one();
        }

        // A line in the log when there is something to say: pictures, or a change of state, or now and then.
        const bool changed = text != m_last_text;
        m_seconds++;
        if (received || dropped || changed || m_seconds % 60 == 0)
        {
            char counters[320];
            std::snprintf(counters, sizeof counters,
                          "agent=%d received=%lld dropped=%lld decoded=%lld composed=%lld written=%lld sender=%d created=%lld "
                          "createFailures=%lld badRecords=%lld decodeFailures=%lld busy=%lldms longest=%lldms lastError=%s",
                          agent ? 1 : 0, received, dropped, decoded, composed, written, m_camera.active() ? 1 : 0,
                          m_camera.created(), m_camera.createFailures(), m_link.bad(), m_failures_total(), m_busy_ms.exchange(0),
                          m_longest_ms.exchange(0), lastDecodeError().c_str());
            m_log->line(std::string(counters) + " | " + text);
            if (!m_decoder_note.empty() && m_decoder_note != m_last_note)
            {
                m_log->line("h264 decoder: " + m_decoder_note);
                m_last_note = m_decoder_note;
            }
        }
        m_last_text = text;
    }

 private:
    AgentLink               m_link;
    Log*                    m_log;
    CameraSender            m_camera;
    std::thread             m_worker;
    std::thread             m_sender;
    std::thread             m_reporter;

    std::mutex              m_rep_mutex;
    std::condition_variable m_rep_wake;
    Control                 m_rep_control;
    std::string             m_rep_line;
    bool                    m_rep_flowing = false;
    bool                    m_rep_pending = false;
    bool                    m_rep_quit = false;

    // The composed picture waiting to be sent. The sender thread writes it with scSendFrame, which sleeps to keep to
    // 30 frames a second: that must not hold up the decoder, or pictures pile up and the decoder keeps having to wait
    // for a key frame.
    std::mutex              m_out_mutex;
    std::condition_variable m_out_wake;
    std::vector<std::uint8_t> m_latest;
    bool                    m_fresh = false;
    bool                    m_out_quit = false;

    std::mutex              m_mutex;
    std::condition_variable m_wake;
    std::deque<Record>      m_queue;
    bool                    m_busy = false;
    bool                    m_quit = false;
    std::atomic<bool>       m_lost{true};       // the decoder waits for a key frame

    std::string             m_description;      // of the last picture, e.g. "1920x1080 H.264"
    std::string             m_decoder_note;
    std::string             m_last_note;
    std::string             m_last_text;
    long long               m_failures_seen = 0;
    int                     m_fail_streak = 0;
    int                     m_seconds = 0;
    std::atomic<long long>  m_received{0}, m_dropped{0}, m_decoded{0}, m_composed{0}, m_written{0};
    std::atomic<long long>  m_last_picture{0};
    std::atomic<long long>  m_decode_failures{0};
    std::atomic<long long>  m_busy_ms{0}, m_longest_ms{0};     // time spent on records in the last second, and the longest one

    long long m_failures_total() const { return m_decode_failures.load(); }
    std::string lastDecodeError()
    {
        std::lock_guard<std::mutex> lock(m_mutex);
        return m_last_error.empty() ? std::string("none") : m_last_error;
    }
    std::string             m_last_error;

    /// A record from the program (on the link's thread). Records can come in a burst (one read of the stream may hold a
    /// second of video), so some may wait: the decoder takes them in order and catches up (a picture takes a few
    /// milliseconds). When more than MAX_IN_FLIGHT wait, the decoder skips ahead to the newest key frame in the queue
    /// (what is in front of it is dropped; a decoder that starts at a key frame needs nothing before it). When more than
    /// HARD_LIMIT wait, everything is dropped and the decoder waits for the next key frame. Either way it never decodes a
    /// picture that depends on one that was dropped (the decoder is told, and starts at a key frame).
    void receive(Record&& record)
    {
        m_received++;
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            m_queue.push_back(std::move(record));
            const int waiting = (int)m_queue.size() + (m_busy ? 1 : 0);
            if (waiting > HARD_LIMIT)
            {
                m_dropped += (long long)m_queue.size();
                m_queue.clear();
                m_lost = true;
            }
            else if (waiting > MAX_IN_FLIGHT)
            {
                int newest_key = -1;
                for (int i = (int)m_queue.size() - 1; i > 0; i--)
                {
                    if (m_queue[(size_t)i].key) { newest_key = i; break; }
                }
                if (newest_key > 0)
                {
                    m_dropped += newest_key;
                    m_queue.erase(m_queue.begin(), m_queue.begin() + newest_key);
                    m_lost = true;      // the decoder starts again at this key frame
                }
            }
        }
        m_wake.notify_one();
    }

    void reportLoop()
    {
        for (;;)
        {
            Control control;
            std::string line;
            bool flowing;
            {
                std::unique_lock<std::mutex> lock(m_rep_mutex);
                m_rep_wake.wait(lock, [this] { return m_rep_quit || m_rep_pending; });
                if (m_rep_quit) break;
                control = m_rep_control;
                line = m_rep_line;
                flowing = m_rep_flowing;
                m_rep_pending = false;
            }
            report(control, line, flowing);
        }
    }

    void sendLoop()
    {
        std::vector<std::uint8_t> frame;
        for (;;)
        {
            {
                std::unique_lock<std::mutex> lock(m_out_mutex);
                m_out_wake.wait(lock, [this] { return m_out_quit || m_fresh; });
                if (m_out_quit) break;
                frame.swap(m_latest);
                m_fresh = false;
            }
            if (frame.size() == (size_t)OUT_WIDTH * OUT_HEIGHT * 3 && m_camera.write(frame.data()))
            {
                m_written++;
                m_last_picture = nowMs();
            }
        }
    }

    void work()
    {
        CoInitializeEx(nullptr, COINIT_MULTITHREADED);
        Decoder decoder;
        Image image;
        std::vector<std::uint8_t> frame;
        for (;;)
        {
            Record record;
            {
                std::unique_lock<std::mutex> lock(m_mutex);
                m_wake.wait(lock, [this] { return m_quit || !m_queue.empty(); });
                if (m_quit) break;
                record = std::move(m_queue.front());
                m_queue.pop_front();
                m_busy = true;
            }
            if (m_lost.exchange(false)) decoder.lostFrame();
            const long long started = nowMs();
            struct Timer
            {
                std::atomic<long long>* total; std::atomic<long long>* longest; long long start;
                ~Timer()
                {
                    long long took = nowMs() - start;
                    *total += took;
                    long long seen = longest->load();
                    while (took > seen && !longest->compare_exchange_weak(seen, took)) {}
                }
            } timer{&m_busy_ms, &m_longest_ms, started};
            try
            {
                if (decoder.decode(record, &image))
                {
                    m_decoded++;
                    compose(image, record.rotation, record.mirror, &frame);
                    m_composed++;
                    {
                        // Hand the picture to the sender thread; one that was not sent yet is replaced (30 a second at most).
                        std::lock_guard<std::mutex> lock(m_out_mutex);
                        m_latest.assign(frame.begin(), frame.end());
                        m_fresh = true;
                    }
                    m_out_wake.notify_one();
                    char size[64];
                    std::snprintf(size, sizeof size, "%dx%d %s", record.rotation & 1 ? record.height : record.width,
                                  record.rotation & 1 ? record.width : record.height, record.codec == Record::H264 ? "H.264" : "JPEG");
                    std::lock_guard<std::mutex> lock(m_mutex);
                    m_description = size;
                    if (record.codec == Record::H264) m_decoder_note = decoder.h264Description();
                }
                m_decode_failures = decoder.failures();
                if (!decoder.lastError().empty()) { std::lock_guard<std::mutex> lock(m_mutex); m_last_error = decoder.lastError(); }
            }
            catch (...)
            {
                // Whatever went wrong with this record, the next one is a new try.
                m_decode_failures++;
                decoder.lostFrame();
            }
            std::lock_guard<std::mutex> lock(m_mutex);
            m_busy = false;
        }
        CoUninitialize();
    }
};

int quitRunningHelper()
{
    HANDLE quit = OpenEventW(EVENT_MODIFY_STATE, FALSE, QUIT_EVENT_NAME);
    if (!quit) return 1;                    // not running
    SetEvent(quit);
    CloseHandle(quit);
    // Wait for it to go: the mutex disappears with the process.
    for (int i = 0; i < 100; i++)
    {
        HANDLE mutex = OpenMutexW(SYNCHRONIZE, FALSE, MUTEX_NAME);
        if (!mutex) return 0;
        CloseHandle(mutex);
        Sleep(50);
    }
    return 2;                               // still there after 5 seconds
}

} //namespace


int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int)
{
    std::wstring dir = defaultConfigDir();
    bool quit = false;
    int argc = 0;
    LPWSTR* argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    for (int i = 1; argv && i < argc; i++)
    {
        std::wstring arg = argv[i];
        if (arg == L"--quit") quit = true;
        else if (arg == L"--config-dir" && i + 1 < argc) dir = argv[++i];
    }
    if (argv) LocalFree(argv);

    if (quit) return quitRunningHelper();

    HANDLE mutex = CreateMutexW(nullptr, TRUE, MUTEX_NAME);
    if (!mutex || GetLastError() == ERROR_ALREADY_EXISTS) return 3;     // one instance only
    HANDLE quit_event = CreateEventW(nullptr, TRUE, FALSE, QUIT_EVENT_NAME);
    if (!quit_event) return 4;

    CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    MFStartup(MF_VERSION);
    {
        Log log(dir.empty() ? std::wstring() : dir + L"\\camera-helper.log");
        log.line("started");
        Bridge bridge(dir, &log);
        bridge.start();
        // The beat of the program: once a second until asked to quit.
        while (WaitForSingleObject(quit_event, 1000) == WAIT_TIMEOUT) bridge.tick();
        log.line("quitting");
        bridge.stop();
    }
    MFShutdown();
    CoUninitialize();
    CloseHandle(quit_event);
    ReleaseMutex(mutex);
    CloseHandle(mutex);
    return 0;
}
