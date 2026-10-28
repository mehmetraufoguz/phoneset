package audio

import "time"

// The two directions every backend needs, kept free of any platform API so the
// Windows and macOS backends share them.

const (
	phoneRate = 48000
	// PhoneFrameSamples is one 20 ms mono frame at the phone's rate.
	PhoneFrameSamples = phoneRate / 50
	// Backends poll their device this often and keep this much queued in it.
	pollInterval   = 5 * time.Millisecond
	renderTargetMs = 30
)

// Feeder turns the phone's microphone (48 kHz mono, held in a Playout buffer)
// into frames in the device's own format, on demand. The device clock decides
// when and how much; the Playout buffer absorbs the difference.
type Feeder struct {
	playout *Playout
	res     *Resampler
	format  RawFormat
	chunk   []int16
	in      []float32
	pending []float32
}

func NewFeeder(p *Playout, format RawFormat) *Feeder {
	return &Feeder{
		playout: p,
		res:     NewResampler(phoneRate, format.Rate, 1),
		format:  format,
		chunk:   make([]int16, phoneRate/100), // 10 ms
		in:      make([]float32, phoneRate/100),
	}
}

// Fill writes exactly frames device frames into dst, which must hold
// frames*format.BlockAlign() bytes. The mono signal goes to every channel.
func (f *Feeder) Fill(dst []byte, frames int) {
	for len(f.pending) < frames {
		f.playout.Read(f.chunk)
		for i, v := range f.chunk {
			f.in[i] = float32(v) / 32768
		}
		f.pending = f.res.Process(f.pending, f.in)
	}
	Encode(dst, f.pending[:frames], f.format)
	f.pending = f.pending[:copy(f.pending, f.pending[frames:])]
}

// Capturer turns whatever the device delivers into the 20 ms, 48 kHz, stereo,
// s16le frames the phone expects.
type Capturer struct {
	format  RawFormat
	res     *Resampler
	framer  *Framer
	onFrame FrameFunc
	raw     []float32
	stereo  []float32
	out     []float32
	pcm     []byte
}

func NewCapturer(format RawFormat, onFrame FrameFunc) *Capturer {
	return &Capturer{
		format:  format,
		res:     NewResampler(format.Rate, phoneRate, 2),
		framer:  NewFramer(phoneRate / 50 * 4), // 20 ms of stereo s16
		onFrame: onFrame,
	}
}

// Push takes raw interleaved device bytes.
func (c *Capturer) Push(data []byte) {
	c.raw = Decode(c.raw[:0], data, c.format)
	c.flush()
}

// PushSilence takes frames of digital silence (a packet the device flagged as such).
func (c *Capturer) PushSilence(frames int) {
	c.raw = append(c.raw[:0], make([]float32, frames*c.format.Channels)...)
	c.flush()
}

func (c *Capturer) flush() {
	c.stereo = ToStereo(c.stereo[:0], c.raw, c.format.Channels)
	c.out = c.res.Process(c.out[:0], c.stereo)
	c.pcm = FloatToS16(c.pcm[:0], c.out)
	c.framer.Push(c.pcm, func(frame []byte) { c.onFrame(frame) })
}

// SilenceGate forwards frames until the source has been digitally silent for half a
// second, then holds them back until sound returns. Capture devices that deliver
// zeros forever (a system-audio tap, a virtual cable) would otherwise stream
// silence to the phone around the clock.
func SilenceGate(next FrameFunc) FrameFunc {
	const graceFrames = 25 // 500 ms
	quiet := 0
	return func(pcm []byte) {
		if allZero(pcm) {
			quiet++
			if quiet > graceFrames {
				return
			}
		} else {
			quiet = 0
		}
		next(pcm)
	}
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
