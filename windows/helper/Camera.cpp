#include "Camera.h"

#include <softcam/softcam.h>

#include "Types.h"

#pragma comment(lib, "softcam.lib")


namespace helper {


bool CameraSender::write(const std::uint8_t* bgr)
{
    std::lock_guard<std::mutex> lock(m_mutex);
    if (!m_camera)
    {
        m_camera = scCreateCamera(OUT_WIDTH, OUT_HEIGHT, OUT_FRAMERATE);
        if (!m_camera)
        {
            m_failures++;
            return false;
        }
        m_created++;
        m_active = true;
    }
    scSendFrame(static_cast<scCamera>(m_camera), bgr);      // sleeps to keep to the frame rate
    return true;
}

void CameraSender::release()
{
    std::lock_guard<std::mutex> lock(m_mutex);
    if (m_camera)
    {
        scDeleteCamera(static_cast<scCamera>(m_camera));
        m_camera = nullptr;
        m_active = false;
    }
}


} //namespace helper
