package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/metrics"
)

func TestStatsMiddleware_IncrementsRequestCounter(t *testing.T) {
	reg := metrics.NewRegistry()
	clusterName := "test-cluster"
	routePrefix := "/v1"

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewStats(reg, clusterName, routePrefix, inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mfs, err := reg.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "tinyenvoy_requests_total" {
			for _, m := range mf.GetMetric() {
				lbls := dtoLabelsToMap(m.GetLabel())
				if lbls["cluster"] == clusterName && lbls["route"] == routePrefix && lbls["status"] == "200" {
					if m.GetCounter().GetValue() != 1 {
						t.Errorf("counter = %v, want 1", m.GetCounter().GetValue())
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("tinyenvoy_requests_total{cluster=test-cluster, route=/v1, status=200} not found")
	}
}

func TestStatsMiddleware_RecordsDuration(t *testing.T) {
	reg := metrics.NewRegistry()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewStats(reg, "cluster", "/", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mfs, err := reg.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "tinyenvoy_request_duration_seconds" {
			for _, m := range mf.GetMetric() {
				if m.GetHistogram().GetSampleCount() >= 1 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("tinyenvoy_request_duration_seconds should have at least 1 observation")
	}
}

func TestStatsMiddleware_CountsNon200(t *testing.T) {
	reg := metrics.NewRegistry()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	handler := NewStats(reg, "cluster", "/", inner)
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mfs, err := reg.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "tinyenvoy_requests_total" {
			for _, m := range mf.GetMetric() {
				lbls := dtoLabelsToMap(m.GetLabel())
				if lbls["status"] == "500" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("should have recorded a 500 status counter")
	}
}

func TestStatsMiddleware_PassesThrough(t *testing.T) {
	reg := metrics.NewRegistry()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("ok"))
	})

	handler := NewStats(reg, "cluster", "/", inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rr.Code)
	}
}

func dtoLabelsToMap(pairs []*dto.LabelPair) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, lp := range pairs {
		m[lp.GetName()] = lp.GetValue()
	}
	return m
}
