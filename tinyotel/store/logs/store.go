// Package logs implements an in-memory log store as a bounded ring buffer.
package logs

import (
	"sync"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// severityOrder maps severity text to a numeric ordering for minimum-severity filtering.
var severityOrder = map[string]int{
	"TRACE": 1,
	"DEBUG": 5,
	"INFO":  9,
	"WARN":  13,
	"ERROR": 17,
	"FATAL": 21,
}

// Query filters log records.
type Query struct {
	Service  string
	Severity string        // minimum severity (inclusive)
	TraceID  model.TraceID // exact match
	StartMs  int64
	EndMs    int64
	Limit    int
}

// Store is a bounded ring-buffer log store.
type Store struct {
	mu       sync.RWMutex
	capacity int
	buf      []model.LogRecord
	head     int // index of next write slot
	count    int // total records stored (capped at capacity)
}

// NewStore creates a LogStore with the given capacity.
func NewStore(capacity int) *Store {
	return &Store{
		capacity: capacity,
		buf:      make([]model.LogRecord, capacity),
	}
}

// Append adds a log record, evicting the oldest if at capacity.
func (s *Store) Append(r model.LogRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf[s.head] = r
	s.head = (s.head + 1) % s.capacity
	if s.count < s.capacity {
		s.count++
	}
}

// Query returns log records matching the query filters.
func (s *Store) Query(q Query) []model.LogRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	minSev := severityOrder[q.Severity]

	var out []model.LogRecord
	// Iterate in insertion order: oldest first.
	start := 0
	if s.count == s.capacity {
		start = s.head // oldest slot
	}
	for i := 0; i < s.count; i++ {
		r := s.buf[(start+i)%s.capacity]

		if q.Service != "" && r.Resource.Attributes["service.name"] != q.Service {
			continue
		}
		if q.Severity != "" && r.SeverityNum < minSev {
			continue
		}
		if q.TraceID != "" && r.TraceID != q.TraceID {
			continue
		}
		if q.StartMs != 0 && r.TimeMs < q.StartMs {
			continue
		}
		if q.EndMs != 0 && r.TimeMs > q.EndMs {
			continue
		}
		out = append(out, r)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out
}
