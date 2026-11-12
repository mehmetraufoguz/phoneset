package control

import (
	"encoding/binary"
	"net/http"
	"time"

	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
	"github.com/mehmetraufoguz/phoneset/pc/internal/video"
)

// FrameRecordLen is the length of the header of one record of /video.frames.
const FrameRecordLen = 18

// frameRecord is the header of one record of /video.frames (docs/protocol.md, section 9): the frame's
// header as the phone sent it (8.3) with the payload's length added, so a reader that is not a browser
// gets what the picture is (codec, size, key frame) and how to turn it upright (rotation, mirror), which
// the raw streams cannot carry.
//
//	seq u32 | captured u32 | codec u8 | flags u8 | width u16 | height u16 | length u32 | payload
//
// All numbers are big endian; the flags are the phone's (bit 0 key frame, bits 1 and 2 the quarter turns
// clockwise, bit 3 mirror).
func frameRecord(f *video.Frame) [FrameRecordLen]byte {
	var h [FrameRecordLen]byte
	binary.BigEndian.PutUint32(h[0:4], f.Seq)
	binary.BigEndian.PutUint32(h[4:8], f.CapturedMs)
	h[8] = f.Codec
	var flags byte
	if f.Key {
		flags |= proto.VideoFlagKey
	}
	flags |= byte(f.Rotation&3) << proto.VideoFlagRotationShift
	if f.Mirror {
		flags |= proto.VideoFlagMirror
	}
	h[9] = flags
	binary.BigEndian.PutUint16(h[10:12], uint16(f.Width))
	binary.BigEndian.PutUint16(h[12:14], uint16(f.Height))
	binary.BigEndian.PutUint32(h[14:18], uint32(len(f.Data)))
	return h
}

// keyGate lets a reader of an h264 stream start at a key frame and, when it missed a frame (the hub hands a
// slow reader only the newest), wait for the next key frame again, so a decoder never sees a gap in the
// middle of a sequence of frames that depend on each other.
type keyGate struct {
	next   uint32
	synced bool
}

// pass reports whether the reader may be given f. askKey is called when the reader has lost its place.
func (g *keyGate) pass(f *video.Frame, askKey func()) bool {
	if g.synced && f.Seq != g.next {
		g.synced = false
		askKey()
	}
	if !g.synced {
		if !f.Key {
			return false
		}
		g.synced = true
	}
	g.next = f.Seq + 1
	return true
}

// frames streams the camera as records (see frameRecord), for readers that decode it themselves: the
// virtual camera on the PC. A JPEG camera is passed on frame by frame, an h264 one from a key frame on.
func (a *api) frames(w http.ResponseWriter, r *http.Request) {
	sub := a.h.Video().Subscribe()
	defer sub.Close()
	w.Header().Set("Content-Type", "application/x-phoneset-frames")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rc := http.NewResponseController(w)
	if rc.Flush() != nil {
		return
	}
	var gate keyGate
	// A phone that supports it is asked for a key frame now, so that this reader does not wait for the next one.
	a.h.Video().RequestKey()
	for {
		select {
		case <-r.Context().Done():
			return
		case f := <-sub.C:
			if f.Codec == proto.CodecByteH264 && !gate.pass(f, a.h.Video().RequestKey) {
				continue
			}
			// A reader that stopped reading must not hold the connection forever.
			_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			head := frameRecord(f)
			if _, err := w.Write(head[:]); err != nil {
				return
			}
			if _, err := w.Write(f.Data); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}
