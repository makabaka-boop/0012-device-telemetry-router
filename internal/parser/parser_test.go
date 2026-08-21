package parser

import (
	"strings"
	"testing"
	"time"
)

func now() time.Time {
	return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
}

func TestParseValidMessage(t *testing.T) {
	ts := now()
	raw := BuildMessage("DEVICE01", ts, "temperature", 25.5, "C")
	tel, err := ParseMessage(raw, ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tel.DeviceID != "DEVICE01" || tel.Metric != "temperature" || tel.Value != 25.5 || tel.Unit != "C" {
		t.Fatalf("unexpected parsed result: %+v", tel)
	}
}

func TestParseMissingField(t *testing.T) {
	_, err := ParseMessage("DEVICE01|2024-06-01T12:00:00Z|temperature|25.5", now())
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestParseExtraField(t *testing.T) {
	_, err := ParseMessage("DEVICE01|2024-06-01T12:00:00Z|temperature|25.5|C|extra|checksum", now())
	if err == nil {
		t.Fatal("expected error for extra field")
	}
}

func TestParseInvalidTimestamp(t *testing.T) {
	raw := "DEVICE01|not-a-time|temperature|25.5|C|0000"
	if _, err := ParseMessage(raw, now()); err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
}

func TestParseNonNumericValue(t *testing.T) {
	ts := now()
	body := "DEVICE01|" + ts.Format(time.RFC3339) + "|temperature|abc|C"
	raw := body + "|" + ChecksumHex(body)
	if _, err := ParseMessage(raw, ts); err == nil {
		t.Fatal("expected error for non-numeric value")
	}
}

func TestParseUnknownMetric(t *testing.T) {
	ts := now()
	body := "DEVICE01|" + ts.Format(time.RFC3339) + "|nosuch|25.5|C"
	raw := body + "|" + ChecksumHex(body)
	if _, err := ParseMessage(raw, ts); err == nil {
		t.Fatal("expected error for unknown metric")
	}
}

func TestChecksumCaseInsensitive(t *testing.T) {
	ts := now()
	body := "DEVICE01|" + ts.Format(time.RFC3339) + "|temperature|25.5|C"
	cs := ChecksumHex(body)
	raw := body + "|" + strings.ToLower(cs)
	tel, err := ParseMessage(raw, ts)
	if err != nil {
		t.Fatalf("expected case-insensitive checksum to pass: %v", err)
	}
	if tel.Checksum != cs {
		t.Fatalf("expected normalized checksum %s, got %s", cs, tel.Checksum)
	}
}

func TestChecksumMismatch(t *testing.T) {
	ts := now()
	body := "DEVICE01|" + ts.Format(time.RFC3339) + "|temperature|25.5|C"
	raw := body + "|FFFF"
	if _, err := ParseMessage(raw, ts); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestValueOutOfRange(t *testing.T) {
	ts := now()
	body := "DEVICE01|" + ts.Format(time.RFC3339) + "|temperature|9999|C"
	raw := body + "|" + ChecksumHex(body)
	if _, err := ParseMessage(raw, ts); err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestEmptyRaw(t *testing.T) {
	if _, err := ParseMessage("   ", now()); err == nil {
		t.Fatal("expected error for empty raw text")
	}
}

func TestChecksumHexStable(t *testing.T) {
	a := ChecksumHex("hello")
	b := ChecksumHex("hello")
	if a != b {
		t.Fatalf("checksum must be deterministic: %s vs %s", a, b)
	}
	if len(a) != 4 {
		t.Fatalf("checksum should be 4 hex digits")
	}
}
