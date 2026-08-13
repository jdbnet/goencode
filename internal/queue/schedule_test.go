package queue

import (
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
	h, m, ok := parseClock("01:00")
	if !ok || h != 1 || m != 0 {
		t.Fatalf("01:00 -> %d:%d ok=%v", h, m, ok)
	}
	h, m, ok = parseClock("8:30:00")
	if !ok || h != 8 || m != 30 {
		t.Fatalf("8:30:00 -> %d:%d ok=%v", h, m, ok)
	}
	if _, _, ok := parseClock(""); ok {
		t.Fatal("empty should fail")
	}
	if _, _, ok := parseClock("25:00"); ok {
		t.Fatal("25:00 should fail")
	}
}

func TestInEncodeWindowSameDay(t *testing.T) {
	now := time.Date(2026, 8, 13, 3, 0, 0, 0, time.UTC)
	if !inEncodeWindow(now, "01:00", "08:00") {
		t.Fatal("03:00 should be inside 01:00-08:00")
	}
	now = time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	if inEncodeWindow(now, "01:00", "08:00") {
		t.Fatal("08:00 should be outside (end exclusive)")
	}
	now = time.Date(2026, 8, 13, 0, 59, 0, 0, time.UTC)
	if inEncodeWindow(now, "01:00", "08:00") {
		t.Fatal("00:59 should be outside")
	}
}

func TestInEncodeWindowOvernight(t *testing.T) {
	now := time.Date(2026, 8, 13, 23, 0, 0, 0, time.UTC)
	if !inEncodeWindow(now, "22:00", "06:00") {
		t.Fatal("23:00 should be inside 22:00-06:00")
	}
	now = time.Date(2026, 8, 13, 5, 59, 0, 0, time.UTC)
	if !inEncodeWindow(now, "22:00", "06:00") {
		t.Fatal("05:59 should be inside")
	}
	now = time.Date(2026, 8, 13, 6, 0, 0, 0, time.UTC)
	if inEncodeWindow(now, "22:00", "06:00") {
		t.Fatal("06:00 should be outside")
	}
	now = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	if inEncodeWindow(now, "22:00", "06:00") {
		t.Fatal("12:00 should be outside")
	}
}

func TestInEncodeWindowEmptyAlwaysOpen(t *testing.T) {
	now := time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)
	if !inEncodeWindow(now, "", "") {
		t.Fatal("empty window should always allow")
	}
}
