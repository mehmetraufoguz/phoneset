#include "Decode.h"

#include <codecapi.h>
#include <mferror.h>
#include <wmcodecdsp.h>
#include <ppl.h>

#include <algorithm>
#include <cstdio>
#include <cstring>

#pragma comment(lib, "mfplat.lib")
#pragma comment(lib, "mf.lib")
#pragma comment(lib, "mfuuid.lib")
#pragma comment(lib, "wmcodecdspuuid.lib")
#pragma comment(lib, "windowscodecs.lib")
#pragma comment(lib, "strmiids.lib")
#pragma comment(lib, "ole32.lib")


namespace helper {

namespace {

constexpr int MAX_DIMENSION = 8192;

inline int clamp255(int v) { return v < 0 ? 0 : (v > 255 ? 255 : v); }

}


void Decoder::fail(const char* what, HRESULT hr)
{
    m_failures++;
    char text[96];
    std::snprintf(text, sizeof text, "%s 0x%08lx", what, (unsigned long)hr);
    m_last_error = text;
}

bool Decoder::decode(const Record& record, Image* out)
{
    try
    {
        switch (record.codec)
        {
        case Record::JPEG: return decodeJpeg(record, out);
        case Record::H264: return decodeH264(record, out);
        default: return false;
        }
    }
    catch (...)
    {
        m_failures++;
        m_needs_key = true;
        return false;
    }
}


// ---------------------------------------------------------------------------------------------------- JPEG

bool Decoder::decodeJpeg(const Record& record, Image* out)
{
    if (record.payload.size() < 4) return false;
    if (!m_wic)
    {
        if (FAILED(CoCreateInstance(CLSID_WICImagingFactory1, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&m_wic))))
        {
            m_failures++;
            return false;
        }
    }
    Com<IWICStream> stream;
    Com<IWICBitmapDecoder> decoder;
    Com<IWICBitmapFrameDecode> frame;
    Com<IWICFormatConverter> converter;
    if (FAILED(m_wic->CreateStream(&stream)) ||
        FAILED(stream->InitializeFromMemory(const_cast<BYTE*>(record.payload.data()), (DWORD)record.payload.size())) ||
        FAILED(m_wic->CreateDecoderFromStream(stream.get(), nullptr, WICDecodeMetadataCacheOnDemand, &decoder)) ||
        FAILED(decoder->GetFrame(0, &frame)))
    {
        m_failures++;
        return false;
    }
    UINT width = 0, height = 0;
    if (FAILED(frame->GetSize(&width, &height)) || width == 0 || height == 0 || width > MAX_DIMENSION || height > MAX_DIMENSION)
    {
        m_failures++;
        return false;
    }
    // The picture is upright already (a JPEG of the stream always is): the record's rotation is 0, and this is not looked at.
    if (FAILED(m_wic->CreateFormatConverter(&converter)) ||
        FAILED(converter->Initialize(frame.get(), GUID_WICPixelFormat32bppBGRA, WICBitmapDitherTypeNone, nullptr, 0.0,
                                     WICBitmapPaletteTypeCustom)))
    {
        m_failures++;
        return false;
    }
    out->width = (int)width;
    out->height = (int)height;
    out->bgra.resize((size_t)width * height * 4);
    if (FAILED(converter->CopyPixels(nullptr, width * 4, (UINT)out->bgra.size(), out->bgra.data())))
    {
        m_failures++;
        return false;
    }
    return true;
}


// --------------------------------------------------------------------------------------------------- H.264

void Decoder::resetDecoder()
{
    m_mft.reset();
    m_out_sample.reset();
    m_width = m_height = 0;
    m_needs_key = true;
}

bool Decoder::makeDecoder(int width, int height)
{
    resetDecoder();
    if (FAILED(CoCreateInstance(CLSID_CMSH264DecoderMFT, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&m_mft))))
    {
        return false;
    }

    // Low latency: a picture comes out for each one that goes in, without waiting for later ones.
    Com<IMFAttributes> attributes;
    if (SUCCEEDED(m_mft->GetAttributes(&attributes)))
    {
        attributes->SetUINT32(MF_LOW_LATENCY, TRUE);
    }
    Com<ICodecAPI> codec;
    if (SUCCEEDED(m_mft->QueryInterface(IID_PPV_ARGS(&codec))))
    {
        VARIANT on;
        VariantInit(&on);
        on.vt = VT_BOOL;
        on.boolVal = VARIANT_TRUE;
        codec->SetValue(&CODECAPI_AVLowLatencyMode, &on);
    }

    Com<IMFMediaType> input;
    if (FAILED(MFCreateMediaType(&input))) { m_mft.reset(); return false; }
    input->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    input->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_H264);      // Annex B: start codes, parameter sets in the stream
    input->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    MFSetAttributeSize(input.get(), MF_MT_FRAME_SIZE, (UINT32)width, (UINT32)height);
    if (FAILED(m_mft->SetInputType(0, input.get(), 0)) || !chooseOutputType())
    {
        m_mft.reset();
        return false;
    }
    m_mft->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0);
    m_mft->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);
    m_mft->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0);
    m_width = width;
    m_height = height;
    m_flush_pending = false;
    return true;
}

