#include "Slate.h"

#include <windows.h>
#include <cstring>
#include <algorithm>

#pragma comment(lib, "gdi32.lib")


namespace softcam {

namespace {

// The macOS slate: background (0.07, 0.08, 0.11), bar (0.23, 0.42, 1.0) of 80 x 8 pixels, white bold text.
const BYTE BACKGROUND_R = 18, BACKGROUND_G = 20, BACKGROUND_B = 28;
const BYTE BAR_R = 59, BAR_G = 107, BAR_B = 255;
const int  BAR_WIDTH = 80;
const int  BAR_HEIGHT = 8;
const int  BAR_STEP = 12;

// Sizes are for a picture 720 pixels high and scale with the height; the baseline is the given part of the height up.
const int    TITLE_SIZE = 84;
const double TITLE_BASELINE = 0.55;
const int    LINE_SIZE = 36;
const double LINE_BASELINE = 0.42;

std::size_t calcStride(int width)
{
    return (static_cast<std::size_t>(width) * 3 + 3) & ~static_cast<std::size_t>(3);
}

/// Segoe UI Bold where it is there, else Arial Bold: GDI would quietly pick something else for a font that is
/// missing, so the face it really took is checked.
HFONT createFont(HDC dc, int pixel_height)
{
    static const wchar_t* const faces[] = { L"Segoe UI", L"Arial" };
    HFONT chosen = nullptr;
    for (const wchar_t* face : faces)
    {
        HFONT font = CreateFontW(-pixel_height, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                                 OUT_TT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                                 DEFAULT_PITCH | FF_SWISS, face);
        if (!font)
        {
            continue;
        }
        HGDIOBJ old = SelectObject(dc, font);
        wchar_t got[LF_FACESIZE] = {};
        GetTextFaceW(dc, LF_FACESIZE, got);
        SelectObject(dc, old);
        if (0 == lstrcmpiW(got, face))
        {
            chosen = font;
            break;
        }
        if (!chosen)
        {
            chosen = font;      // whatever GDI gave, if neither face is there
        }
        else
        {
            DeleteObject(font);
        }
    }
    return chosen;
}

void drawText(HDC dc, int width, int height, int size_at_720, double baseline_up, const wchar_t* text)
{
    int pixel_height = (std::max)(1, static_cast<int>(size_at_720 * height / 720.0 + 0.5));
    HFONT font = createFont(dc, pixel_height);
    if (!font)
    {
        return;
    }
    HGDIOBJ old = SelectObject(dc, font);
    SetTextAlign(dc, TA_BASELINE | TA_CENTER | TA_NOUPDATECP);
    int baseline_down = height - static_cast<int>(height * baseline_up + 0.5);
    TextOutW(dc, width / 2, baseline_down, text, lstrlenW(text));
    SelectObject(dc, old);
    DeleteObject(font);
}

} //namespace


Slate::Slate(int width, int height) :
    m_width(width),
    m_height(height),
    m_stride(calcStride(width)),
    m_base(calcStride(width) * static_cast<std::size_t>(height))
{
    // The background is also written directly, so that the slate is a plain colour even if GDI is not available.
    for (int y = 0; y < height; y++)
    {
        std::uint8_t* row = m_base.data() + m_stride * y;
        for (int x = 0; x < width; x++)
        {
            row[3 * x + 0] = BACKGROUND_B;
            row[3 * x + 1] = BACKGROUND_G;
            row[3 * x + 2] = BACKGROUND_R;
        }
    }

    HDC screen = GetDC(nullptr);
    HDC dc = screen ? CreateCompatibleDC(screen) : nullptr;
    if (dc)
    {
        BITMAPINFO info = {};
        info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
        info.bmiHeader.biWidth = width;
        info.bmiHeader.biHeight = height;       // positive: bottom-up, like the filter's media type
        info.bmiHeader.biPlanes = 1;
        info.bmiHeader.biBitCount = 24;
        info.bmiHeader.biCompression = BI_RGB;
        void* bits = nullptr;
        HBITMAP bitmap = CreateDIBSection(dc, &info, DIB_RGB_COLORS, &bits, nullptr, 0);
        if (bitmap && bits)
        {
            HGDIOBJ old = SelectObject(dc, bitmap);
            RECT all = { 0, 0, width, height };
            HBRUSH brush = CreateSolidBrush(RGB(BACKGROUND_R, BACKGROUND_G, BACKGROUND_B));
            FillRect(dc, &all, brush);
            DeleteObject(brush);
            SetBkMode(dc, TRANSPARENT);
            SetTextColor(dc, RGB(255, 255, 255));
            drawText(dc, width, height, TITLE_SIZE, TITLE_BASELINE, L"phoneset Camera");
            drawText(dc, width, height, LINE_SIZE, LINE_BASELINE, L"Waiting for your phone\x2026");
            GdiFlush();
            std::memcpy(m_base.data(), bits, m_base.size());
            SelectObject(dc, old);
        }
        if (bitmap)
        {
            DeleteObject(bitmap);
        }
        DeleteDC(dc);
    }
    if (screen)
    {
        ReleaseDC(nullptr, screen);
    }
}

void Slate::render(std::uint8_t* dib, std::size_t dib_size, std::uint64_t frame_number) const
{
    if (!dib || dib_size < m_base.size())
    {
        return;
    }
    std::memcpy(dib, m_base.data(), m_base.size());

    // The bar is on the bottom edge: the first rows of a bottom-up DIB. Like the macOS slate it is cut at the right edge.
    int bar_x = static_cast<int>((frame_number * BAR_STEP) % static_cast<std::uint64_t>(m_width));
    int bar_end = (std::min)(bar_x + BAR_WIDTH, m_width);
    for (int y = 0; y < (std::min)(BAR_HEIGHT, m_height); y++)
    {
        std::uint8_t* row = dib + m_stride * y;
        for (int x = bar_x; x < bar_end; x++)
        {
            row[3 * x + 0] = BAR_B;
            row[3 * x + 1] = BAR_G;
            row[3 * x + 2] = BAR_R;
        }
    }
}


} //namespace softcam
