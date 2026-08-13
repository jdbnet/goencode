package db

import "testing"

func TestExtractCRFPreset(t *testing.T) {
	crf, preset, rest := extractCRFPreset("-crf 22 -preset slow")
	if crf != "22" || preset != "slow" || rest != "" {
		t.Errorf("got crf=%q preset=%q rest=%q", crf, preset, rest)
	}

	crf, preset, rest = extractCRFPreset("-map 0 -crf 18 -preset medium -tag:v hvc1")
	if crf != "18" || preset != "medium" {
		t.Errorf("got crf=%q preset=%q", crf, preset)
	}
	if rest != "-map 0 -tag:v hvc1" {
		t.Errorf("rest = %q", rest)
	}

	crf, preset, rest = extractCRFPreset("-map 0")
	if crf != "" || preset != "" || rest != "-map 0" {
		t.Errorf("got crf=%q preset=%q rest=%q", crf, preset, rest)
	}
}
