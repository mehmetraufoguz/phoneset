// The phoneset program ("the agent") from the helper's side: finding it, reading its frames, reporting to it.
#pragma once

#include <atomic>
#include <functional>
#include <mutex>
#include <string>
#include <thread>

#include "Types.h"


namespace helper {


/// Where the agent's port and token are: control.json in its config folder, written while it runs.
struct Control
{
    int         port = 0;
    std::string token;      // never logged, never printed

    /// Reads `<dir>\control.json`; false when it is not there or is not what is expected.
    static bool read(const std::wstring& dir, Control* out);
};

/// Reads `/video.frames` with WinHTTP and hands each record over. It runs a thread of its own; when the agent is not
/// there or the stream ends it tries again every second. A record that is not what it says it is ends the connection
/// (and a new one is made); it never throws.
class AgentLink
{
 public:
    explicit AgentLink(std::wstring config_dir) : m_dir(std::move(config_dir)) {}
    ~AgentLink() { stop(); }

    /// Called on the link's thread for each record.
    std::function<void(Record&&)> on_record;

    void start();
    void stop();

    /// True while a connection to the agent's stream is open.
    bool connected() const { return m_connected.load(); }
    /// Records read, bad records seen (since the start).
    long long records() const { return m_records.load(); }
    long long bad() const { return m_bad.load(); }

    /// The agent's control information as read last, if any (for reporting).
    bool control(Control* out) const;

 private:
    std::wstring            m_dir;
    std::thread             m_thread;
    std::atomic<bool>       m_quit{false};
    std::atomic<bool>       m_connected{false};
    std::atomic<long long>  m_records{0};
    std::atomic<long long>  m_bad{0};
    mutable std::mutex      m_mutex;
    Control                 m_control;
    bool                    m_have_control = false;
    void*                   m_request = nullptr;    // the HINTERNET of the request being read, closed by stop()

    void run();
    bool stream(const Control& control);
    void sleepOrQuit(int ms);
};

/// Tells the agent what the camera is doing: `POST /api/camera/report`, so that `phoneset status`, `phoneset camera`
/// and the tray can say it. Best effort, short timeouts.
void report(const Control& control, const std::string& line, bool flowing);


} //namespace helper
