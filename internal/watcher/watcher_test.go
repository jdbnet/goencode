package watcher

import (
	"path/filepath"
	"testing"
)

func TestAlreadyKnown(t *testing.T) {
	if alreadyKnown("/media/a.mkv", nil) {
		t.Fatal("nil set should not treat files as known")
	}
	known := map[string]struct{}{
		"/media/a.mkv": {},
	}
	if !alreadyKnown("/media/a.mkv", known) {
		t.Fatal("expected known path")
	}
	if !alreadyKnown("/media/./a.mkv", known) {
		t.Fatal("expected cleaned path to match")
	}
	if alreadyKnown("/media/b.mkv", known) {
		t.Fatal("unknown path")
	}
}

func TestHandleEventIfIdleUsesKnownSet(t *testing.T) {
	m, err := NewManager(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		m.timersMu.Lock()
		for _, t := range m.timers {
			t.Stop()
		}
		m.timersMu.Unlock()
		m.Stop()
	}()

	known := map[string]struct{}{
		filepath.Clean("/media/done.mkv"): {},
	}
	m.handleEventIfIdle("/media/done.mkv", known)
	m.handleEventIfIdle("/media/new.mkv", known)

	m.timersMu.Lock()
	_, donePending := m.timers[filepath.Clean("/media/done.mkv")]
	_, newPending := m.timers[filepath.Clean("/media/new.mkv")]
	m.timersMu.Unlock()

	if donePending {
		t.Fatal("known file should not be scheduled")
	}
	if !newPending {
		t.Fatal("unknown file should be scheduled without a DB round-trip")
	}
}
