package db

import (
	"testing"
)

func TestDownloadJobCRUD(t *testing.T) {
	setupSQLite(t)

	job := DownloadJob{
		URL:      "https://example.com/video",
		Title:    "Example",
		Filename: "example",
		DestPath: "/tmp/downloads",
		FormatID: "best",
		Mode:     "download_only",
	}
	id, err := AddDownloadJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}

	got, err := GetDownloadJobByID(int(id))
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != job.URL || got.Mode != job.Mode {
		t.Fatalf("got %#v", got)
	}

	claimed, err := ClaimDownloadJob(int(id))
	if err != nil || !claimed {
		t.Fatalf("claim: ok=%v err=%v", claimed, err)
	}

	if err := UpdateDownloadJobProgress(int(id), 50); err != nil {
		t.Fatal(err)
	}
	if err := CompleteDownloadJob(int(id), "/tmp/downloads/example.mp4", 1234); err != nil {
		t.Fatal(err)
	}

	got, err = GetDownloadJobByID(int(id))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.FilePath == "" || got.FileSize != 1234 {
		t.Fatalf("got %#v", got)
	}

	jobs, err := GetDownloadJobs(10)
	if err != nil || len(jobs) == 0 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
}
