// Turns the records of the stream into pictures: a JPEG with WIC, H.264 with Media Foundation's decoder (a software
// decoder in its low latency mode; no GPU path yet).
#pragma once

#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <wincodec.h>

#include <string>

#include "Com.h"
#include "Types.h"


namespace helper {


/// How many worker threads the software H.264 decoder may use (0: its own choice). --decoder-threads, to try values.
extern int decoderThreads;

/// Not thread safe: use it from one thread that has called CoInitializeEx and MFStartup.
class Decoder
{
 public:
    Decoder() = default;

    /// Makes the decoder wait for the next key frame (a frame was dropped in front of it).
    void lostFrame() { m_needs_key = true; m_flush_pending = true; }

    /// Decodes a record. False when there is no picture to show: an H.264 frame while the decoder waits for a key frame,
    /// a decoder that has nothing out yet, or a frame that did not decode (then the decoder waits for a key frame).
    bool decode(const Record& record, Image* out);

    /// What the H.264 decoder is doing, for the log: e.g. "NV12 1920x1080 limited BT.709".
    const std::string& h264Description() const { return m_description; }

    long long failures() const { return m_failures; }

    /// What went wrong last, with the HRESULT, e.g. "ProcessInput 0xc00d36b5" (empty when nothing did).
    const std::string& lastError() const { return m_last_error; }

 private:
    // JPEG
    Com<IWICImagingFactory> m_wic;
    bool decodeJpeg(const Record& record, Image* out);

    // H.264
    Com<IMFTransform>   m_mft;
    Com<IMFSample>      m_out_sample;
    bool                m_provides_samples = false;
    int                 m_width = 0;            // what the decoder was made for (the record's size)
    int                 m_height = 0;
    bool                m_needs_key = true;
    bool                m_flush_pending = false;
    // The output type as the decoder gave it
    int                 m_frame_width = 0;
    int                 m_frame_height = 0;
    int                 m_stride = 0;
    RECT                m_visible = {};
    bool                m_full_range = false;
    bool                m_bt709 = true;
    std::string         m_description;
    long long           m_failures = 0;
    std::string         m_last_error;
    void fail(const char* what, HRESULT hr);

    bool decodeH264(const Record& record, Image* out);
    bool makeDecoder(int width, int height);
    bool chooseOutputType();
    void resetDecoder();
    bool takeOutputs(Image* out);
    bool convert(IMFSample* sample, Image* out);
};


} //namespace helper
