// Package secure implements the encrypted channel that sits on top of a
// WebSocket: an ephemeral P-256 ECDH exchange, HKDF-derived per-direction keys and
// AES-256-GCM for every message. Counters double as nonces, which is safe
// because the WebSocket is ordered and reliable. Every primitive is one the phone's
// operating system provides (CryptoKit on iOS, the Java cryptography architecture on
// Android), so the phone app carries no cryptography of its own.
package secure

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
)

const (
	pubLen       = 65 // uncompressed P-256 point: 0x04 || X || Y
	helloLen     = 1 + pubLen
	nonceLen     = 12
	maxMessage   = 1 << 16
	writeTimeout = 3 * time.Second
)

var errHello = errors.New("secure: malformed client hello")

// Channel is an authenticated-encryption layer over a WebSocket.
type Channel struct {
	ws         *websocket.Conn
	send       cipher.AEAD
	recv       cipher.AEAD
	transcript [32]byte

	sendMu sync.Mutex
	sendN  uint64
	recvN  uint64
}

// Accept performs the server side of the handshake.
func Accept(ctx context.Context, ws *websocket.Conn) (*Channel, error) {
	ws.SetReadLimit(maxMessage)

	typ, hello, err := ws.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageBinary || len(hello) != helloLen || hello[0] != proto.Version {
		return nil, errHello
	}

	curve := ecdh.P256()
	// NewPublicKey rejects anything that is not a point on the curve.
	clientPub, err := curve.NewPublicKey(hello[1:])
	if err != nil {
		return nil, errHello
	}
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(clientPub)
	if err != nil {
		return nil, err
	}
	serverPub := priv.PublicKey().Bytes()

	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := ws.Write(wctx, websocket.MessageBinary, serverPub); err != nil {
		return nil, err
	}

	h := sha256.New()
	h.Write([]byte("phoneset/v2"))
	h.Write(hello[1:])
	h.Write(serverPub)
	c := &Channel{ws: ws}
	copy(c.transcript[:], h.Sum(nil))

	okm, err := hkdf.Key(sha256.New, shared, c.transcript[:], "phoneset/v2/keys", 64)
	if err != nil {
		return nil, err
	}
	// Client-to-server key first; the server receives with it and sends with the other.
	if c.recv, err = newGCM(okm[:32]); err != nil {
		return nil, err
	}
	if c.send, err = newGCM(okm[32:]); err != nil {
		return nil, err
	}
	return c, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Transcript binds proofs to this particular handshake.
func (c *Channel) Transcript() []byte { return c.transcript[:] }

func nonce(n uint64) []byte {
	var b [nonceLen]byte
	binary.BigEndian.PutUint64(b[4:], n)
	return b[:]
}

// Send encrypts typ followed by the concatenation of parts and writes it.
func (c *Channel) Send(ctx context.Context, typ byte, parts ...[]byte) error {
	size := 1
	for _, p := range parts {
		size += len(p)
	}
	buf := make([]byte, 1, size+c.send.Overhead())
	buf[0] = typ
	for _, p := range parts {
		buf = append(buf, p...)
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	sealed := c.send.Seal(buf[:0], nonce(c.sendN), buf, c.transcript[:])
	c.sendN++
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.ws.Write(wctx, websocket.MessageBinary, sealed)
}

// Recv reads and decrypts the next message. Only one goroutine may call it.
func (c *Channel) Recv(ctx context.Context) (byte, []byte, error) {
	mt, data, err := c.ws.Read(ctx)
	if err != nil {
		return 0, nil, err
	}
	if mt != websocket.MessageBinary {
		return 0, nil, errors.New("secure: text frame on encrypted channel")
	}
	plain, err := c.recv.Open(data[:0], nonce(c.recvN), data, c.transcript[:])
	if err != nil {
		return 0, nil, fmt.Errorf("secure: decrypt: %w", err)
	}
	c.recvN++
	if len(plain) == 0 {
		return 0, nil, errors.New("secure: empty message")
	}
	return plain[0], plain[1:], nil
}

// Close closes the underlying connection with a normal status.
func (c *Channel) Close(reason string) {
	_ = c.ws.Close(websocket.StatusNormalClosure, reason)
}

// Proof computes HMAC-SHA256(key, label || transcript || parts...).
func Proof(key []byte, label string, transcript []byte, parts ...[]byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(label))
	m.Write(transcript)
	for _, p := range parts {
		m.Write(p)
	}
	return m.Sum(nil)
}

// Equal compares two MACs in constant time.
func Equal(a, b []byte) bool { return hmac.Equal(a, b) }

// CodeKey derives the HMAC key used when pairing with the 6-digit code.
func CodeKey(code string) []byte {
	sum := sha256.Sum256([]byte("phoneset/code/" + code))
	return sum[:]
}
