// Package parser implements the fixed text protocol parsing and field
// validation as pure functions with no database dependency.
package parser

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Telemetry is a fully parsed and validated telemetry payload.
type Telemetry struct {
	DeviceID string
	TS       time.Time
	Metric   string
	Value    float64
	Unit     string
	Checksum string
}

// MetricRanges maps a whitelisted metric to its inclusive value range.
var MetricRanges = map[string][2]float64{
	"temperature": {-60, 120},
	"humidity":    {0, 100},
	"pressure":    {800, 1200},
	"voltage":     {0, 40},
	"current":     {0, 100},
	"wind_speed":  {0, 80},
}

// KnownMetric reports whether the metric is in the whitelist.
func KnownMetric(metric string) bool {
	_, ok := MetricRanges[metric]
	return ok
}

// ParseMessage splits a raw text line into fields and validates them. The
// protocol is pipe-delimited:
//
//	DEVICE_ID|TIMESTAMP|METRIC|VALUE|UNIT|CHECKSUM
//
// Timestamps are RFC3339. The checksum is CRC16-CCITT (uppercase hex)
// over the body before the final pipe-separated checksum field.
func ParseMessage(raw string, now time.Time) (*Telemetry, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("empty raw_text")
	}
	if len(raw) > 4096 {
		return nil, errors.New("raw_text too long")
	}

	body, checksum, err := splitChecksum(raw)
	if err != nil {
		return nil, err
	}

	fields := strings.Split(body, "|")
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	t := &Telemetry{
		DeviceID: fields[0],
		Metric:   strings.TrimSpace(fields[2]),
		Unit:     strings.TrimSpace(fields[4]),
		Checksum: checksum,
	}

	if err := validateDeviceID(t.DeviceID); err != nil {
		return nil, err
	}

	ts, err := time.Parse(time.RFC3339, fields[1])
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}
	if ts.After(now.Add(5 * time.Minute)) {
		return nil, fmt.Errorf("timestamp too far in future")
	}
	if ts.Before(now.Add(-30 * 24 * time.Hour)) {
		return nil, fmt.Errorf("timestamp too old")
	}
	t.TS = ts

	if !KnownMetric(t.Metric) {
		return nil, fmt.Errorf("unknown metric %q", t.Metric)
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
		return nil, fmt.Errorf("non-numeric value %q", fields[3])
	}
	if rng, ok := MetricRanges[t.Metric]; ok && (val < rng[0] || val > rng[1]) {
		return nil, fmt.Errorf("value %v out of range [%v, %v]", val, rng[0], rng[1])
	}
	t.Value = val

	expected := ChecksumHex(body)
	if strings.ToUpper(checksum) != expected {
		return nil, fmt.Errorf("checksum mismatch: got %s want %s", checksum, expected)
	}
	t.Checksum = expected

	return t, nil
}

// splitChecksum separates the checksum (last pipe-separated field) from the
// body that the checksum covers.
func splitChecksum(raw string) (body, checksum string, err error) {
	idx := strings.LastIndex(raw, "|")
	if idx < 0 {
		return "", "", errors.New("missing checksum field")
	}
	body = raw[:idx]
	checksum = raw[idx+1:]
	if checksum == "" {
		return "", "", errors.New("empty checksum")
	}
	if _, err := hex.DecodeString(strings.ToUpper(checksum)); err != nil {
		return "", "", errors.New("invalid checksum format")
	}
	return body, checksum, nil
}

func validateDeviceID(id string) error {
	if len(id) < 8 || len(id) > 32 {
		return fmt.Errorf("device_id length must be 8..32")
	}
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '-' {
			return fmt.Errorf("device_id contains invalid character %q", c)
		}
	}
	return nil
}

// ChecksumHex computes the CRC16-CCITT checksum (uppercase hex) over the
// given body using polynomial 0x1021 and initial value 0xFFFF.
func ChecksumHex(body string) string {
	crc := uint16(0xFFFF)
	for i := 0; i < len(body); i++ {
		crc ^= uint16(body[i]) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}

// BuildMessage assembles a well-formed protocol line for a telemetry
// payload so tests and clients can produce valid input.
func BuildMessage(deviceID string, ts time.Time, metric string, value float64, unit string) string {
	body := fmt.Sprintf("%s|%s|%s|%s|%s",
		deviceID, ts.Format(time.RFC3339), metric, strconv.FormatFloat(value, 'f', -1, 64), unit)
	return body + "|" + ChecksumHex(body)
}
