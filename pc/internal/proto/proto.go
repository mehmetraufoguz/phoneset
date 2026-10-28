// Package proto defines the phoneset wire protocol (v1).
// The authoritative description lives in docs/protocol.md.
package proto

const (
	Version     = 1
	ServiceType = "_phoneset._tcp"
	DefaultPort = 47831
	WSPath      = "/v1/ws"
	InfoPath    = "/v1/info"
)

// Audio parameters are fixed in v1: 48 kHz, signed 16-bit little endian,
// 20 ms frames. The phone's microphone is mono, the PC's output is stereo.
const (
	SampleRate      = 48000
	FrameMs         = 20
	FrameSamples    = SampleRate * FrameMs / 1000 // per channel
	MicChannels     = 1
	SpeakerChannels = 2
)

// Frame types: the first plaintext byte of every message in the encrypted channel.
const (
	FrameControl byte = 0x01
	FrameMic     byte = 0x02
	FrameSpeaker byte = 0x03
)

// Control message tags.
const (
	TPair   = "pair"
	TPaired = "paired"
	TAuth   = "auth"
	TAuthed = "authed"
	TReady  = "ready"
	TStream = "stream"
	TPing   = "ping"
	TPong   = "pong"
	TStats  = "stats"
	TError  = "error"
	TBye    = "bye"
)

// Pairing methods.
const (
	MethodQR   = "qr"
	MethodCode = "code"
)

// Error codes sent in Error.Code.
const (
	ErrBadProof      = "bad_proof"
	ErrUnknownDevice = "unknown_device"
	ErrNoPairing     = "no_pairing"
	ErrRateLimited   = "rate_limited"
	ErrBusy          = "busy"
	ErrProtocol      = "protocol"
)

// Info is served unauthenticated on InfoPath so a phone can tell which of a PC's
// addresses is reachable and whether it is the PC it expects.
type Info struct {
	App  string `json:"app"`
	V    int    `json:"v"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Envelope struct {
	T string `json:"t"`
}

type Pair struct {
	T        string `json:"t"`
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Method   string `json:"method"`
	Proof    string `json:"proof"`
}

type Paired struct {
	T      string `json:"t"`
	PCID   string `json:"pcId"`
	PCName string `json:"pcName"`
	Token  string `json:"token"`
	Proof  string `json:"proof"`
}

type Auth struct {
	T        string `json:"t"`
	DeviceID string `json:"deviceId"`
	Proof    string `json:"proof"`
}

type Authed struct {
	T      string `json:"t"`
	PCID   string `json:"pcId"`
	PCName string `json:"pcName"`
	Proof  string `json:"proof"`
}

type Endpoint struct {
	Available bool   `json:"available"`
	Device    string `json:"device,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type AudioFormat struct {
	Rate            int `json:"rate"`
	FrameMs         int `json:"frameMs"`
	MicChannels     int `json:"micChannels"`
	SpeakerChannels int `json:"speakerChannels"`
}

type Ready struct {
	T       string      `json:"t"`
	Audio   AudioFormat `json:"audio"`
	Mic     Endpoint    `json:"mic"`
	Speaker Endpoint    `json:"speaker"`
}

// Stream is sent by the phone to request streams and echoed by the PC with the
// effective state. Pointers keep "unchanged" distinct from "false" in requests.
type Stream struct {
	T          string `json:"t"`
	Mic        *bool  `json:"mic,omitempty"`
	Speaker    *bool  `json:"speaker,omitempty"`
	MicError   string `json:"micError,omitempty"`
	SpeakerErr string `json:"speakerError,omitempty"`
}

type Ping struct {
	T string  `json:"t"`
	N uint32  `json:"n"`
	C float64 `json:"c"`
}

type MicStats struct {
	BufferMs  int    `json:"bufferMs"`
	Underruns uint64 `json:"underruns"`
	Drops     uint64 `json:"drops"`
}

type SpeakerStats struct {
	Sent    uint64 `json:"sent"`
	Dropped uint64 `json:"dropped"`
}

type Stats struct {
	T       string       `json:"t"`
	Mic     MicStats     `json:"mic"`
	Speaker SpeakerStats `json:"speaker"`
}

type Error struct {
	T       string `json:"t"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Bye struct {
	T      string `json:"t"`
	Reason string `json:"reason,omitempty"`
}

// AudioHeaderLen is the length of the sequence number that prefixes PCM data.
const AudioHeaderLen = 4
