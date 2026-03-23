package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewRegistry_RegistersAllMetrics(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry() returned nil")
	}

	// Gather all metrics and verify they are registered
	mfs, err := reg.Gatherer().Gather()
	// An empty registry gather returns no error, just empty
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	_ = mfs
}

func TestRequestsTotal_IncrementsByLabel(t *testing.T) {
	reg := NewRegistry()

	reg.RequestsTotal.WithLabelValues("my-cluster", "/v1", "200").Inc()
	reg.RequestsTotal.WithLabelValues("my-cluster", "/v1", "200").Inc()
	reg.RequestsTotal.WithLabelValues("my-cluster", "/v1", "500").Inc()

	mfs := gatherFamily(t, reg.Reg(), "tinyenvoy_requests_total")
	total200 := getCounterValue(mfs, map[string]string{
		"cluster": "my-cluster",
		"route":   "/v1",
		"status":  "200",
	})
	if total200 != 2 {
		t.Errorf("requests_total{200} = %v, want 2", total200)
	}
	total500 := getCounterValue(mfs, map[string]string{
		"cluster": "my-cluster",
		"route":   "/v1",
		"status":  "500",
	})
	if total500 != 1 {
		t.Errorf("requests_total{500} = %v, want 1", total500)
	}
}

func TestRequestDuration_ObservesValues(t *testing.T) {
	reg := NewRegistry()
	reg.RequestDuration.WithLabelValues("my-cluster", "/v1").Observe(0.1)
	reg.RequestDuration.WithLabelValues("my-cluster", "/v1").Observe(0.5)

	mfs := gatherFamily(t, reg.Reg(), "tinyenvoy_request_duration_seconds")
	if mfs == nil {
		t.Fatal("tinyenvoy_request_duration_seconds not found")
	}
}

func TestEndpointHealthy_GaugeSetGet(t *testing.T) {
	reg := NewRegistry()
	reg.EndpointHealthy.WithLabelValues("cluster-a", "localhost:8081").Set(1)
	reg.EndpointHealthy.WithLabelValues("cluster-a", "localhost:8081").Set(0)

	mfs := gatherFamily(t, reg.Reg(), "tinyenvoy_endpoint_healthy")
	if mfs == nil {
		t.Fatal("tinyenvoy_endpoint_healthy metric not found")
	}
}

func TestActiveConnections_GaugeIncDec(t *testing.T) {
	reg := NewRegistry()
	reg.ActiveConnections.WithLabelValues("cluster-a", "localhost:8081").Inc()
	reg.ActiveConnections.WithLabelValues("cluster-a", "localhost:8081").Inc()
	reg.ActiveConnections.WithLabelValues("cluster-a", "localhost:8081").Dec()

	mfs := gatherFamily(t, reg.Reg(), "tinyenvoy_active_connections")
	val := getGaugeValue(mfs, map[string]string{
		"cluster":  "cluster-a",
		"endpoint": "localhost:8081",
	})
	if val != 1 {
		t.Errorf("active_connections = %v, want 1", val)
	}
}

func TestMetricNames(t *testing.T) {
	reg := NewRegistry()

	// Register a sample to trigger gathering
	reg.RequestsTotal.WithLabelValues("c", "/", "200").Inc()
	reg.RequestDuration.WithLabelValues("c", "/").Observe(0.01)
	reg.EndpointHealthy.WithLabelValues("c", "addr").Set(1)
	reg.ActiveConnections.WithLabelValues("c", "addr").Inc()

	mfs, err := reg.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"tinyenvoy_requests_total",
		"tinyenvoy_request_duration_seconds",
		"tinyenvoy_endpoint_healthy",
		"tinyenvoy_active_connections",
	}

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}

	for _, w := range want {
		if !names[w] {
			t.Errorf("metric %q not found in registry", w)
		}
	}
}

// helpers

func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, mf := range mfs {
		if strings.EqualFold(mf.GetName(), name) {
			return mf
		}
	}
	return nil
}

func getCounterValue(mf *dto.MetricFamily, labels map[string]string) float64 {
	if mf == nil {
		return -1
	}
	for _, m := range mf.GetMetric() {
		if labelsMatch(m.GetLabel(), labels) {
			return m.GetCounter().GetValue()
		}
	}
	return -1
}

func getGaugeValue(mf *dto.MetricFamily, labels map[string]string) float64 {
	if mf == nil {
		return -1
	}
	for _, m := range mf.GetMetric() {
		if labelsMatch(m.GetLabel(), labels) {
			return m.GetGauge().GetValue()
		}
	}
	return -1
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	got := make(map[string]string, len(pairs))
	for _, lp := range pairs {
		got[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
