package db

import (
	"path/filepath"
	"testing"
	"time"

	"goencode/internal/config"
)

func setupSQLite(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goencode.db")
	if err := Init(&config.DatabaseConfig{Driver: "sqlite", Path: path}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if DB != nil {
			_ = DB.Close()
			DB = nil
		}
	})
}

func TestSQLiteFolderJobReportRoundTrip(t *testing.T) {
	setupSQLite(t)

	folder := WatchFolder{
		FolderPath: "/media/movies",
		MediaType:  "video",
		Enabled:    true,
		EncodeSettings: EncodeSettings{
			VideoCodec:       "libx265",
			KeepExtraStreams: true,
			Container:        "mkv",
		},
	}
	if err := AddWatchFolder(folder); err != nil {
		t.Fatal(err)
	}
	folders, err := GetWatchFolders()
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].FolderPath != "/media/movies" || !folders[0].Enabled {
		t.Fatalf("folders = %+v", folders)
	}

	job := folders[0].NewJob("/media/movies/Film.mkv", 1000, 1)
	job.Force = true
	if err := AddJob(job); err != nil {
		t.Fatal(err)
	}
	queued, err := IsFileQueued("/media/movies/Film.mkv")
	if err != nil || !queued {
		t.Fatalf("queued = %v err=%v", queued, err)
	}

	jobs, err := GetPendingJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pending = %+v err=%v", jobs, err)
	}
	if !jobs[0].Force {
		t.Fatal("expected force_encode to round-trip")
	}
	claimed, err := ClaimJob(jobs[0].ID)
	if err != nil || !claimed {
		t.Fatalf("claimed = %v err=%v", claimed, err)
	}

	jobs[0].ErrorMessage = ""
	if err := AddJobReport(jobs[0], "success", 400, 600, 12.5); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJob(jobs[0].ID); err != nil {
		t.Fatal(err)
	}

	reports, total, err := GetJobReports(10, 0, "success", "")
	if err != nil || total != 1 || len(reports) != 1 {
		t.Fatalf("reports total=%d n=%d err=%v", total, len(reports), err)
	}
	if reports[0].SizeSaved != 600 || reports[0].CreatedAt.IsZero() {
		t.Fatalf("report = %+v", reports[0])
	}

	stats, err := GetDashboardStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesEncoded != 1 || stats.TotalSavedSpace != 600 {
		t.Fatalf("stats = %+v", stats)
	}

	if err := SetAppConfig("queue_paused", "true"); err != nil {
		t.Fatal(err)
	}
	got, err := GetAppConfig("queue_paused")
	if err != nil || got != "true" {
		t.Fatalf("app config = %q err=%v", got, err)
	}
	if err := SetAppConfig("queue_paused", "false"); err != nil {
		t.Fatal(err)
	}
	got, err = GetAppConfig("queue_paused")
	if err != nil || got != "false" {
		t.Fatalf("app config upsert = %q err=%v", got, err)
	}

	repStats, err := GetReportStats("7d")
	if err != nil {
		t.Fatal(err)
	}
	if repStats.Totals.Success != 1 {
		t.Fatalf("report stats = %+v", repStats.Totals)
	}
	if time.Since(reports[0].CreatedAt) > time.Minute {
		t.Fatalf("created_at too old: %v", reports[0].CreatedAt)
	}
}

func TestSQLiteKnownPathsAndSkip(t *testing.T) {
	setupSQLite(t)

	job := Job{FilePath: "/media/tv/Show/ep.mkv", MediaType: "video", Status: "pending"}
	if err := AddJob(job); err != nil {
		t.Fatal(err)
	}
	if err := AddJobReport(Job{FilePath: "/media/tv/Show/old.mkv", MediaType: "video"}, "skipped", 0, 0, 0); err != nil {
		t.Fatal(err)
	}

	known, err := KnownFilePathsUnder("/media/tv")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := known["/media/tv/Show/ep.mkv"]; !ok {
		t.Fatalf("missing queued path: %#v", known)
	}
	if _, ok := known["/media/tv/Show/old.mkv"]; !ok {
		t.Fatalf("missing report path: %#v", known)
	}

	n, err := DeleteJobsUnderPath("/media/tv")
	if err != nil || n != 1 {
		t.Fatalf("deleted jobs = %d err=%v", n, err)
	}
}

func TestInitRejectsUnknownDriver(t *testing.T) {
	err := Init(&config.DatabaseConfig{Driver: "postgres"})
	if err == nil {
		t.Fatal("expected error")
	}
}
