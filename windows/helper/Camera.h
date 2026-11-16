// The camera's sender: softcam's sender API (windows/dist/softcam.dll next to the helper), behind a lock, because
// scDeleteCamera while scSendFrame runs is not safe.
#pragma once

#include <atomic>
#include <cstdint>
#include <mutex>


namespace helper {


class CameraSender
{
 public:
    ~CameraSender() { release(); }

    /// Sends one frame (OUT_WIDTH x OUT_HEIGHT, BGR, top row first). The sender is made on the first frame, and made
    /// again after release(). False when it could not be made: the filter keeps a frame buffer of the last sender until
    /// it notices that this one is gone, and scCreateCamera fails for that moment; the next frame tries again.
    bool write(const std::uint8_t* bgr);

    /// Lets the sender go (scDeleteCamera): the filter shows its slate. Does nothing without a sender.
    void release();

    bool active() const { return m_active.load(); }
    long long created() const { return m_created.load(); }
    long long createFailures() const { return m_failures.load(); }

 private:
    std::mutex              m_mutex;
    void*                   m_camera = nullptr;     // softcam's scCamera
    std::atomic<bool>       m_active{false};
    std::atomic<long long>  m_created{0};
    std::atomic<long long>  m_failures{0};
};


} //namespace helper
