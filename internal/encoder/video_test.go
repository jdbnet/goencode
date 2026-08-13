package encoder

import (
	"strings"
	"testing"
)

func TestShouldSkipVideo(t *testing.T) {
	tests := []struct {
		codec, height, targetRes, videoCodec string
		skip                                 bool
	}{
		{"hevc", "1080", "", "libx265", true},
		{"h264", "1080", "", "libx265", false},
		{"hevc", "1080", "720p", "libx265", false},
		{"hevc", "720", "1080p", "libx265", true},
		{"hevc", "1080", "", "copy", false},
		{"h264", "1080", "", "libx264", true},
		{"av1", "1080", "", "libsvtav1", true},
		{"hevc", "1080", "", "libsvtav1", false},
		{"av1", "2160", "1080p", "libsvtav1", false},
		{"av1", "720", "1080p", "libsvtav1", true},
	}
	for _, tt := range tests {
		got, _ := shouldSkipVideo(tt.codec, tt.height, tt.targetRes, tt.videoCodec)
		if got != tt.skip {
			t.Errorf("shouldSkipVideo(%q, %q, %q, %q) = %v, want %v",
				tt.codec, tt.height, tt.targetRes, tt.videoCodec, got, tt.skip)
		}
	}
}

func TestBuildVideoArgsDefaults(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{KeepExtraStreams: true})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:v libx265") {
		t.Errorf("expected libx265, got %s", joined)
	}
	if !strings.Contains(joined, "-c:a copy") {
		t.Errorf("expected audio copy, got %s", joined)
	}
	if !strings.Contains(joined, "-preset medium") {
		t.Errorf("expected medium preset, got %s", joined)
	}
	if !strings.Contains(joined, "-crf 22") {
		t.Errorf("expected crf 22, got %s", joined)
	}
	if !strings.Contains(joined, "-f matroska") {
		t.Errorf("expected matroska, got %s", joined)
	}
	if strings.Contains(joined, "-threads") {
		t.Errorf("default should not set threads, got %s", joined)
	}
}

func TestBuildVideoArgsDropsExtraStreams(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-map") {
		t.Errorf("keep extra off should not map extra streams, got %s", joined)
	}
}

func TestBuildVideoArgsCustomMapSkipsDefault(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{
		KeepExtraStreams: true,
		CustomFlags:      "-map 0:v:0 -map 0:a:0",
	})
	joined := strings.Join(args, " ")
	if strings.Count(joined, "-map") != 2 {
		t.Errorf("expected only custom maps, got %s", joined)
	}
	if strings.Contains(joined, "-c:s copy") {
		t.Errorf("custom map should skip default subtitle copy, got %s", joined)
	}
}

func TestBuildVideoArgsMP4KeepsAudioNotSubs(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mp4", VideoEncodeOptions{
		KeepExtraStreams: true,
		Container:        "mp4",
		VideoCodec:       "libx265",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-map 0:v?") || !strings.Contains(joined, "-map 0:a?") {
		t.Errorf("mp4 should map video and all audio, got %s", joined)
	}
	if strings.Contains(joined, "-c:s copy") {
		t.Errorf("mp4 should not copy mkv subs, got %s", joined)
	}
}

func TestBuildVideoArgsCopy(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mp4", VideoEncodeOptions{
		VideoCodec:       "copy",
		AudioCodec:       "aac",
		Container:        "mp4",
		CRF:              "18",
		Preset:           "slow",
		KeepExtraStreams: true,
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:v copy") {
		t.Errorf("expected copy video, got %s", joined)
	}
	if strings.Contains(joined, "-crf") || strings.Contains(joined, "-preset") {
		t.Errorf("copy should not set crf/preset, got %s", joined)
	}
	if !strings.Contains(joined, "-c:a aac") {
		t.Errorf("expected aac, got %s", joined)
	}
	if !strings.Contains(joined, "-f mp4") {
		t.Errorf("expected mp4, got %s", joined)
	}
}

func TestBuildVideoArgsAV1MP4(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mp4", VideoEncodeOptions{
		VideoCodec: "libsvtav1",
		Container:  "mp4",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-crf 35") {
		t.Errorf("expected av1 default crf 35, got %s", joined)
	}
	if !strings.Contains(joined, "-tag:v av01") {
		t.Errorf("expected av01 tag, got %s", joined)
	}
	if strings.Contains(joined, "-preset medium") {
		t.Errorf("av1 should not default to medium preset, got %s", joined)
	}
}

