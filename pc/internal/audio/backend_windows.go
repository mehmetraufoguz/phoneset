//go:build windows

package audio

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// WASAPI backend.
//
//   - Phone microphone -> a render endpoint (normally a virtual cable such as
//     VB-CABLE's "CABLE Input"). Apps then record from the cable's other end.
//   - PC audio -> phone: WASAPI loopback capture of a render endpoint.
//
// Everything is shared mode in the endpoint's own mix format; conversion to and
// from 48 kHz s16 happens here. COM objects live on dedicated, OS-thread-locked
// goroutines, one per stream.

func init() {
	// Go's timers follow the system tick; ask for 1 ms so 5 ms polling is accurate.
	_, _, _ = syscall.NewLazyDLL("winmm.dll").NewProc("timeBeginPeriod").Call(1)
}

const (
	pollInterval   = 5 * time.Millisecond
	renderTargetMs = 30
	hundredNsPerMs = 10_000
)

type winBackend struct{}

func NewBackend() Backend { return winBackend{} }

func (winBackend) Name() string { return "WASAPI" }

// withCOM runs fn on a locked OS thread with COM initialised.
func withCOM(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		release, err := comInit()
		if err != nil {
			done <- err
			return
		}
		defer release()
		done <- fn()
	}()
	return <-done
}

func comInit() (func(), error) {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var oe *ole.OleError
		// S_FALSE: already initialised on this thread, which is fine.
		if !errors.As(err, &oe) || oe.Code() != 1 {
			runtime.UnlockOSThread()
			return nil, fmt.Errorf("initialise COM: %w", err)
		}
	}
	return func() {
		ole.CoUninitialize()
		runtime.UnlockOSThread()
	}, nil
}

func newEnumerator() (*wca.IMMDeviceEnumerator, error) {
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return nil, fmt.Errorf("create device enumerator: %w", err)
	}
	return mmde, nil
}

func deviceInfo(mmd *wca.IMMDevice) (id, name string) {
	_ = mmd.GetId(&id)
	var ps *wca.IPropertyStore
	if err := mmd.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
		return id, id
	}
	defer ps.Release()
	var pv wca.PROPVARIANT
	if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err == nil {
		name = pv.String()
	}
	if name == "" {
		name = id
	}
	return id, name
}

func (winBackend) Playback() (devs []Device, err error) {
	err = withCOM(func() error {
		mmde, err := newEnumerator()
		if err != nil {
			return err
		}
		defer mmde.Release()

		var defaultID string
		var def *wca.IMMDevice
		if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &def); err == nil {
			defaultID, _ = deviceInfo(def)
			def.Release()
		}

		var dc *wca.IMMDeviceCollection
		if err := mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &dc); err != nil {
			return fmt.Errorf("list playback devices: %w", err)
		}
		defer dc.Release()
		var count uint32
		if err := dc.GetCount(&count); err != nil {
			return err
		}
		for i := uint32(0); i < count; i++ {
			var mmd *wca.IMMDevice
			if err := dc.Item(i, &mmd); err != nil {
				continue
			}
			id, name := deviceInfo(mmd)
			mmd.Release()
			devs = append(devs, Device{ID: id, Name: name, Default: id == defaultID, Virtual: looksVirtual(name)})
		}
		return nil
	})
	return devs, err
}

func (b winBackend) ResolveMic(selector string) (Device, error) {
	devs, err := b.Playback()
	if err != nil {
		return Device{}, err
	}
	if selector == "" {
		return firstVirtual(devs)
	}
	return match(devs, selector)
}

func (b winBackend) ResolveSpeaker(selector string) (Device, error) {
	devs, err := b.Playback()
	if err != nil {
		return Device{}, err
	}
	if selector == "" {
		return defaultDevice(devs)
	}
	return match(devs, selector)
}

// client is an initialised shared-mode IAudioClient on one endpoint.
type client struct {
	mmde   *wca.IMMDeviceEnumerator
	mmd    *wca.IMMDevice
	ac     *wca.IAudioClient
	format RawFormat
	frames uint32 // device buffer size in frames
}

func (c *client) close() {
	if c.ac != nil {
		c.ac.Release()
	}
	if c.mmd != nil {
		c.mmd.Release()
	}
	if c.mmde != nil {
		c.mmde.Release()
	}
}

func findEndpoint(mmde *wca.IMMDeviceEnumerator, id string) (*wca.IMMDevice, error) {
	var dc *wca.IMMDeviceCollection
	if err := mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &dc); err != nil {
		return nil, err
	}
	defer dc.Release()
	var count uint32
	if err := dc.GetCount(&count); err != nil {
		return nil, err
	}
	for i := uint32(0); i < count; i++ {
		var mmd *wca.IMMDevice
		if err := dc.Item(i, &mmd); err != nil {
			continue
		}
		if got, _ := deviceInfo(mmd); got == id {
			return mmd, nil
		}
		mmd.Release()
	}
	return nil, ErrNoDevice
}

func parseFormat(w *wca.WAVEFORMATEX) RawFormat {
	f := RawFormat{Rate: int(w.NSamplesPerSec), Channels: int(w.NChannels), Bits: int(w.WBitsPerSample)}
	const (
		tagFloat       = 3
		tagExtensible  = 0xFFFE
		subFormatField = 24 // offset of WAVEFORMATEXTENSIBLE.SubFormat
	)
	switch w.WFormatTag {
	case tagFloat:
		f.Float = true
	case tagExtensible:
		// SubFormat is a GUID whose first dword is the legacy format tag.
		f.Float = *(*uint32)(unsafe.Add(unsafe.Pointer(w), subFormatField)) == tagFloat
	}
	return f
}

