package dedup

import (
	"testing"
	"time"
)

func TestKeyDeterministic(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	a := Key("DEVICE01", ts, "temperature", 25.5, "C")
	b := Key("DEVICE01", ts, "temperature", 25.5, "C")
	if a != b {
		t.Fatalf("keys must be deterministic: %s vs %s", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("key length should be 32, got %d", len(a))
	}
}

func TestKeyDiffers(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	a := Key("DEVICE01", ts, "temperature", 25.5, "C")
	b := Key("DEVICE01", ts, "temperature", 26.5, "C")
	if a == b {
		t.Fatal("keys should differ for different values")
	}
}

func TestWithinWindow(t *testing.T) {
	now := time.Now()
	window := 24 * time.Hour
	if !WithinWindow(now.Add(-time.Hour), now, window) {
		t.Fatal("should be within window")
	}
	if WithinWindow(now.Add(-25*time.Hour), now, window) {
		t.Fatal("should be outside window")
	}
}
