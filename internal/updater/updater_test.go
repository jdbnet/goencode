package updater

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		remote  string
		current string
		newer   bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		{"v1.2.3", "1.2.2", true},
		{"1.2", "1.2.0", false},
		{"1.2.0", "1.2", false},
		{"1.3", "1.2.9", true},
		{"1.2.3-rc.1", "1.2.3", false},
		{"1.2.4-rc.1", "1.2.3", true},
	}
	for _, tt := range tests {
		got, err := isNewer(tt.remote, tt.current)
		if err != nil {
			t.Fatalf("isNewer(%q, %q): %v", tt.remote, tt.current, err)
		}
		if got != tt.newer {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.remote, tt.current, got, tt.newer)
		}
	}
}

func TestParseSemverInvalid(t *testing.T) {
	for _, s := range []string{"", "dev", "abc", "1.2.3.4", "1.x.0"} {
		if _, err := parseSemver(s); err == nil {
			t.Errorf("parseSemver(%q) succeeded, want error", s)
		}
	}
}

func TestParseSHA256(t *testing.T) {
	digest := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	got, err := parseSHA256(digest)
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Errorf("got %q, want %q", got, digest)
	}

	got, err = parseSHA256(digest + "  goencode\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Errorf("got %q, want %q", got, digest)
	}

	got, err = parseSHA256(digest + " *goencode")
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Errorf("got %q, want %q", got, digest)
	}

	for _, s := range []string{"", "abc", digest[:63], digest + "0"} {
		if _, err := parseSHA256(s); err == nil {
			t.Errorf("parseSHA256(%q) succeeded, want error", s)
		}
	}
}

func TestShouldSkip(t *testing.T) {
	if !shouldSkip("1.0.0", true) {
		t.Error("disabled should skip")
	}
	if !shouldSkip("dev", false) {
		t.Error("dev version should skip")
	}
	if !shouldSkip("", false) {
		t.Error("empty version should skip")
	}

	t.Setenv("GOENCODE_NO_UPDATE", "1")
	if !shouldSkip("1.0.0", false) {
		t.Error("GOENCODE_NO_UPDATE=1 should skip")
	}
	t.Setenv("GOENCODE_NO_UPDATE", "true")
	if !shouldSkip("1.0.0", false) {
		t.Error("GOENCODE_NO_UPDATE=true should skip")
	}
	t.Setenv("GOENCODE_NO_UPDATE", "")
	if shouldSkip("1.0.0", false) && !inContainer() {
		t.Error("release version should not skip when update is enabled")
	}
}
