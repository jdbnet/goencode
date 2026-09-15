package downloader

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"My Video Title", "My Video Title"},
		{"  bad/name:here?  ", "badnamehere"},
		{"", "download"},
		{"....", "download"},
		{"valid-name.mp4", "valid-name.mp4"},
	}
	for _, tc := range tests {
		got := SanitizeFilename(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseProgress(t *testing.T) {
	if got := parseProgress("no progress here"); got >= 0 {
		t.Fatalf("expected -1, got %v", got)
	}
	if got := parseProgress("[download]  45.2% of 10.00MiB"); got != 45.2 {
		t.Fatalf("got %v, want 45.2", got)
	}
	if got := parseProgress("100%"); got != 100 {
		t.Fatalf("got %v, want 100", got)
	}
}

func TestCookieArgs(t *testing.T) {
	dir := t.TempDir()
	cookies := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookies, []byte("# Netscape HTTP Cookie File\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client := NewClientWithOptions(ClientOptions{
		BinaryPath:  "yt-dlp",
		CookiesFile: cookies,
	})
	if !client.CookiesConfigured() {
		t.Fatal("expected cookies configured")
	}
	args, err := client.appendCookieArgs([]string{"yt-dlp", "--no-playlist", "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 5 || args[2] != "--cookies" || args[3] != cookies || args[4] != "https://example.com" {
		t.Fatalf("args = %#v", args)
	}

	browserClient := NewClientWithOptions(ClientOptions{
		BinaryPath:         "yt-dlp",
		CookiesFromBrowser: "chrome",
	})
	if !browserClient.CookiesConfigured() {
		t.Fatal("expected browser cookies configured")
	}
	browserArgs, err := browserClient.appendCookieArgs([]string{"yt-dlp", "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if browserArgs[1] != "--cookies-from-browser" || browserArgs[2] != "chrome" {
		t.Fatalf("browser args = %#v", browserArgs)
	}
}

func TestCookieArgsMissingFile(t *testing.T) {
	client := NewClientWithOptions(ClientOptions{
		BinaryPath:  "yt-dlp",
		CookiesFile: "/does/not/exist/cookies.txt",
	})
	if client.CookiesConfigured() {
		t.Fatal("expected cookies not configured when file missing")
	}
	_, err := client.appendCookieArgs([]string{"yt-dlp", "https://example.com"})
	if err == nil {
		t.Fatal("expected error for missing cookies file")
	}
}

func TestProbeResultFromJSON(t *testing.T) {
	raw := `{
		"_type": "video",
		"title": "Example Video",
		"duration": 120.5,
		"thumbnail": "https://example.com/thumb.jpg",
		"uploader": "Example Channel",
		"extractor": "generic",
		"ext": "mp4",
		"formats": [
			{"format_id": "22", "ext": "mp4", "resolution": "720p", "vcodec": "avc1", "acodec": "mp4a"}
		]
	}`
	var info ytdlpInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatal(err)
	}
	if info.Type != "video" {
		t.Fatalf("type = %q", info.Type)
	}
	if info.Title != "Example Video" {
		t.Fatalf("title = %q", info.Title)
	}
	formats := filterFormats(info.Formats)
	if len(formats) != 1 || formats[0].ID != "22" {
		t.Fatalf("formats = %#v", formats)
	}
	presets := buildPresets(formats)
	if len(presets) < 5 {
		t.Fatalf("expected presets, got %d", len(presets))
	}
}

func TestProbeRejectsPlaylist(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "yt-dlp")
	body := `#!/bin/sh
echo '{"_type":"playlist","title":"Playlist"}'
`
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}

	client := NewClient(script)
	result, err := client.Probe(context.Background(), "https://example.com/list")
	if err == nil {
		t.Fatalf("expected error, got result %#v", result)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestDownloadWithMockYTDLP(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(dir, "yt-dlp")
	body := `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
	dest="${out%.%(ext)s}.mp4"
	if [ "$dest" = "$out" ]; then
		dest="$out"
	fi
mkdir -p "$(dirname "$dest")"
echo "[download] 100% of 1.00KiB" >&2
: > "$dest"
echo "$dest"
`
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}

	client := NewClient(script)
	path, err := client.Download(context.Background(), DownloadOptions{
		URL:       "https://example.com/video",
		FormatID:  "best",
		OutputDir: outDir,
		Filename:  "test-video",
		ProgressFn: func(pct float64) {
			if pct != 100 {
				t.Logf("progress %v", pct)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file missing: %v", err)
	}
}

func TestClientAvailable(t *testing.T) {
	client := NewClient("yt-dlp-that-does-not-exist")
	if client.Available() {
		t.Fatal("expected unavailable client")
	}
	if path, err := exec.LookPath("sh"); err == nil {
		c := NewClient(path)
		if !c.Available() {
			t.Fatalf("expected %q to be available", path)
		}
	}
}
