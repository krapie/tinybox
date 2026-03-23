package config

import (
	"github.com/fsnotify/fsnotify"
)

// Watcher watches a config file for changes and calls a callback on each write.
// Analogous to Envoy's xDS file-based config subscription.
type Watcher struct {
	fsw *fsnotify.Watcher
}

// NewWatcher creates a Watcher that calls onChange whenever the file at path is written.
// The watcher goroutine runs until Close() is called.
func NewWatcher(path string, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-fsw.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					onChange()
				}
			case _, ok := <-fsw.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return &Watcher{fsw: fsw}, nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	return w.fsw.Close()
}