/// Takes NV12 as the output and reads what the decoder says about it (size, stride, the visible part, colours).
bool Decoder::chooseOutputType()
{
    for (DWORD i = 0;; i++)
    {
        Com<IMFMediaType> type;
        HRESULT hr = m_mft->GetOutputAvailableType(0, i, &type);
        if (FAILED(hr)) return false;
        GUID subtype = {};
        if (FAILED(type->GetGUID(MF_MT_SUBTYPE, &subtype)) || subtype != MFVideoFormat_NV12) continue;
        if (FAILED(m_mft->SetOutputType(0, type.get(), 0))) return false;

        UINT32 w = 0, h = 0;
        MFGetAttributeSize(type.get(), MF_MT_FRAME_SIZE, &w, &h);
        m_frame_width = (int)w;
        m_frame_height = (int)h;
        UINT32 stride = MFGetAttributeUINT32(type.get(), MF_MT_DEFAULT_STRIDE, 0);
        if ((INT32)stride <= 0)
        {
            LONG s = 0;
            if (FAILED(MFGetStrideForBitmapInfoHeader(FCC('NV12'), w, &s)) || s <= 0) s = (LONG)w;
            stride = (UINT32)s;
        }
        m_stride = (int)stride;
        m_visible = { 0, 0, m_frame_width, m_frame_height };
        MFVideoArea area = {};
        if (SUCCEEDED(type->GetBlob(MF_MT_MINIMUM_DISPLAY_APERTURE, (UINT8*)&area, sizeof area, nullptr)) &&
            area.Area.cx > 0 && area.Area.cy > 0)
        {
            m_visible = { area.OffsetX.value, area.OffsetY.value, area.OffsetX.value + area.Area.cx,
                          area.OffsetY.value + area.Area.cy };
        }
        // What the stream says about its colours; without it: limited range, and BT.709 from 720 lines up.
        UINT32 range = MFGetAttributeUINT32(type.get(), MF_MT_VIDEO_NOMINAL_RANGE, MFNominalRange_Unknown);
        UINT32 matrix = MFGetAttributeUINT32(type.get(), MF_MT_YUV_MATRIX, MFVideoTransferMatrix_Unknown);
        m_full_range = range == MFNominalRange_0_255;
        m_bt709 = matrix == MFVideoTransferMatrix_BT709 || (matrix != MFVideoTransferMatrix_BT601 && h >= 720);
        char text[96];
        std::snprintf(text, sizeof text, "NV12 %dx%d (visible %ldx%ld), %s range, %s", m_frame_width, m_frame_height,
                      m_visible.right - m_visible.left, m_visible.bottom - m_visible.top,
                      m_full_range ? "full" : "limited", m_bt709 ? "BT.709" : "BT.601");
        m_description = text;

        // A sample for the decoder to write into, unless it brings its own.
        MFT_OUTPUT_STREAM_INFO info = {};
        m_out_sample.reset();
        m_provides_samples = false;
        if (SUCCEEDED(m_mft->GetOutputStreamInfo(0, &info)))
        {
            m_provides_samples = (info.dwFlags & (MFT_OUTPUT_STREAM_PROVIDES_SAMPLES | MFT_OUTPUT_STREAM_CAN_PROVIDE_SAMPLES)) != 0;
            if (!m_provides_samples)
            {
                Com<IMFMediaBuffer> buffer;
                if (FAILED(MFCreateSample(&m_out_sample)) ||
                    FAILED(MFCreateAlignedMemoryBuffer(info.cbSize ? info.cbSize : (DWORD)(m_stride * m_frame_height * 3 / 2),
                                                       info.cbAlignment > 0 ? info.cbAlignment - 1 : 0, &buffer)) ||
                    FAILED(m_out_sample->AddBuffer(buffer.get())))
                {
                    return false;
                }
            }
        }
        return true;
    }
}

bool Decoder::decodeH264(const Record& record, Image* out)
{
    if (record.width <= 0 || record.height <= 0 || record.width > MAX_DIMENSION || record.height > MAX_DIMENSION)
    {
        return false;
    }
    if (record.key)
    {
        // A key frame starts (or restarts) the decoder; a new size needs a decoder made for it.
        if (!m_mft || record.width != m_width || record.height != m_height)
        {
            if (!makeDecoder(record.width, record.height))
            {
                m_failures++;
                return false;
            }
        }
        else if (m_flush_pending)
        {
            m_mft->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0);
            m_mft->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0);
            m_flush_pending = false;
        }
        m_needs_key = false;
    }
    if (m_needs_key || !m_mft) return false;

    Com<IMFMediaBuffer> buffer;
    Com<IMFSample> sample;
    if (FAILED(MFCreateMemoryBuffer((DWORD)record.payload.size(), &buffer))) return false;
    BYTE* data = nullptr;
    if (FAILED(buffer->Lock(&data, nullptr, nullptr))) return false;
    std::memcpy(data, record.payload.data(), record.payload.size());
    buffer->Unlock();
    buffer->SetCurrentLength((DWORD)record.payload.size());
    if (FAILED(MFCreateSample(&sample)) || FAILED(sample->AddBuffer(buffer.get()))) return false;
    sample->SetSampleTime((LONGLONG)record.captured_ms * 10000);
    sample->SetSampleDuration(333333);

    bool got = false;
    HRESULT hr = m_mft->ProcessInput(0, sample.get(), 0);
    if (hr == MF_E_NOTACCEPTING)
    {
        // The decoder has pictures waiting: take them, then try again.
        Image ignored;
        takeOutputs(&ignored);
        hr = m_mft->ProcessInput(0, sample.get(), 0);
    }
    if (FAILED(hr))
    {
        fail("ProcessInput", hr);
        resetDecoder();     // waits for the next key frame, with a decoder made new
        return false;
    }
    got = takeOutputs(out);
    return got;
}

