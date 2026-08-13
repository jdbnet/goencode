package encoder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseResolution(t *testing.T) {
	tests := []struct {
		in      string
		w, h    int
		wantErr bool
	}{
		{"1920,1080", 1920, 1080, false},
		{"1920x1080", 1920, 1080, false},
		{"width,height\n1920,1080", 1920, 1080, false},
		{"1920,1080\nDisplay Matrix,9", 1920, 1080, false},
		{"1920\n1080", 1920, 1080, false},
		{"1920,1080\r\n", 1920, 1080, false},
		{"1920,1012,", 1920, 1012, false},
		{"[mov,mp4,m4a,3gp,3g2,mj2 @ 0x5aff5e034300] Referenced QT chapter track not found\n1920,1012,\n", 1920, 1012, false},
		{"", 0, 0, true},
		{"N/A,N/A", 0, 0, true},
		{"nope", 0, 0, true},
	}
	for _, tt := range tests {
		w, h, err := parseResolution(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseResolution(%q) = %d,%d, want error", tt.in, w, h)
			}
			continue
		}
		if err != nil || w != tt.w || h != tt.h {
			t.Errorf("parseResolution(%q) = %d,%d,%v want %d,%d", tt.in, w, h, err, tt.w, tt.h)
		}
	}
}

func TestFfprobePath(t *testing.T) {
	if got := ffprobePath("ffmpeg"); got != "ffprobe" {
		t.Errorf("ffprobePath(ffmpeg) = %q", got)
	}
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	probe := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(ffmpeg, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := ffprobePath(ffmpeg); got != "ffprobe" {
		t.Errorf("missing sibling: got %q", got)
	}
	if err := os.WriteFile(probe, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := ffprobePath(ffmpeg); got != probe {
		t.Errorf("sibling = %q want %q", got, probe)
	}
}

func TestParseResolutionErrorMentionsEmpty(t *testing.T) {
	_, _, err := parseResolution("   \n")
	if err == nil || !strings.Contains(err.Error(), "empty ffprobe") {
		t.Fatalf("err = %v", err)
	}
}
