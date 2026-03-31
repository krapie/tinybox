package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/api"
	logstore "github.com/krapi0314/tinybox/tinyotel/store/logs"
	metricsstore "github.com/krapi0314/tinybox/tinyotel/store/metrics"
	tracestore "github.com/krapi0314/tinybox/tinyotel/store/trace"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

func makeSpan(traceID, spanID, service, name string, startMs, endMs int64, isError bool) model.Span {
	statusCode := 1
	if isError {
		statusCode = 2
	}
	return model.Span{
		TraceID:     model.TraceID(traceID),
		SpanID:      model.SpanID(spanID),
		Name:        name,
		Kind:        model.SpanKindServer,
		StartTimeMs: startMs,
		EndTimeMs:   endMs,
		Attributes:  map[string]string{},
		Resource:    model.Resource{Attributes: map[string]string{"service.name": service}},
		Status:      model.SpanStatus{Code: statusCode},
	}
}

// ── Trace API tests ──────────────────────────────────────────────────────────

func TestGetTracesReturnsEmptyList(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp []interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("expected empty list, got %v", resp)
	}
}

func TestGetTracesReturnsStoredTraces(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	ts.Append(makeSpan("trace1", "span1", "frontend", "http-handler", 1000, 2000, false))

	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(resp))
	}
	if resp[0]["traceID"] != "trace1" {
		t.Errorf("traceID = %v", resp[0]["traceID"])
	}
}

func TestGetTracesFilterByService(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	ts.Append(makeSpan("trace1", "span1", "frontend", "handler", 1000, 2000, false))
	ts.Append(makeSpan("trace2", "span2", "backend", "db-query", 1000, 2000, false))

	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces?service=frontend", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(resp))
	}
}

func TestGetTraceByID(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	ts.Append(makeSpan("tracexyz", "span1", "svc", "op", 1000, 3000, false))

	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/tracexyz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 span, got %d", len(resp))
	}
}

func TestGetTraceByIDNotFound(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/notexist", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestGetServices(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	ts.Append(makeSpan("t1", "s1", "frontend", "op", 1000, 2000, false))
	ts.Append(makeSpan("t2", "s2", "backend", "op", 1000, 2000, false))

	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []string
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Errorf("expected 2 services, got %v", resp)
	}
}

func TestGetOperations(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	ts.Append(makeSpan("t1", "s1", "frontend", "GET /home", 1000, 2000, false))
	ts.Append(makeSpan("t2", "s2", "frontend", "GET /about", 1000, 2000, false))

	h := api.NewHandler(ts, metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations?service=frontend", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []string
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Errorf("expected 2 operations, got %v", resp)
	}
}

// ── Metrics API tests ────────────────────────────────────────────────────────

func TestGetMetrics(t *testing.T) {
	ms := metricsstore.NewStore(time.Hour)
	ms.Append(model.Metric{
		Name:     "cpu.usage",
		Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}},
		Data: model.Gauge{
			Points: []model.NumberDataPoint{{TimeMs: 1000, Value: 0.5}},
		},
	})

	h := api.NewHandler(tracestore.NewStore(time.Hour), ms, logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?name=cpu.usage", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) == 0 {
		t.Error("expected at least one series")
	}
}

func TestGetMetricNames(t *testing.T) {
	ms := metricsstore.NewStore(time.Hour)
	ms.Append(model.Metric{
		Name:     "cpu.usage",
		Resource: model.Resource{Attributes: map[string]string{}},
		Data:     model.Gauge{Points: []model.NumberDataPoint{{TimeMs: 1000, Value: 0.5}}},
	})
	ms.Append(model.Metric{
		Name:     "mem.usage",
		Resource: model.Resource{Attributes: map[string]string{}},
		Data:     model.Gauge{Points: []model.NumberDataPoint{{TimeMs: 1000, Value: 100}}},
	})

	h := api.NewHandler(tracestore.NewStore(time.Hour), ms, logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metric-names", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []string
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Errorf("expected 2 metric names, got %v", resp)
	}
}

// ── Logs API tests ───────────────────────────────────────────────────────────

func TestGetLogs(t *testing.T) {
	ls := logstore.NewStore(1000)
	ls.Append(model.LogRecord{
		TimeMs:       1000,
		SeverityText: "INFO",
		SeverityNum:  9,
		Body:         "hello",
		Resource:     model.Resource{Attributes: map[string]string{"service.name": "svc"}},
	})

	h := api.NewHandler(tracestore.NewStore(time.Hour), metricsstore.NewStore(time.Hour), ls)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Errorf("expected 1 log record, got %d", len(resp))
	}
}

func TestGetLogsFilterBySeverity(t *testing.T) {
	ls := logstore.NewStore(1000)
	ls.Append(model.LogRecord{TimeMs: 1000, SeverityText: "DEBUG", SeverityNum: 5, Body: "debug", Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}}})
	ls.Append(model.LogRecord{TimeMs: 2000, SeverityText: "ERROR", SeverityNum: 17, Body: "error", Resource: model.Resource{Attributes: map[string]string{"service.name": "svc"}}})

	h := api.NewHandler(tracestore.NewStore(time.Hour), metricsstore.NewStore(time.Hour), ls)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?severity=WARN", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) != 1 || resp[0]["body"] != "error" {
		t.Errorf("unexpected results: %v", resp)
	}
}

// ── Health endpoint ──────────────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	h := api.NewHandler(tracestore.NewStore(time.Hour), metricsstore.NewStore(time.Hour), logstore.NewStore(1000))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
