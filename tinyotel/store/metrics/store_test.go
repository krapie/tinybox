package metrics_test

import (
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/store/metrics"
)

func gaugeMetric(name string, timeMs int64, value float64, attrs map[string]string) model.Metric {
	return model.Metric{
		Name:     name,
		Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}},
		Data: model.Gauge{
			Points: []model.NumberDataPoint{{TimeMs: timeMs, Value: value, Attributes: attrs}},
		},
	}
}

func sumMetric(name string, timeMs int64, value float64) model.Metric {
	return model.Metric{
		Name:     name,
		Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}},
		Data: model.Sum{
			IsMonotonic: true,
			Points:      []model.NumberDataPoint{{TimeMs: timeMs, Value: value}},
		},
	}
}

func TestQueryByName(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(gaugeMetric("cpu.usage", 1000, 0.5, nil))
	s.Append(gaugeMetric("mem.usage", 1000, 100.0, nil))

	results := s.Query(metrics.Query{Name: "cpu.usage"})
	if len(results) != 1 {
		t.Fatalf("expected 1 series, got %d", len(results))
	}
	if results[0].Name != "cpu.usage" {
		t.Errorf("unexpected name %q", results[0].Name)
	}
}

func TestQueryReturnsDataPoints(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(gaugeMetric("cpu.usage", 1000, 0.3, nil))
	s.Append(gaugeMetric("cpu.usage", 2000, 0.7, nil))

	results := s.Query(metrics.Query{Name: "cpu.usage"})
	if len(results) == 0 {
		t.Fatal("no series returned")
	}
	if len(results[0].Points) != 2 {
		t.Errorf("expected 2 points, got %d", len(results[0].Points))
	}
}

func TestQueryTimeRange(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(gaugeMetric("cpu.usage", 1000, 0.1, nil))
	s.Append(gaugeMetric("cpu.usage", 5000, 0.5, nil))
	s.Append(gaugeMetric("cpu.usage", 9000, 0.9, nil))

	results := s.Query(metrics.Query{Name: "cpu.usage", StartMs: 2000, EndMs: 8000})
	if len(results) == 0 {
		t.Fatal("no series")
	}
	pts := results[0].Points
	if len(pts) != 1 {
		t.Errorf("expected 1 point in range, got %d", len(pts))
	}
	if pts[0].Value != 0.5 {
		t.Errorf("unexpected value %v", pts[0].Value)
	}
}

func TestQueryByAttributes(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(gaugeMetric("req.count", 1000, 10, map[string]string{"method": "GET"}))
	s.Append(gaugeMetric("req.count", 1000, 20, map[string]string{"method": "POST"}))

	results := s.Query(metrics.Query{Name: "req.count", Attributes: map[string]string{"method": "GET"}})
	if len(results) != 1 {
		t.Fatalf("expected 1 series, got %d", len(results))
	}
	if results[0].Points[0].Value != 10 {
		t.Errorf("wrong value %v", results[0].Points[0].Value)
	}
}

func TestAppendSumMetric(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(sumMetric("http.requests", 1000, 42))

	results := s.Query(metrics.Query{Name: "http.requests"})
	if len(results) != 1 {
		t.Fatalf("expected 1 series, got %d", len(results))
	}
	if results[0].Points[0].Value != 42 {
		t.Errorf("unexpected value %v", results[0].Points[0].Value)
	}
}

func TestAppendHistogramMetric(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	m := model.Metric{
		Name:     "http.latency",
		Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}},
		Data: model.Histogram{
			Points: []model.HistogramDataPoint{
				{TimeMs: 1000, Count: 10, Sum: 500.0, BucketCounts: []uint64{2, 3, 5}},
			},
		},
	}
	s.Append(m)

	hdp := s.QueryHistogram(metrics.Query{Name: "http.latency"})
	if len(hdp) == 0 {
		t.Fatal("no histogram series")
	}
	if hdp[0].Points[0].Count != 10 {
		t.Errorf("unexpected count %d", hdp[0].Points[0].Count)
	}
}

func TestRetentionEviction(t *testing.T) {
	s := metrics.NewStore(1 * time.Millisecond)
	s.Append(gaugeMetric("cpu.usage", 1, 0.5, nil))

	time.Sleep(5 * time.Millisecond)
	s.Evict()

	results := s.Query(metrics.Query{Name: "cpu.usage"})
	if len(results) > 0 && len(results[0].Points) > 0 {
		t.Error("expected evicted data points, found some")
	}
}

func TestMetricNames(t *testing.T) {
	s := metrics.NewStore(2 * time.Hour)
	s.Append(gaugeMetric("cpu.usage", 1000, 0.5, nil))
	s.Append(gaugeMetric("mem.usage", 1000, 100.0, nil))

	names := s.MetricNames()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["cpu.usage"] || !found["mem.usage"] {
		t.Errorf("expected both metric names, got %v", names)
	}
}
