package requestid

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"
)

const (
	HeaderName = "X-Request-ID"
	MaxLength  = 128
)

type contextKey struct{}

var fallbackCounter atomic.Uint64

// IsValid reports whether an incoming request ID is safe to reuse.
func IsValid(value string) bool {
	if value == "" || len(value) > MaxLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return false
	}
	return true
}

// New returns a UUIDv4-compatible request ID.
func New() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		binary.BigEndian.PutUint64(raw[0:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(raw[8:16], fallbackCounter.Add(1))
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	return string(encoded[:])
}

// WithContext stores a request ID using a private context key.
func WithContext(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, contextKey{}, value)
}

// FromContext returns the request ID carried by ctx, or an empty string.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}
