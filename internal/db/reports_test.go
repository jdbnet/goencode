package db

import (
	"testing"
	"time"
)

func TestFillReportBucketsDaily(t *testing.T) {
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	existing := []ReportBucket{
		{Day: "2026-08-11", Jobs: 2, Success: 2, SpaceSaved: 100},
	}
	got := fillReportBuckets(existing, from, to, false)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].Day != "2026-08-10" || got[0].Jobs != 0 {
		t.Errorf("first bucket = %+v", got[0])
	}
	if got[1].Day != "2026-08-11" || got[1].Jobs != 2 || got[1].SpaceSaved != 100 {
		t.Errorf("filled bucket = %+v", got[1])
	}
	if got[3].Day != "2026-08-13" {
		t.Errorf("last bucket = %+v", got[3])
	}
}

func TestFillReportBucketsMonthly(t *testing.T) {
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	existing := []ReportBucket{{Day: "2026-02-01", Jobs: 5}}
	got := fillReportBuckets(existing, from, to, true)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Day != "2026-01-01" || got[1].Day != "2026-02-01" || got[1].Jobs != 5 || got[2].Day != "2026-03-01" {
		t.Errorf("got %+v", got)
	}
}

func TestReportWindow7d(t *testing.T) {
	now := time.Date(2026, 8, 13, 15, 4, 0, 0, time.UTC)
	since, monthly, err := reportWindow("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	if monthly {
		t.Fatal("7d should be daily")
	}
	want := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	if !since.Equal(want) {
		t.Errorf("since = %v, want %v", since, want)
	}
}

func TestNormalizeReportRange(t *testing.T) {
	if NormalizeReportRange("") != "30d" {
		t.Fatal("empty should default to 30d")
	}
	if NormalizeReportRange("7d") != "7d" {
		t.Fatal("7d")
	}
	if NormalizeReportRange("ALL") != "all" {
		t.Fatal("all")
	}
}
