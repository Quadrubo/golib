package etag

import (
	"encoding/base64"
	"encoding/binary"
	"time"
)

// Encode returns updateTime as an etag, truncated to microseconds and quoted
// as RFC 7232 requires.
func Encode(updateTime time.Time) string {
	var micros [8]byte
	binary.BigEndian.PutUint64(micros[:], uint64(updateTime.UnixMicro()))

	return `"` + base64.RawURLEncoding.EncodeToString(micros[:]) + `"`
}
