// The picture "phoneset Camera" shows when no sender is there: the same slate as the macOS camera
// (macos/Extension/DeviceSource.swift, drawSlate). Added to softcam's filter by windows/build.ps1.
#pragma once

#include <cstddef>
#include <cstdint>
#include <vector>


namespace softcam {


class Slate
{
 public:
    /// Draws the static part (background and the two lines of text) once, with GDI.
    Slate(int width, int height);

    /// Writes slate number `frame_number` into a bottom-up 24-bit DIB with rows padded to 4 bytes (the layout the
    /// filter's media type has). Only the bar at the bottom changes from frame to frame: it moves 12 pixels to the right
    /// each time, so that a frozen picture is easy to tell from a live one.
    void render(std::uint8_t* dib, std::size_t dib_size, std::uint64_t frame_number) const;

    int width() const { return m_width; }
    int height() const { return m_height; }

 private:
    int                         m_width;
    int                         m_height;
    std::size_t                 m_stride;
    std::vector<std::uint8_t>   m_base;
};


} //namespace softcam
