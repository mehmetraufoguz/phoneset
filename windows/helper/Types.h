// What the helper passes around: a record of the agent's /video.frames stream and a decoded picture.
#pragma once

#include <cstdint>
#include <vector>


namespace helper {


/// One record of the agent's `/video.frames` stream (docs/protocol.md, section 9).
struct Record
{
    static constexpr std::uint8_t JPEG = 1;
    static constexpr std::uint8_t H264 = 2;

    std::uint32_t   seq = 0;
    std::uint32_t   captured_ms = 0;
    std::uint8_t    codec = 0;
    bool            key = false;
    int             rotation = 0;       // quarter turns clockwise that make the decoded picture upright
    bool            mirror = false;     // then mirror it horizontally
    int             width = 0;
    int             height = 0;
    std::vector<std::uint8_t> payload;
};

/// A decoded picture: 32-bit BGRA, top row first, `width * 4` bytes a row.
struct Image
{
    int             width = 0;
    int             height = 0;
    std::vector<std::uint8_t> bgra;
};

/// The size of the camera's picture: what the filter offers when no sender is there (windows/filter), so that a sender
/// of this size and the slate are the same type to an app.
constexpr int OUT_WIDTH = 1280;
constexpr int OUT_HEIGHT = 720;
constexpr float OUT_FRAMERATE = 30.0f;


} //namespace helper
