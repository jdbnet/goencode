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
	if !strings.Contains(joined, "-map 0") || !strings.Contains(joined, "-c:s copy") || !strings.Contains(joined, "-c:t copy") {
		t.Errorf("expected map all streams, got %s", joined)
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