func TestExtractHEVCMP4Tag(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mp4", VideoEncodeOptions{
		VideoCodec: "libx265",
		Container:  "mp4",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-tag:v hvc1") {
		t.Errorf("expected hvc1 tag, got %s", joined)
	}
}

func TestFormatArgsQuotesSpaces(t *testing.T) {
	got := FormatArgs([]string{"ffmpeg", "-i", "/media/My Movie.mkv", "-metadata", "title=Hi", "out.mkv", "-y"})
	if !strings.Contains(got, `"/media/My Movie.mkv"`) {
		t.Errorf("expected quoted path, got %s", got)
	}
	if strings.Contains(got, `"ffmpeg"`) {
		t.Errorf("plain tokens should not be quoted, got %s", got)
	}
}

func TestPreviewCommandIncludesCustomFlags(t *testing.T) {
	m := NewManager("/usr/bin/ffmpeg")
	cmd := m.PreviewCommand("video", "input.mkv", "output.mkv", VideoEncodeOptions{
		KeepExtraStreams: true,
		CustomFlags:      `-metadata title="My Movie"`,
	})
	if !strings.Contains(cmd, "-c:v libx265") {
		t.Errorf("expected default codec, got %s", cmd)
	}
	if !strings.Contains(cmd, "title=My Movie") {
		t.Errorf("expected custom metadata, got %s", cmd)
	}
	if !strings.HasPrefix(cmd, "/usr/bin/ffmpeg ") {
		t.Errorf("expected binary path prefix, got %s", cmd)
	}
}

func TestPreviewCommandAudio(t *testing.T) {
	m := NewManager("ffmpeg")
	cmd := m.PreviewCommand("audio", "input.flac", "output.mp3", VideoEncodeOptions{
		AudioCodec:   "libmp3lame",
		AudioBitrate: "256k",
		CustomFlags:  "-q:a 2",
	})
	if !strings.Contains(cmd, "-c:a libmp3lame") {
		t.Errorf("expected mp3 encoder, got %s", cmd)
	}
	if !strings.Contains(cmd, "-b:a 256k") {
		t.Errorf("expected 256k, got %s", cmd)
	}
	if !strings.Contains(cmd, "-q:a 2") {
		t.Errorf("expected custom flag, got %s", cmd)
	}
}

func TestPreviewCommandScaleAssumesSource(t *testing.T) {
	m := NewManager("ffmpeg")
	cmd := m.PreviewCommand("video", "input.mkv", "output.mkv", VideoEncodeOptions{
		TargetResolution: "1080p",
		OriginalWidth:    3840,
		OriginalHeight:   2160,
	})
	if !strings.Contains(cmd, "scale=1920:1080") {
		t.Errorf("expected 4k-to-1080 scale, got %s", cmd)
	}
}

func TestBuildVideoArgsThreadsX265(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{Threads: 4})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-threads 4") {
		t.Errorf("expected -threads 4, got %s", joined)
	}
	if !strings.Contains(joined, "-x265-params pools=4") {
		t.Errorf("expected x265 pools, got %s", joined)
	}
}

func TestBuildVideoArgsThreadsSVTAV1(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{VideoCodec: "libsvtav1", Threads: 2})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-threads 2") || !strings.Contains(joined, "-svtav1-params lp=2") {
		t.Errorf("got %s", joined)
	}
}

func TestBuildVideoArgsThreadsSkippedWhenCustom(t *testing.T) {
	args := buildVideoArgs("in.mkv", "out.mkv", VideoEncodeOptions{
		Threads:     4,
		CustomFlags: "-threads 8 -x265-params pools=8",
	})
	joined := strings.Join(args, " ")
	if strings.Count(joined, "-threads") != 1 || !strings.Contains(joined, "-threads 8") {
		t.Errorf("custom -threads should win, got %s", joined)
	}
	if strings.Count(joined, "-x265-params") != 1 {
		t.Errorf("custom x265-params should win, got %s", joined)
	}
}

func TestPreviewCommandUsesManagerThreads(t *testing.T) {
	m := NewManager("ffmpeg")
	m.Threads = 3
	cmd := m.PreviewCommand("video", "input.mkv", "output.mkv", VideoEncodeOptions{})
	if !strings.Contains(cmd, "-threads 3") || !strings.Contains(cmd, "pools=3") {
		t.Errorf("preview should include manager threads, got %s", cmd)
	}
}
