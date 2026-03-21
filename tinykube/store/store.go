package store

import (
	"context"
	"strings"
	"sync"

	"github.com/krapi0314/tinybox/tinykube/logger"
)

// EventType classifies a watch event.
type EventType string

const (
	EventAdded    EventType = "Added"
	EventModified EventType = "Modified"
	EventDeleted  EventType = "Deleted"
)

// Event is emitted on a watch channel when a key changes.
type Event struct {
	Type   EventType
	Key    string
	Object interface{}
}

// Store is an in-memory key-value store with watch support.
type Store struct {
	mu       sync.RWMutex
	data     map[string]interface{}
	watchers []chan Event
	log      *logger.Logger
}

// New creates a new empty Store.
func New(log *logger.Logger) *Store {
	return &Store{
		data: make(map[string]interface{}),
		log:  log,
	}
}

// Put inserts or updates key with obj, broadcasting the appropriate event.
func (s *Store) Put(key string, obj interface{}) {
	s.mu.Lock()
	_, exists := s.data[key]
	s.data[key] = obj
	eventType := EventAdded
	if exists {
		eventType = EventModified
	}
	event := Event{Type: eventType, Key: key, Object: obj}
	watchers := s.copyWatchers()
	s.mu.Unlock()

	s.log.Debug("store: put key=%s event=%s", key, eventType)
	s.broadcast(watchers, event)
}

// Get retrieves the value for key. ok is false if the key does not exist.
func (s *Store) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	return val, ok
}

// Delete removes key from the store and broadcasts a Deleted event.
func (s *Store) Delete(key string) {
	s.mu.Lock()
	obj, exists := s.data[key]
	if !exists {
		s.mu.Unlock()
		return
	}
	delete(s.data, key)
	event := Event{Type: EventDeleted, Key: key, Object: obj}
	watchers := s.copyWatchers()
	s.mu.Unlock()

	s.log.Debug("store: deleted key=%s", key)
	s.broadcast(watchers, event)
}

// List returns all values whose key starts with prefix.
func (s *Store) List(prefix string) []interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []interface{}
	for k, v := range s.data {
		if strings.HasPrefix(k, prefix) {
			result = append(result, v)
		}
	}
	return result
}

// Watch returns a channel that receives events until ctx is cancelled.
func (s *Store) Watch(ctx context.Context) <-chan Event {
	ch := make(chan Event, 64)
	s.mu.Lock()
	s.watchers = append(s.watchers, ch)
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		for i, w := range s.watchers {
			if w == ch {
				s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	return ch
}

// copyWatchers must be called with s.mu held (write lock).
func (s *Store) copyWatchers() []chan Event {
	cp := make([]chan Event, len(s.watchers))
	copy(cp, s.watchers)
	return cp
}

// broadcast sends event to all watcher channels (non-blocking).
func (s *Store) broadcast(watchers []chan Event, event Event) {
	for _, ch := range watchers {
		select {
		case ch <- event:
		default:
			// Drop if channel is full to avoid blocking
		}
	}
}
