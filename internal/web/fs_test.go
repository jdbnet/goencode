package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanBrowsePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	path, err := cleanBrowsePath("")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Clean(wd) {
		t.Fatalf("empty path = %q, want %q", path, filepath.Clean(wd))
	}

	path, err = cleanBrowsePath("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Clean("/tmp") {
		t.Fatalf("got %q", path)
	}
}

func TestParentPath(t *testing.T) {
	if got := parentPath("/"); got != "/" {
		t.Fatalf("parent of / = %q", got)
	}
	if got := parentPath("/tmp/foo"); got != "/tmp" {
		t.Fatalf("parent = %q", got)
	}
}
