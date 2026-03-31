// Package metrics implements an in-memory metrics store.
package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// Query filters metric series by name, attributes, and time range.
type Query struct {
	Name       string
	Attributes map[string]string
	StartMs    int64
	EndMs      int64
}

// Series holds data points for a single (metric, attributes) combination.
type Series struct {
	Name       string
	Attributes map[string]string
	Points     []model.NumberDataPoint
}

// HistogramSeries holds histogram data points.
type HistogramSeries struct {
	Name       string
	Attributes map[string]string
	Points     []model.HistogramDataPoint
}

type seriesKey struct {
	name  string
	attrs string // sorted "k=v,k2=v2"
}

// Store holds metric data points grouped by metric name and attributes.
type Store struct {
	mu        sync.RWMutex
	retention time.Duration

	gauge     map[seriesKey]*Series
	histogram map[seriesKey]*HistogramSeries
}

// NewStore creates a MetricsStore with the given retention window.
func NewStore(retention time.Duration) *Store {
	return &Store{
		retention: retention,
		gauge:     make(map[seriesKey]*Series),
		histogram: make(map[seriesKey]*HistogramSeries),
	}
}

// Append stores all data points in m.
func (s *Store) Append(m model.Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch data := m.Data.(type) {
	case model.Sum:
		for _, pt := range data.Points {
			k := makeKey(m.Name, pt.Attributes)
			if _, ok := s.gauge[k]; !ok {
				s.gauge[k] = &Series{Name: m.Name, Attributes: pt.Attributes}
			}
			s.gauge[k].Points = append(s.gauge[k].Points, pt)
		}
	case model.Gauge:
		for _, pt := range data.Points {
			k := makeKey(m.Name, pt.Attributes)
			if _, ok := s.gauge[k]; !ok {
				s.gauge[k] = &Series{Name: m.Name, Attributes: pt.Attributes}
			}
			s.gauge[k].Points = append(s.gauge[k].Points, pt)
		}
	case model.Histogram:
		for _, pt := range data.Points {
			k := makeKey(m.Name, pt.Attributes)
			if _, ok := s.histogram[k]; !ok {
				s.histogram[k] = &HistogramSeries{Name: m.Name, Attributes: pt.Attributes}
			}
			s.histogram[k].Points = append(s.histogram[k].Points, pt)
		}
	}
}

// Query returns matching gauge/sum series.
func (s *Store) Query(q Query) []Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Series
	for _, ser := range s.gauge {
		if q.Name != "" && ser.Name != q.Name {
			continue
		}
		if !attrsMatch(ser.Attributes, q.Attributes) {
			continue
		}
		pts := filterPoints(ser.Points, q.StartMs, q.EndMs)
		out = append(out, Series{Name: ser.Name, Attributes: ser.Attributes, Points: pts})
	}
	return out
}

// QueryHistogram returns matching histogram series.
func (s *Store) QueryHistogram(q Query) []HistogramSeries {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []HistogramSeries
	for _, ser := range s.histogram {
		if q.Name != "" && ser.Name != q.Name {
			continue
		}
		if !attrsMatch(ser.Attributes, q.Attributes) {
			continue
		}
		pts := filterHistPoints(ser.Points, q.StartMs, q.EndMs)
		out = append(out, HistogramSeries{Name: ser.Name, Attributes: ser.Attributes, Points: pts})
	}
	return out
}

// Evict removes data points older than the retention window.
func (s *Store) Evict() {
	cutoff := time.Now().Add(-s.retention).UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, ser := range s.gauge {
		kept := ser.Points[:0]
		for _, pt := range ser.Points {
			if pt.TimeMs >= cutoff {
				kept = append(kept, pt)
			}
		}
		ser.Points = kept
		if len(ser.Points) == 0 {
			delete(s.gauge, k)
		}
	}
	for k, ser := range s.histogram {
		kept := ser.Points[:0]
		for _, pt := range ser.Points {
			if pt.TimeMs >= cutoff {
				kept = append(kept, pt)
			}
		}
		ser.Points = kept
		if len(ser.Points) == 0 {
			delete(s.histogram, k)
		}
	}
}

// MetricNames returns all observed metric names (sorted).
func (s *Store) MetricNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, ser := range s.gauge {
		seen[ser.Name] = struct{}{}
	}
	for _, ser := range s.histogram {
		seen[ser.Name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// makeKey builds a deterministic map key from metric name and attributes.
func makeKey(name string, attrs map[string]string) seriesKey {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, k...)
		b = append(b, '=')
		b = append(b, attrs[k]...)
	}
	return seriesKey{name: name, attrs: string(b)}
}

func attrsMatch(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func filterPoints(pts []model.NumberDataPoint, startMs, endMs int64) []model.NumberDataPoint {
	if startMs == 0 && endMs == 0 {
		return pts
	}
	var out []model.NumberDataPoint
	for _, pt := range pts {
		if startMs != 0 && pt.TimeMs < startMs {
			continue
		}
		if endMs != 0 && pt.TimeMs > endMs {
			continue
		}
		out = append(out, pt)
	}
	return out
}

func filterHistPoints(pts []model.HistogramDataPoint, startMs, endMs int64) []model.HistogramDataPoint {
	if startMs == 0 && endMs == 0 {
		return pts
	}
	var out []model.HistogramDataPoint
	for _, pt := range pts {
		if startMs != 0 && pt.TimeMs < startMs {
			continue
		}
		if endMs != 0 && pt.TimeMs > endMs {
			continue
		}
		out = append(out, pt)
	}
	return out
}
