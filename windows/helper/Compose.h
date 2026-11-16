// Turns a decoded picture into one frame of the camera: upright (the record's rotation and mirror), as large as it fits
// in OUT_WIDTH x OUT_HEIGHT (no cropping), black bars at the sides or on top where the shape differs. Like the macOS
// helper's Compositor.swift.
#pragma once

#include <cstdint>
#include <vector>

#include "Types.h"


namespace helper {


/// `out` becomes OUT_WIDTH x OUT_HEIGHT, 24-bit BGR, top row first, no padding: what softcam's scSendFrame takes.
/// `rotation` is quarter turns clockwise that make the picture upright; `mirror` flips the upright picture horizontally.
void compose(const Image& image, int rotation, bool mirror, std::vector<std::uint8_t>* out);


} //namespace helper
