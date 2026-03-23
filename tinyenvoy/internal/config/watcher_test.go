package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_CallsCallbackOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial config
	initial := []byte("listener:\n  addr: \":8080\"\n")
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}

	called := make(chan struct{}, 1)
	w, err := NewWatcher(path, func() {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Modify the file
	updated := []byte("listener:\n  addr: \":9090\"\n")
	time.Sleep(50 * time.Millisecond) // let watcher start
	if err := os.WriteFile(path, updated, 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-called:
		// success
	case <-time.After(2 * time.Second):
		t.Error("callback not called after file change")
	}
}

func TestWatcher_CloseStopsWatcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(path, func() {})
	if err != nil {
		t.Fatal(err)
	}

	// Should not panic or block
	if err := w.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
