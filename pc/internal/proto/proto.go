// Package proto defines the phoneset wire protocol (v2).
// The authoritative description lives in docs/protocol.md.
package proto

const (
	Version     = 2
	ServiceType = "_phoneset._tcp"
	DefaultPort = 47831
	WSPath      = "/v2/ws"
	VideoPath   = "/v2/video" // a second connection, for the camera only (docs/protocol.md, section 8)
	InfoPath    = "/v2/info"
)

// Audio parameters are fixed in v2: 48 kHz, signed 16-bit little endian,
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
	FrameVideo   byte = 0x04
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
	TVideo  = "video"
	TVAck   = "vack"
	TVStats = "vstats"
	TVKey   = "vkey"
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
	ErrNoSession     = "no_session" // the video connection needs the phone's main connection first
	ErrVideoOff      = "video_off"
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
	Video   *VideoCaps  `json:"video,omitempty"` // nil on a PC that has no video
}

// VideoCaps tells the phone whether it may open the video connection (docs/protocol.md, section 8).
type VideoCaps struct {
	Available     bool     `json:"available"`
	Reason        string   `json:"reason,omitempty"`
	Path          string   `json:"path,omitempty"`
	MaxFrameBytes int      `json:"maxFrameBytes,omitempty"`
	Codecs        []string `json:"codecs,omitempty"`   // the codecs this PC accepts
	Features      []string `json:"features,omitempty"` // optional extras; none yet (section 8.8 reserves "probe")
}

// VideoMode is a codec, a size and a frame rate. The protocol names no tiers: 720p30 is just a mode.
type VideoMode struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	FPS    int    `json:"fps"`
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

// VideoState is sent on the video connection: by the phone to start or stop the camera (and to say
// which one), and by the PC with the state that is in effect.
type VideoState struct {
	T      string      `json:"t"`
	On     bool        `json:"on"`
	Camera string      `json:"camera,omitempty"`
	Mode   *VideoMode  `json:"mode,omitempty"`  // what the phone sends, or what is in effect
	Modes  []VideoMode `json:"modes,omitempty"` // everything the phone is willing to send (a request only)
	Error  string      `json:"error,omitempty"`
	// Features lists optional things the phone supports (a request only). A PC sends a message of a kind the
	// phone does not know only to a phone that listed it here (docs/protocol.md, section 8.6).
	Features []string `json:"features,omitempty"`
}

// FeatureKeyRequest is the video feature "keyreq": the phone answers a VideoKeyRequest with a key frame.
const FeatureKeyRequest = "keyreq"

// VideoKeyRequest asks the phone for a key frame as soon as it can (section 8.3), for a reader that has just
// joined or has lost its place. Sent only to a phone that listed FeatureKeyRequest.
type VideoKeyRequest struct {
	T string `json:"t"`
}

// VideoAck confirms that the PC has read a video frame (section 8.3).
type VideoAck struct {
	T   string `json:"t"`
	Seq uint32 `json:"seq"`
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

// VideoStats is sent on the video connection every 2 s while the camera is on.
type VideoStats struct {
	T       string `json:"t"`
	Frames  uint64 `json:"frames"`
	Bytes   uint64 `json:"bytes"`
	Viewers int    `json:"viewers"`
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

// A video message is seq (u32), capture time in ms (u32), codec (u8), flags (u8), width (u16), height
// (u16), then the coded picture.
const (
	VideoHeaderLen = 14
	// MaxVideoFrame is the largest frame a PC reads. It is announced in Ready.
	MaxVideoFrame = 4 << 20
)

// Codec names (in modes) and numbers (in the frame header). Only JPEG has a defined payload so far.
const (
	CodecJPEG = "jpeg"
	CodecH264 = "h264"
	CodecHEVC = "hevc"

	CodecByteJPEG byte = 1
	CodecByteH264 byte = 2
	CodecByteHEVC byte = 3
)

// The flags byte of a video frame (section 8.3): bit 0 marks a frame that can be decoded on its own, bits 1 and 2
// are the quarter turns clockwise the receiver applies to the decoded picture to make it upright, and bit 3 asks it
// to mirror the picture horizontally after that. The other bits are 0.
const (
	VideoFlagKey           byte = 1
	VideoFlagRotationShift      = 1
	VideoFlagRotationMask  byte = 3 << VideoFlagRotationShift
	VideoFlagMirror        byte = 8
)

// Camera facings in Stream.Camera.
const (
	CameraBack  = "back"
	CameraFront = "front"
)
