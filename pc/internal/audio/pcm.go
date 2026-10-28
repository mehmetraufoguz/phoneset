package audio

import (
	"encoding/binary"
	"math"
)

// RawFormat describes interleaved little-endian PCM as delivered by a device.
type RawFormat struct {
	Rate     int
	Channels int
	Bits     int // container size: 16, 24 or 32
	Float    bool
}

func (f RawFormat) BlockAlign() int { return f.Channels * f.Bits / 8 }

// Decode converts raw device bytes to interleaved float32 in [-1, 1].
func Decode(dst []float32, raw []byte, f RawFormat) []float32 {
	bytesPer := f.Bits / 8
	n := len(raw) / bytesPer
	switch {
	case f.Float && f.Bits == 32:
		for i := 0; i < n; i++ {
			dst = append(dst, math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:])))
		}
	case f.Bits == 16:
		for i := 0; i < n; i++ {
			dst = append(dst, float32(int16(binary.LittleEndian.Uint16(raw[i*2:])))/32768)
		}
	case f.Bits == 24:
		for i := 0; i < n; i++ {
			b := raw[i*3:]
			v := int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24) >> 8
			dst = append(dst, float32(v)/8388608)
		}
	case f.Bits == 32:
		for i := 0; i < n; i++ {
			dst = append(dst, float32(int32(binary.LittleEndian.Uint32(raw[i*4:])))/2147483648)
		}
	}
	return dst
}

// Encode writes mono samples into every channel of an interleaved device buffer.
func Encode(dst []byte, mono []float32, f RawFormat) {
	bytesPer := f.Bits / 8
	o := 0
	for _, s := range mono {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		for c := 0; c < f.Channels; c++ {
			switch {
			case f.Float && f.Bits == 32:
				binary.LittleEndian.PutUint32(dst[o:], math.Float32bits(s))
			case f.Bits == 16:
				binary.LittleEndian.PutUint16(dst[o:], uint16(int16(s*32767)))
			case f.Bits == 24:
				v := int32(s * 8388607)
				dst[o], dst[o+1], dst[o+2] = byte(v), byte(v>>8), byte(v>>16)
			case f.Bits == 32:
				binary.LittleEndian.PutUint32(dst[o:], uint32(int32(s*2147483647)))
			}
			o += bytesPer
		}
	}
}

// ToStereo reduces or expands interleaved frames to stereo. Multichannel input
// keeps front left/right; the rest of the layout is dropped.
func ToStereo(dst, src []float32, channels int) []float32 {
	switch channels {
	case 1:
		for _, s := range src {
			dst = append(dst, s, s)
		}
	case 2:
		dst = append(dst, src...)
	default:
		for i := 0; i+channels <= len(src); i += channels {
			dst = append(dst, src[i], src[i+1])
		}
	}
	return dst
}

// FloatToS16 appends s16le bytes for float samples.
func FloatToS16(dst []byte, src []float32) []byte {
	for _, s := range src {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		dst = binary.LittleEndian.AppendUint16(dst, uint16(int16(s*32767)))
	}
	return dst
}

// S16ToFloat appends float samples for s16le bytes.
func S16ToFloat(dst []float32, src []byte) []float32 {
	for i := 0; i+1 < len(src); i += 2 {
		dst = append(dst, float32(int16(binary.LittleEndian.Uint16(src[i:])))/32768)
	}
	return dst
}

// Resampler is a streaming linear-interpolation resampler for interleaved audio.
// Linear interpolation is plenty for speech and keeps latency at one frame.
type Resampler struct {
	step  float64 // input frames consumed per output frame
	ch    int
	pos   float64
	carry []float32
	buf   []float32
}

func NewResampler(inRate, outRate, channels int) *Resampler {
	return &Resampler{step: float64(inRate) / float64(outRate), ch: channels}
}

func (r *Resampler) Process(dst, in []float32) []float32 {
	if r.step == 1 {
		return append(dst, in...)
	}
	ch := r.ch
	r.buf = append(append(r.buf[:0], r.carry...), in...)
	frames := len(r.buf) / ch
	for {
		i := int(r.pos)
		if i+1 >= frames {
			break
		}
		frac := float32(r.pos - float64(i))
		for c := 0; c < ch; c++ {
			a, b := r.buf[i*ch+c], r.buf[(i+1)*ch+c]
			dst = append(dst, a+(b-a)*frac)
		}
		r.pos += r.step
	}
	keep := int(r.pos)
	if keep > frames {
		keep = frames
	}
	r.carry = append(r.carry[:0], r.buf[keep*ch:]...)
	r.pos -= float64(keep)
	return dst
}

// Framer slices a byte stream into fixed-size frames.
type Framer struct {
	size int
	buf  []byte
}

func NewFramer(size int) *Framer { return &Framer{size: size} }

// Push appends data and calls emit for every complete frame. The slice passed
// to emit is only valid during the call.
func (f *Framer) Push(data []byte, emit func([]byte)) {
	f.buf = append(f.buf, data...)
	for len(f.buf) >= f.size {
		emit(f.buf[:f.size])
		f.buf = f.buf[:copy(f.buf, f.buf[f.size:])]
	}
}
