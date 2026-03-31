// Package trace implements an in-memory trace store indexed by traceID,
// service name, and span name with TTL-based retention eviction.
package trace

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// Query holds search parameters for trace lookup.
type Query struct {
	Service     string
	Operation   string
	MinDuration time.Duration
	MaxDuration time.Duration
	StartTime   time.Time
	EndTime     time.Time
	Tags        map[string]string
	Limit       int
}

// Summary is a lightweight view of a trace returned by Search.
type Summary struct {
	TraceID   model.TraceID
	RootSpan  model.Span
	SpanCount int
	Duration  time.Duration
	Services  []string
	HasError  bool
}

type traceEntry struct {
	spans     []model.Span
	createdAt time.Time
}

// Store is a concurrency-safe in-memory trace store.
type Store struct {
	mu        sync.RWMutex
	traces    map[model.TraceID]*traceEntry // traceID → spans
	retention time.Duration
}

// NewStore creates a Store that retains traces for the given duration.
func NewStore(retention time.Duration) *Store {
	return &Store{
		traces:    make(map[model.TraceID]*traceEntry),
		retention: retention,
	}
}

// Append adds a span to the store, creating a new trace entry if needed.
func (s *Store) Append(span model.Span) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.traces[span.TraceID]
	if !ok {
		e = &traceEntry{createdAt: time.Now()}
		s.traces[span.TraceID] = e
	}
	e.spans = append(e.spans, span)
}

// GetTrace returns all spans for traceID sorted by start time.
func (s *Store) GetTrace(traceID model.TraceID) ([]model.Span, error) {
	s.mu.RLock()
	e, ok := s.traces[traceID]
	s.mu.RUnlock()

	if !ok {
		return nil, errors.New("trace not found: " + string(traceID))
	}

	out := make([]model.Span, len(e.spans))
	copy(out, e.spans)
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTimeMs < out[j].StartTimeMs
	})
	return out, nil
}

// Search returns trace summaries matching q, up to q.Limit results.
func (s *Store) Search(q Query) []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Summary
	for _, e := range s.traces {
		sum := summarize(e.spans)
		if !matches(sum, e.spans, q) {
			continue
		}
		results = append(results, sum)
		if q.Limit > 0 && len(results) >= q.Limit {
			break
		}
	}
	return results
}

// Evict removes all traces whose creation time exceeds the retention window.
func (s *Store) Evict() {
	cutoff := time.Now().Add(-s.retention)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.traces {
		if e.createdAt.Before(cutoff) {
			delete(s.traces, id)
		}
	}
}

// Services returns all unique service names seen across stored traces.
func (s *Store) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, e := range s.traces {
		for _, sp := range e.spans {
			if svc := sp.ServiceName(); svc != "" {
				seen[svc] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for svc := range seen {
		out = append(out, svc)
	}
	sort.Strings(out)
	return out
}

// Operations returns all unique span names seen for the given service.
func (s *Store) Operations(service string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, e := range s.traces {
		for _, sp := range e.spans {
			if sp.ServiceName() == service {
				seen[sp.Name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for op := range seen {
		out = append(out, op)
	}
	sort.Strings(out)
	return out
}

// summarize builds a Summary from a slice of spans belonging to one trace.
func summarize(spans []model.Span) Summary {
	if len(spans) == 0 {
		return Summary{}
	}

	var root model.Span
	var minStart, maxEnd int64
	hasError := false
	svcSet := map[string]bool{}

	for i, sp := range spans {
		if i == 0 || sp.StartTimeMs < minStart {
			minStart = sp.StartTimeMs
		}
		if i == 0 || sp.EndTimeMs > maxEnd {
			maxEnd = sp.EndTimeMs
		}
		if sp.IsRoot() {
			root = sp
		}
		if sp.HasError() {
			hasError = true
		}
		if svc := sp.ServiceName(); svc != "" {
			svcSet[svc] = true
		}
	}
	// Fall back to first span if no explicit root found.
	if root.SpanID == "" {
		root = spans[0]
	}

	svcs := make([]string, 0, len(svcSet))
	for svc := range svcSet {
		svcs = append(svcs, svc)
	}
	sort.Strings(svcs)

	return Summary{
		TraceID:   root.TraceID,
		RootSpan:  root,
		SpanCount: len(spans),
		Duration:  time.Duration(maxEnd-minStart) * time.Millisecond,
		Services:  svcs,
		HasError:  hasError,
	}
}

// matches tests a trace summary + spans against a Query.
func matches(sum Summary, spans []model.Span, q Query) bool {
	if q.Service != "" && !containsService(spans, q.Service) {
		return false
	}
	if q.Operation != "" && !containsOperation(spans, q.Operation) {
		return false
	}
	if q.MinDuration > 0 && sum.Duration < q.MinDuration {
		return false
	}
	if q.MaxDuration > 0 && sum.Duration > q.MaxDuration {
		return false
	}
	if !q.StartTime.IsZero() && sum.RootSpan.StartTimeMs < q.StartTime.UnixMilli() {
		return false
	}
	if !q.EndTime.IsZero() && sum.RootSpan.StartTimeMs > q.EndTime.UnixMilli() {
		return false
	}
	if len(q.Tags) > 0 && !containsTags(spans, q.Tags) {
		return false
	}
	return true
}

func containsService(spans []model.Span, svc string) bool {
	for _, sp := range spans {
		if sp.ServiceName() == svc {
			return true
		}
	}
	return false
}

func containsOperation(spans []model.Span, op string) bool {
	for _, sp := range spans {
		if sp.Name == op {
			return true
		}
	}
	return false
}

func containsTags(spans []model.Span, tags map[string]string) bool {
	for _, sp := range spans {
		allMatch := true
		for k, v := range tags {
			if sp.Attributes[k] != v {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}
