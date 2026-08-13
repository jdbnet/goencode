package queue

import (
	"strings"
	"testing"
)

func TestTempSpaceNeeded(t *testing.T) {
	const gb = 1024 * 1024 * 1024
	need := tempSpaceNeeded(50*gb, 5*gb)
	want := int64(105) * gb
	if need != want {
		t.Fatalf("tempSpaceNeeded = %d, want %d", need, want)
	}
}

func TestEnsureDiskSpaceRejectsSmallTmp(t *testing.T) {
	const gb = 1024 * 1024 * 1024
	old := lookupFree
	t.Cleanup(func() { lookupFree = old })
	lookupFree = func(path string) (int64, error) {
		return 2 * gb, nil
	}
	err := ensureDiskSpace("/tmp", tempSpaceNeeded(50*gb, 5*gb), "temp")
	if err == nil {
		t.Fatal("expected abort copying 50 GB onto 2 GB tmp")
	}
	if !strings.Contains(err.Error(), "temp filesystem") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureDiskSpaceAllowsFit(t *testing.T) {
	const gb = 1024 * 1024 * 1024
	old := lookupFree
	t.Cleanup(func() { lookupFree = old })
	lookupFree = func(path string) (int64, error) {
		return 200 * gb, nil
	}
	if err := ensureDiskSpace("/tmp", tempSpaceNeeded(50*gb, 5*gb), "temp"); err != nil {
		t.Fatal(err)
	}
}
