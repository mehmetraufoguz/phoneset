package audio

import (
	"encoding/binary"
	"math"
	"sync"
)

const maxRateAdjust = 0.02

type PlayoutStats struct {
	BufferedMs int
	Underruns  uint64
	Drops      uint64
}

// Playout is a jitter buffer for mono s16 audio. The network side writes bursts,
// the device side reads at its own pace.
//
//   - The read speed follows the fill level (up to +-2 %) so the two devices'
//     clocks can drift apart for hours without the buffer ever running dry or
//     overflowing; +-2 % is inaudible on speech.
//   - Until `target` samples are buffered it hands out silence (priming).
//   - On underrun it drains what it has, pads with silence and primes again, which
//     sounds better than stuttering through a half-empty buffer.
//   - If the writer gets ahead by more than `max` (clock drift, a TCP stall that
//     released all at once) the oldest audio is dropped down to `target`.
type Playout struct {
	rate   int
	target int
	max    int

	mu        sync.Mutex
	ring      []int16
	r, n      int
	frac      float64 // fractional read position between ring[r] and ring[r+1]
	primed    bool
	underruns uint64
	drops     uint64
}

func NewPlayout(rate, targetMs, maxMs int) *Playout {
	max := rate * maxMs / 1000
	return &Playout{
		rate:   rate,
		target: rate * targetMs / 1000,
		max:    max,
		ring:   make([]int16, max*2),
	}
}

// Write takes s16le mono bytes.
func (p *Playout) Write(pcm []byte) {
	count := len(pcm) / 2
	if count == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.n+count > p.max {
		keep := p.target
		if count > keep {
			keep = 0
		}
		p.drop(p.n - keep)
		p.drops++
	}
	if count > len(p.ring) {
		pcm = pcm[(count-len(p.ring))*2:]
		count = len(p.ring)
	}
	p.drop(p.n + count - len(p.ring))
	w := (p.r + p.n) % len(p.ring)
	for i := 0; i < count; i++ {
		p.ring[w] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		w++
		if w == len(p.ring) {
			w = 0
		}
	}
	p.n += count
}

// Read always fills dst, with silence when it has nothing to give.
func (p *Playout) Read(dst []int16) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.primed {
		if p.n < p.target {
			clear(dst)
			return
		}
		p.primed = true
	}

	// Fuller than the target: consume a little faster; emptier: a little slower.
	rate := 1 + math.Max(-maxRateAdjust, math.Min(maxRateAdjust,
		(float64(p.n-p.target)/float64(p.target))*maxRateAdjust))

	size := len(p.ring)
	for i := range dst {
		if p.n < 2 {
			clear(dst[i:])
			p.underruns++
			p.primed = false
			p.frac = 0
			return
		}
		a := float64(p.ring[p.r])
		b := float64(p.ring[(p.r+1)%size])
		dst[i] = int16(a + (b-a)*p.frac)

		p.frac += rate
		adv := int(p.frac)
		p.frac -= float64(adv)
		p.r = (p.r + adv) % size
		p.n -= adv
	}
}

func (p *Playout) Stats() PlayoutStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlayoutStats{BufferedMs: p.n * 1000 / p.rate, Underruns: p.underruns, Drops: p.drops}
}

func (p *Playout) drop(count int) {
	if count <= 0 {
		return
	}
	if count > p.n {
		count = p.n
	}
	p.r = (p.r + count) % len(p.ring)
	p.n -= count
}
