#include "Profile.h"

#include <tlhelp32.h>
#include <psapi.h>

#include <algorithm>
#include <atomic>
#include <cstdio>
#include <map>
#include <vector>


namespace helper {
namespace profile {

namespace {

std::atomic<long long> g_ticks[StageCount];

const char* const STAGE_NAMES[StageCount] = { "decode", "convert", "compose", "copy", "send" };

long long toUnits(const FILETIME& t) { return ((long long)t.dwHighDateTime << 32) | t.dwLowDateTime; }    // 100 ns

struct ThreadSeen { long long cpu = 0; };
std::map<DWORD, ThreadSeen> g_seen;

typedef LONG (NTAPI* NtQueryInformationThreadFn)(HANDLE, int, PVOID, ULONG, PULONG);

std::string moduleOf(HANDLE thread)
{
    static NtQueryInformationThreadFn query = (NtQueryInformationThreadFn)GetProcAddress(GetModuleHandleW(L"ntdll.dll"), "NtQueryInformationThread");
    PVOID address = nullptr;
    // 9 = ThreadQuerySetWin32StartAddress; needs THREAD_QUERY_INFORMATION
    if (!query || query(thread, 9, &address, sizeof address, nullptr) != 0 || !address) return "?";
    HMODULE module = nullptr;
    if (!GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS | GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                            (LPCWSTR)address, &module))
        return "?";
    wchar_t path[MAX_PATH] = {};
    GetModuleFileNameW(module, path, MAX_PATH);
    const wchar_t* name = path;
    for (const wchar_t* c = path; *c; c++) if (*c == L'\\') name = c + 1;
    std::string text;
    for (const wchar_t* c = name; *c; c++) text += (char)(*c < 128 ? *c : L'?');
    return text;
}

}   // namespace


void add(Stage stage, long long ticks) { g_ticks[stage] += ticks; }

void nameThisThread(const wchar_t* name)
{
    typedef HRESULT (WINAPI* SetThreadDescriptionFn)(HANDLE, PCWSTR);
    static SetThreadDescriptionFn set = (SetThreadDescriptionFn)GetProcAddress(GetModuleHandleW(L"kernelbase.dll"), "SetThreadDescription");
    if (set) set(GetCurrentThread(), name);
}

std::string report()
{
    LARGE_INTEGER freq;
    QueryPerformanceFrequency(&freq);
    std::string text = "profile wall ms: ";
    char piece[96];
    for (int i = 0; i < StageCount; i++)
    {
        long long ticks = g_ticks[i].exchange(0);
        std::snprintf(piece, sizeof piece, "%s=%.0f ", STAGE_NAMES[i], 1000.0 * (double)ticks / (double)freq.QuadPart);
        text += piece;
    }

    typedef HRESULT (WINAPI* GetThreadDescriptionFn)(HANDLE, PWSTR*);
    static GetThreadDescriptionFn describe = (GetThreadDescriptionFn)GetProcAddress(GetModuleHandleW(L"kernelbase.dll"), "GetThreadDescription");

    std::vector<std::pair<long long, std::string>> used;
    long long total = 0;
    int threads = 0;
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, 0);
    if (snapshot != INVALID_HANDLE_VALUE)
    {
        THREADENTRY32 entry = {};
        entry.dwSize = sizeof entry;
        const DWORD pid = GetCurrentProcessId();
        for (BOOL ok = Thread32First(snapshot, &entry); ok; ok = Thread32Next(snapshot, &entry))
        {
            if (entry.th32OwnerProcessID != pid) continue;
            threads++;
            HANDLE thread = OpenThread(THREAD_QUERY_INFORMATION, FALSE, entry.th32ThreadID);
            if (!thread) continue;
            FILETIME created, exited, kernel, user;
            if (GetThreadTimes(thread, &created, &exited, &kernel, &user))
            {
                const long long cpu = toUnits(kernel) + toUnits(user);
                ThreadSeen& seen = g_seen[entry.th32ThreadID];
                const long long delta = cpu - seen.cpu;
                seen.cpu = cpu;
                total += delta;
                if (delta >= 20000)      // 2 ms or more
                {
                    std::string name;
                    PWSTR description = nullptr;
                    if (describe && SUCCEEDED(describe(thread, &description)) && description && *description)
                    {
                        for (const wchar_t* c = description; *c; c++) name += (char)(*c < 128 ? *c : L'?');
                    }
                    if (description) LocalFree(description);
                    if (name.empty()) name = moduleOf(thread);
                    std::snprintf(piece, sizeof piece, "%s(%lu)=%lld", name.c_str(), (unsigned long)entry.th32ThreadID, delta / 10000);
                    used.emplace_back(delta, piece);
                }
            }
            CloseHandle(thread);
        }
        CloseHandle(snapshot);
    }
    std::sort(used.begin(), used.end(), [](const auto& a, const auto& b) { return a.first > b.first; });
    std::snprintf(piece, sizeof piece, "| threads=%d cpu total ms=%lld | ", threads, total / 10000);
    text += piece;
    for (size_t i = 0; i < used.size() && i < 14; i++) text += used[i].second + " ";
    return text;
}


} //namespace profile
} //namespace helper
