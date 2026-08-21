// Package dedup computes deduplication keys and expiry for telemetry
// messages, implementing the 24-hour idempotency window.
package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// Key computes the dedup key as the first 32 hex characters of
// SHA256(device_id|ts|metric|value|unit). The ts is the numeric unix
// nanosecond value so that identical payloads always collide.
func Key(deviceID string, ts time.Time, metric string, value float64, unit string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%d|%s|%s|%s",
		deviceID, ts.UnixNano(), metric, formatValue(value), unit,
	)))
	return hex.EncodeToString(sum[:])[:32]
}

func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// WithinWindow reports whether the given received time is still inside the
// dedup window relative to now.
func WithinWindow(receivedAt, now time.Time, window time.Duration) bool {
	return now.Sub(receivedAt) <= window
}
