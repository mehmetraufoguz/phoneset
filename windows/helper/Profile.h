// Where the helper's time goes: wall time per stage of a picture's way (QueryPerformanceCounter around each), and the CPU
// time of every thread of the process (GetThreadTimes), so that a thread that burns CPU without doing a stage's work shows
// up. Written to the log once a second only with --profile; the stages are always counted (a few nanoseconds each).
#pragma once

#include <windows.h>

#include <string>


namespace helper {
namespace profile {


enum Stage { Decode, Convert, Compose, Copy, Send, StageCount };

void add(Stage stage, long long ticks);

struct Scope
{
    Stage stage;
    LARGE_INTEGER start;
    explicit Scope(Stage s) : stage(s) { QueryPerformanceCounter(&start); }
    ~Scope()
    {
        LARGE_INTEGER end;
        QueryPerformanceCounter(&end);
        add(stage, end.QuadPart - start.QuadPart);
    }
};

/// Names the calling thread (shows in the report).
void nameThisThread(const wchar_t* name);

/// One line: the wall milliseconds each stage took since the last call, then the threads that used CPU since the last call,
/// each as "name(tid)=cpu ms" with the module its start address is in when it has no name.
std::string report();


} //namespace profile
} //namespace helper
