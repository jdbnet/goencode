package db

import "testing"

func TestApplyAudioDefaultsLegacyMP3(t *testing.T) {
	e := EncodeSettings{AudioCodec: "copy", Container: "mkv"}
	e.ApplyDefaultsFor("audio")
	if e.AudioCodec != "libmp3lame" || e.Container != "mp3" || e.AudioBitrate != "320k" {
		t.Fatalf("legacy audio defaults: %+v", e)
	}
}

func TestApplyAudioDefaultsOpus(t *testing.T) {
	e := EncodeSettings{AudioCodec: "libopus", AudioBitrate: "96k"}
	e.ApplyDefaultsFor("audio")
	if e.Container != "opus" {
		t.Fatalf("container = %q", e.Container)
	}
	if e.AudioBitrate != "96k" {
		t.Fatalf("bitrate = %q", e.AudioBitrate)
	}
}

func TestOutputExtAudio(t *testing.T) {
	e := EncodeSettings{AudioCodec: "aac", Container: "m4a"}
	if got := e.OutputExt("audio"); got != ".m4a" {
		t.Fatalf("got %q", got)
	}
	copyKeep := EncodeSettings{AudioCodec: "copy"}
	copyKeep.ApplyDefaultsFor("audio")
	if copyKeep.AudioCodec != "copy" {
		t.Fatalf("explicit copy should stay copy, got %+v", copyKeep)
	}
	if got := copyKeep.OutputExt("audio"); got != "" {
		t.Fatalf("copy ext = %q, want empty (keep source)", got)
	}
	flac := EncodeSettings{AudioCodec: "flac"}
	flac.ApplyDefaultsFor("audio")
	if got := flac.OutputExt("audio"); got != ".flac" {
		t.Fatalf("flac ext = %q", got)
	}
}

func TestOutputExtVideoUnchanged(t *testing.T) {
	e := EncodeSettings{Container: "mp4"}
	if got := e.OutputExt("video"); got != ".mp4" {
		t.Fatalf("got %q", got)
	}
}