func openClient(id string, flags uint32, bufferMs int) (*client, error) {
	c := &client{}
	var err error
	if c.mmde, err = newEnumerator(); err != nil {
		return nil, err
	}
	if c.mmd, err = findEndpoint(c.mmde, id); err != nil {
		c.close()
		return nil, err
	}
	if err = c.mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &c.ac); err != nil {
		c.close()
		return nil, fmt.Errorf("activate audio client: %w", err)
	}
	var wfx *wca.WAVEFORMATEX
	if err = c.ac.GetMixFormat(&wfx); err != nil {
		c.close()
		return nil, fmt.Errorf("read mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))
	c.format = parseFormat(wfx)
	if c.format.Bits != 16 && c.format.Bits != 24 && c.format.Bits != 32 {
		c.close()
		return nil, fmt.Errorf("unsupported sample size: %d bits", c.format.Bits)
	}
	duration := wca.REFERENCE_TIME(bufferMs * hundredNsPerMs)
	if err = c.ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, flags, duration, 0, wfx, nil); err != nil {
		c.close()
		return nil, fmt.Errorf("initialise audio client: %w", err)
	}
	if err = c.ac.GetBufferSize(&c.frames); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

// ---------------------------------------------------------------- microphone

type winSink struct {
	playout *Playout
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (winBackend) OpenMic(dev Device) (Sink, error) {
	s := &winSink{
		playout: NewPlayout(48000, 60, 150),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	ready := make(chan error, 1)
	go s.run(dev.ID, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return s, nil
}

func (s *winSink) Write(pcm []byte)    { s.playout.Write(pcm) }
func (s *winSink) Stats() PlayoutStats { return s.playout.Stats() }

func (s *winSink) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

func (s *winSink) run(id string, ready chan<- error) {
	defer close(s.done)
	release, err := comInit()
	if err != nil {
		ready <- err
		return
	}
	defer release()

	c, err := openClient(id, 0, 100)
	if err != nil {
		ready <- err
		return
	}
	defer c.close()

	var arc *wca.IAudioRenderClient
	if err := c.ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		ready <- fmt.Errorf("get render client: %w", err)
		return
	}
	defer arc.Release()

	if err := c.ac.Start(); err != nil {
		ready <- fmt.Errorf("start render stream: %w", err)
		return
	}
	defer c.ac.Stop()
	ready <- nil

	var (
		res     = NewResampler(48000, c.format.Rate, 1)
		chunk   = make([]int16, 480) // 10 ms from the jitter buffer
		in      = make([]float32, 480)
		pending []float32 // mono at the device rate
		target  = uint32(c.format.Rate * renderTargetMs / 1000)
	)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
		}

		var padding uint32
		if err := c.ac.GetCurrentPadding(&padding); err != nil {
			return
		}
		if padding >= target {
			continue
		}
		want := min(target-padding, c.frames-padding)
		for uint32(len(pending)) < want {
			s.playout.Read(chunk)
			for i, v := range chunk {
				in[i] = float32(v) / 32768
			}
			pending = res.Process(pending, in)
		}

		var data *byte
		if err := arc.GetBuffer(want, &data); err != nil {
			return
		}
		Encode(unsafe.Slice(data, int(want)*c.format.BlockAlign()), pending[:want], c.format)
		if err := arc.ReleaseBuffer(want, 0); err != nil {
			return
		}
		pending = pending[:copy(pending, pending[want:])]
	}
}

// ------------------------------------------------------------------- speaker

type winSource struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func (winBackend) OpenSpeaker(dev Device, onFrame FrameFunc, onErr func(error)) (Source, error) {
	s := &winSource{stop: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go s.run(dev.ID, onFrame, onErr, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return s, nil
}

func (s *winSource) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

func (s *winSource) run(id string, onFrame FrameFunc, onErr func(error), ready chan<- error) {
	defer close(s.done)
	release, err := comInit()
	if err != nil {
		ready <- err
		return
	}
	defer release()

	c, err := openClient(id, wca.AUDCLNT_STREAMFLAGS_LOOPBACK, 200)
	if err != nil {
		ready <- err
		return
	}
	defer c.close()

	var acc *wca.IAudioCaptureClient
	if err := c.ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		ready <- fmt.Errorf("get capture client: %w", err)
		return
	}
	defer acc.Release()

	if err := c.ac.Start(); err != nil {
		ready <- fmt.Errorf("start loopback stream: %w", err)
		return
	}
	defer c.ac.Stop()
	ready <- nil

	var (
		res    = NewResampler(c.format.Rate, 48000, 2)
		framer = NewFramer(960 * 4)
		raw    []float32
		stereo []float32
		out    []float32
		pcm    []byte
		align  = c.format.BlockAlign()
	)
	fail := func(err error) {
		if onErr != nil {
			onErr(err)
		}
	}
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
		}

		for {
			var next uint32
			if err := acc.GetNextPacketSize(&next); err != nil {
				fail(fmt.Errorf("loopback: %w", err))
				return
			}
			if next == 0 {
				break
			}
			var (
				data   *byte
				frames uint32
				flags  uint32
			)
			if err := acc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
				fail(fmt.Errorf("loopback: %w", err))
				return
			}
			raw = raw[:0]
			if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
				raw = append(raw, make([]float32, int(frames)*c.format.Channels)...)
			} else {
				raw = Decode(raw, unsafe.Slice(data, int(frames)*align), c.format)
			}
			if err := acc.ReleaseBuffer(frames); err != nil {
				fail(fmt.Errorf("loopback: %w", err))
				return
			}

			stereo = ToStereo(stereo[:0], raw, c.format.Channels)
			out = res.Process(out[:0], stereo)
			pcm = FloatToS16(pcm[:0], out)
			framer.Push(pcm, func(frame []byte) { onFrame(frame) })
		}
	}
}
