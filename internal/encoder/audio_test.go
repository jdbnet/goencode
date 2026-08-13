package encoder

import (
	"strings"
	"testing"
)

func TestBuildAudioArgsDefaultsMP3(t *testing.T) {
	args := buildAudioArgs("in.flac", "out.mp3", AudioEncodeOptions{})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:a libmp3lame") {
		t.Errorf("expected libmp3lame, got %s", joined)
	}
	if !strings.Contains(joined, "-b:a 320k") {
		t.Errorf("expected 320k, got %s", joined)
	}
	if !strings.Contains(joined, "-f mp3") {
		t.Errorf("expected mp3 muxer, got %s", joined)
	}
	if !strings.Contains(joined, "-vn") {
		t.Errorf("expected -vn, got %s", joined)
	}
}

func TestBuildAudioArgsOpus(t *testing.T) {
	args := buildAudioArgs("in.flac", "out.opus", AudioEncodeOptions{Codec: "libopus", Bitrate: "96k"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:a libopus") || !strings.Contains(joined, "-b:a 96k") || !strings.Contains(joined, "-f opus") {
		t.Errorf("got %s", joined)
	}
}

func TestBuildAudioArgsFLACOmitsBitrate(t *testing.T) {
	args := buildAudioArgs("in.wav", "out.flac", AudioEncodeOptions{Codec: "flac"})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-b:a") {
		t.Errorf("flac should not set bitrate, got %s", joined)
	}
	if !strings.Contains(joined, "-c:a flac") {
		t.Errorf("expected flac codec, got %s", joined)
	}
}

func TestBuildAudioArgsCopy(t *testing.T) {
	args := buildAudioArgs("in.flac", "out.flac", AudioEncodeOptions{Codec: "copy"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:a copy") {
		t.Errorf("expected copy, got %s", joined)
	}
	if strings.Contains(joined, "-b:a") {
		t.Errorf("copy should not set bitrate, got %s", joined)
	}
}

func TestNormalizeAudioBitrate(t *testing.T) {
	if got := NormalizeAudioBitrate("320"); got != "320k" {
		t.Fatalf("320 -> %q", got)
	}
	if got := NormalizeAudioBitrate("192k"); got != "192k" {
		t.Fatalf("192k -> %q", got)
	}
}

func TestAudioSkipSameCodec(t *testing.T) {
	skip, _ := shouldSkipAudio("mp3", 320000, "libmp3lame", "320k")
	if !skip {
		t.Fatal("expected skip for same mp3 bitrate")
	}
	skip, _ = shouldSkipAudio("mp3", 128000, "libmp3lame", "320k")
	if skip {
		t.Fatal("should re-encode lower bitrate mp3")
	}
	skip, _ = shouldSkipAudio("flac", 0, "flac", "")
	if !skip {
		t.Fatal("expected skip for flac")
	}
	skip, _ = shouldSkipAudio("aac", 256000, "libmp3lame", "320k")
	if skip {
		t.Fatal("aac should not skip when targeting mp3")
	}
}

func TestBuildAudioArgsThreads(t *testing.T) {
	args := buildAudioArgs("in.flac", "out.mp3", AudioEncodeOptions{Threads: 2})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-threads 2") {
		t.Errorf("expected -threads 2, got %s", joined)
	}
}
