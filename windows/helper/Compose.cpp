#include "Compose.h"

#include <ppl.h>

#include <algorithm>
#include <cmath>
#include <cstring>


namespace helper {


void compose(const Image& image, int rotation, bool mirror, std::vector<std::uint8_t>* out)
{
    out->assign((size_t)OUT_WIDTH * OUT_HEIGHT * 3, 0);      // black, the bars stay this
    const int w = image.width, h = image.height;
    if (w <= 0 || h <= 0 || image.bgra.size() < (size_t)w * h * 4) return;

    const int turns = rotation & 3;
    const int uw = (turns & 1) ? h : w;     // the upright picture
    const int uh = (turns & 1) ? w : h;
    const double scale = (std::min)((double)OUT_WIDTH / uw, (double)OUT_HEIGHT / uh);
    const int dw = (std::max)(1, (std::min)(OUT_WIDTH, (int)std::floor(uw * scale + 0.5)));
    const int dh = (std::max)(1, (std::min)(OUT_HEIGHT, (int)std::floor(uh * scale + 0.5)));
    const int ox = (OUT_WIDTH - dw) / 2;
    const int oy = (OUT_HEIGHT - dh) / 2;

    const std::uint8_t* src = image.bgra.data();
    std::uint8_t* dst = out->data();
    const float step_u = (float)uw / dw;
    const float step_v = (float)uh / dh;

    concurrency::parallel_for(0, dh, [&](int row) {
        const float v = (row + 0.5f) * step_v - 0.5f;
        std::uint8_t* o = dst + ((size_t)(oy + row) * OUT_WIDTH + ox) * 3;
        for (int col = 0; col < dw; col++, o += 3)
        {
            float u = (col + 0.5f) * step_u - 0.5f;
            if (mirror) u = (float)(uw - 1) - u;
            // Where this pixel of the upright picture is in the decoded one: the turn clockwise, undone.
            float sx, sy;
            switch (turns)
            {
            case 1:  sx = v;                sy = (float)(h - 1) - u;    break;
            case 2:  sx = (float)(w - 1) - u; sy = (float)(h - 1) - v;  break;
            case 3:  sx = (float)(w - 1) - v; sy = u;                   break;
            default: sx = u;                sy = v;                     break;
            }
            sx = (std::min)((std::max)(sx, 0.0f), (float)(w - 1));
            sy = (std::min)((std::max)(sy, 0.0f), (float)(h - 1));
            const int x0 = (int)sx, y0 = (int)sy;
            const int x1 = (std::min)(x0 + 1, w - 1), y1 = (std::min)(y0 + 1, h - 1);
            const int fx = (int)((sx - x0) * 256.0f), fy = (int)((sy - y0) * 256.0f);
            const std::uint8_t* p00 = src + ((size_t)y0 * w + x0) * 4;
            const std::uint8_t* p10 = src + ((size_t)y0 * w + x1) * 4;
            const std::uint8_t* p01 = src + ((size_t)y1 * w + x0) * 4;
            const std::uint8_t* p11 = src + ((size_t)y1 * w + x1) * 4;
            for (int c = 0; c < 3; c++)
            {
                int top = p00[c] * (256 - fx) + p10[c] * fx;
                int bottom = p01[c] * (256 - fx) + p11[c] * fx;
                o[c] = (std::uint8_t)((top * (256 - fy) + bottom * fy + 32768) >> 16);
            }
        }
    });
}


} //namespace helper