/// Takes every picture the decoder has ready; `out` is the last one. False when there was none.
bool Decoder::takeOutputs(Image* out)
{
    bool got = false;
    for (int guard = 0; guard < 16; guard++)
    {
        MFT_OUTPUT_DATA_BUFFER output = {};
        output.dwStreamID = 0;
        output.pSample = m_provides_samples ? nullptr : m_out_sample.get();
        if (output.pSample)
        {
            // The buffer of the last picture is empty again before the decoder writes the next one.
            Com<IMFMediaBuffer> previous;
            if (SUCCEEDED(output.pSample->GetBufferByIndex(0, &previous))) previous->SetCurrentLength(0);
        }
        DWORD status = 0;
        HRESULT hr = m_mft->ProcessOutput(0, 1, &output, &status);
        if (hr == MF_E_TRANSFORM_NEED_MORE_INPUT) break;
        if (hr == MF_E_TRANSFORM_STREAM_CHANGE)
        {
            if (output.pEvents) output.pEvents->Release();
            if (!chooseOutputType())
            {
                fail("output type", E_FAIL);
                resetDecoder();
                return got;
            }
            continue;
        }
        if (FAILED(hr))
        {
            if (output.pEvents) output.pEvents->Release();
            fail("ProcessOutput", hr);
            resetDecoder();
            return got;
        }
        IMFSample* produced = output.pSample;
        if (produced && convert(produced, out)) got = true;
        if (m_provides_samples && output.pSample) output.pSample->Release();
        if (output.pEvents) output.pEvents->Release();
    }
    return got;
}

/// NV12 to BGRA, the visible part only.
bool Decoder::convert(IMFSample* sample, Image* out)
{
    Com<IMFMediaBuffer> buffer;
    if (FAILED(sample->ConvertToContiguousBuffer(&buffer))) return false;
    BYTE* data = nullptr;
    DWORD length = 0;
    if (FAILED(buffer->Lock(&data, nullptr, &length))) return false;

    const int stride = m_stride;
    const int left = m_visible.left, top = m_visible.top;
    const int width = m_visible.right - m_visible.left, height = m_visible.bottom - m_visible.top;
    // The chroma plane starts behind the luma plane, whose height may be padded (1088 for 1080).
    const int slice_height = stride > 0 ? (int)((long long)length * 2 / 3 / stride) : 0;
    bool ok = width > 0 && height > 0 && stride >= left + width && slice_height >= top + height;
    if (ok)
    {
        const BYTE* luma = data;
        const BYTE* chroma = data + (size_t)stride * slice_height;
        out->width = width;
        out->height = height;
        out->bgra.resize((size_t)width * height * 4);
        BYTE* pixels = out->bgra.data();
        const bool full = m_full_range, bt709 = m_bt709;
        // Fixed point (8 bits): the coefficients of the four cases
        const int cr = bt709 ? (full ? 403 : 459) : (full ? 359 : 409);
        const int cgu = bt709 ? (full ? 48 : 55) : (full ? 88 : 100);
        const int cgv = bt709 ? (full ? 120 : 136) : (full ? 183 : 208);
        const int cb = bt709 ? (full ? 475 : 541) : (full ? 454 : 516);
        const int ys = full ? 256 : 298;
        const int yo = full ? 0 : 16;
        concurrency::parallel_for(0, height, [&](int y) {
            const BYTE* yr = luma + (size_t)(top + y) * stride + left;
            const BYTE* cr_row = chroma + (size_t)((top + y) >> 1) * stride;
            BYTE* o = pixels + (size_t)y * width * 4;
            for (int x = 0; x < width; x++)
            {
                int pair = (left + x) & ~1;
                int c = ys * (yr[x] - yo);
                int d = cr_row[pair] - 128;
                int e = cr_row[pair + 1] - 128;
                o[4 * x + 0] = (BYTE)clamp255((c + cb * d + 128) >> 8);
                o[4 * x + 1] = (BYTE)clamp255((c - cgu * d - cgv * e + 128) >> 8);
                o[4 * x + 2] = (BYTE)clamp255((c + cr * e + 128) >> 8);
                o[4 * x + 3] = 255;
            }
        });
    }
    buffer->Unlock();
    return ok;
}


} //namespace helper
