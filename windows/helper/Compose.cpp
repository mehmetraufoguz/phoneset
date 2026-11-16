#include "Compose.h"

#include "Profile.h"

#include <algorithm>
#include <cmath>
#include <cstring>


namespace helper {

namespace {

/// Where an output column or row takes its pixels from along one axis of the decoded picture: the two neighbours and the
/// weight (0..255) of the second one. Worked out once per picture, so the loop over the pixels does only integer work.
struct Tap { int i0, i1, f; };

inline int clamp255(int v) { return v < 0 ? 0 : (v > 255 ? 255 : v); }

/// `count` positions of the upright picture, `step` source pixels apart, along an axis of `axisN` decoded pixels. `mirror`
/// flips the upright picture first (over `uSize` pixels), `flip` is the turn that runs the axis backwards.
void makeTable(std::vector<Tap>* table, int count, float step, bool mirror, int uSize, bool flip, int axisN)
{
    table->resize((size_t)count);
    for (int p = 0; p < count; p++)
    {
        float c = (p + 0.5f) * step - 0.5f;
        if (mirror) c = (float)(uSize - 1) - c;
        if (flip) c = (float)(axisN - 1) - c;
        c = (std::min)((std::max)(c, 0.0f), (float)(axisN - 1));
        Tap& t = (*table)[(size_t)p];
        t.i0 = (int)c;
        t.i1 = (std::min)(t.i0 + 1, axisN - 1);
        t.f = (int)((c - t.i0) * 256.0f);
    }
}

}   // namespace


void compose(const Image& image, int rotation, bool mirror, std::vector<std::uint8_t>* out)
{
    out->assign((size_t)OUT_WIDTH * OUT_HEIGHT * 3, 0);      // black, the bars stay this
    const int w = image.width, h = image.height;
    if (w <= 0 || h <= 0) return;
    if (image.nv12 ? image.yuv.size() < (size_t)w * h + (size_t)image.chroma_stride * ((h + 1) / 2) : image.bgra.size() < (size_t)w * h * 4) return;

    const int turns = rotation & 3;
    const int uw = (turns & 1) ? h : w;     // the upright picture
    const int uh = (turns & 1) ? w : h;
    const double scale = (std::min)((double)OUT_WIDTH / uw, (double)OUT_HEIGHT / uh);
    const int dw = (std::max)(1, (std::min)(OUT_WIDTH, (int)std::floor(uw * scale + 0.5)));
    const int dh = (std::max)(1, (std::min)(OUT_HEIGHT, (int)std::floor(uh * scale + 0.5)));
    const int ox = (OUT_WIDTH - dw) / 2;
    const int oy = (OUT_HEIGHT - dh) / 2;

    const float step_u = (float)uw / dw;
    const float step_v = (float)uh / dh;
    // The turn clockwise, undone: a column of the upright picture runs along x (turns 0 and 2) or y (1 and 3) of the
    // decoded one, a row along the other; some of them backwards.
    //   turn 0: x = u,         y = v
    //   turn 1: x = v,         y = h-1-u
    //   turn 2: x = w-1-u,     y = h-1-v
    //   turn 3: x = w-1-v,     y = u
    const bool odd = (turns & 1) != 0;
    std::vector<Tap> cols, rows;
    makeTable(&cols, dw, step_u, mirror, uw, turns == 1 || turns == 2, odd ? h : w);
    makeTable(&rows, dh, step_v, false, uh, turns == 2 || turns == 3, odd ? w : h);

    std::uint8_t* dst = out->data();

    if (image.nv12)
    {
        const std::uint8_t* luma = image.yuv.data();
        const std::uint8_t* chroma = luma + (size_t)w * h;
        const int cs = image.chroma_stride;
        const bool full = image.full_range, bt709 = image.bt709;
        // Fixed point (8 bits): the coefficients of the four cases
        const int cr = bt709 ? (full ? 403 : 459) : (full ? 359 : 409);
        const int cgu = bt709 ? (full ? 48 : 55) : (full ? 88 : 100);
        const int cgv = bt709 ? (full ? 120 : 136) : (full ? 183 : 208);
        const int cb = bt709 ? (full ? 475 : 541) : (full ? 454 : 516);
        const int ys = full ? 256 : 298;
        const int yo = full ? 0 : 16;
        for (int row = 0; row < dh; row++)
        {
            std::uint8_t* o = dst + ((size_t)(oy + row) * OUT_WIDTH + ox) * 3;
            const Tap& tr = rows[(size_t)row];
            for (int col = 0; col < dw; col++, o += 3)
            {
                const Tap& tc = cols[(size_t)col];
                const Tap& tx = odd ? tr : tc;
                const Tap& ty = odd ? tc : tr;
                const std::uint8_t* r0 = luma + (size_t)ty.i0 * w;
                const std::uint8_t* r1 = luma + (size_t)ty.i1 * w;
                const int top = r0[tx.i0] * (256 - tx.f) + r0[tx.i1] * tx.f;
                const int bottom = r1[tx.i0] * (256 - tx.f) + r1[tx.i1] * tx.f;
                const int y = (top * (256 - ty.f) + bottom * ty.f + 32768) >> 16;
                const std::uint8_t* c = chroma + (size_t)(ty.i0 >> 1) * cs + (tx.i0 & ~1);
                const int d = c[0] - 128, e = c[1] - 128;
                const int base = ys * (y - yo);
                o[0] = (std::uint8_t)clamp255((base + cb * d + 128) >> 8);
                o[1] = (std::uint8_t)clamp255((base - cgu * d - cgv * e + 128) >> 8);
                o[2] = (std::uint8_t)clamp255((base + cr * e + 128) >> 8);
            }
        }
        return;
    }

    const std::uint8_t* src = image.bgra.data();
    for (int row = 0; row < dh; row++)
    {
        std::uint8_t* o = dst + ((size_t)(oy + row) * OUT_WIDTH + ox) * 3;
        const Tap& tr = rows[(size_t)row];
        for (int col = 0; col < dw; col++, o += 3)
        {
            const Tap& tc = cols[(size_t)col];
            const Tap& tx = odd ? tr : tc;
            const Tap& ty = odd ? tc : tr;
            const std::uint8_t* p00 = src + ((size_t)ty.i0 * w + tx.i0) * 4;
            const std::uint8_t* p10 = src + ((size_t)ty.i0 * w + tx.i1) * 4;
            const std::uint8_t* p01 = src + ((size_t)ty.i1 * w + tx.i0) * 4;
            const std::uint8_t* p11 = src + ((size_t)ty.i1 * w + tx.i1) * 4;
            for (int c = 0; c < 3; c++)
            {
                int top = p00[c] * (256 - tx.f) + p10[c] * tx.f;
                int bottom = p01[c] * (256 - tx.f) + p11[c] * tx.f;
                o[c] = (std::uint8_t)((top * (256 - ty.f) + bottom * ty.f + 32768) >> 16);
            }
        }
    }
}


} //namespace helper
