package receiver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/receiver"
	logstore "github.com/krapi0314/tinybox/tinyotel/store/logs"
	metricsstore "github.com/krapi0314/tinybox/tinyotel/store/metrics"
	tracestore "github.com/krapi0314/tinybox/tinyotel/store/trace"
)

// otlpTracesPayload builds a minimal OTLP/JSON traces request body.
func otlpTracesPayload(traceID, spanID, service, name string, startNano, endNano int64) []byte {
	payload := map[string]interface{}{
		"resourceSpans": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{
							"key":   "service.name",
							"value": map[string]interface{}{"stringValue": service},
						},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"spans": []interface{}{
							map[string]interface{}{
								"traceId":           traceID,
								"spanId":            spanID,
								"parentSpanId":      "",
								"name":              name,
								"kind":              2,
								"startTimeUnixNano": startNano,
								"endTimeUnixNano":   endNano,
								"attributes": []interface{}{
									map[string]interface{}{
										"key":   "http.method",
										"value": map[string]interface{}{"stringValue": "GET"},
									},
								},
								"status": map[string]interface{}{"code": 1},
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestTracesEndpointStoresSpan(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := receiver.NewHandler(ts, nil, nil)

	body := otlpTracesPayload(
		"aabbccddeeff00112233445566778899",
		"aabbccdd11223344",
		"tinykube",
		"reconcile",
		1700000000_000000000,
		1700000001_000000000,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	spans, err := ts.GetTrace("aabbccddeeff00112233445566778899")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}

	sp := spans[0]
	if sp.Name != "reconcile" {
		t.Errorf("Name = %q, want reconcile", sp.Name)
	}
	if sp.ServiceName() != "tinykube" {
		t.Errorf("service.name = %q, want tinykube", sp.ServiceName())
	}
	if sp.Attributes["http.method"] != "GET" {
		t.Errorf("http.method = %q, want GET", sp.Attributes["http.method"])
	}
	if sp.Kind != model.SpanKindServer {
		t.Errorf("Kind = %d, want %d (Server)", sp.Kind, model.SpanKindServer)
	}
	// startTimeUnixNano 1700000000_000000000 → 1700000000000 ms
	if sp.StartTimeMs != 1700000000000 {
		t.Errorf("StartTimeMs = %d, want 1700000000000", sp.StartTimeMs)
	}
}

func TestTracesEndpointPartialSuccessResponse(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := receiver.NewHandler(ts, nil, nil)

	body := otlpTracesPayload("trace1", "span1", "svc", "op", 1e9, 2e9)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["partialSuccess"]; !ok {
		t.Errorf("response missing partialSuccess field: %v", resp)
	}
}

func TestTracesEndpointBadJSON(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := receiver.NewHandler(ts, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestTracesEndpointMultipleResourceSpans(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := receiver.NewHandler(ts, nil, nil)

	// Two resource spans, different services, both in the same trace.
	payload := map[string]interface{}{
		"resourceSpans": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "frontend"}},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"spans": []interface{}{
							map[string]interface{}{
								"traceId": "trace-multi", "spanId": "span-a",
								"name": "http-handler", "kind": 2,
								"startTimeUnixNano": int64(1e9), "endTimeUnixNano": int64(2e9),
								"attributes": []interface{}{}, "status": map[string]interface{}{"code": 1},
							},
						},
					},
				},
			},
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "backend"}},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"spans": []interface{}{
							map[string]interface{}{
								"traceId": "trace-multi", "spanId": "span-b",
								"name": "db-query", "kind": 3,
								"startTimeUnixNano": int64(1.2e9), "endTimeUnixNano": int64(1.8e9),
								"attributes": []interface{}{}, "status": map[string]interface{}{"code": 1},
							},
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	spans, err := ts.GetTrace("trace-multi")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if len(spans) != 2 {
		t.Errorf("span count = %d, want 2", len(spans))
	}
}

func TestWrongMethodReturns405(t *testing.T) {
	ts := tracestore.NewStore(time.Hour)
	h := receiver.NewHandler(ts, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// ── Metrics endpoint tests ────────────────────────────────────────────────────

func otlpMetricsPayload(name string, timeNano int64, value float64) []byte {
	payload := map[string]interface{}{
		"resourceMetrics": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "mysvc"}},
					},
				},
				"scopeMetrics": []interface{}{
					map[string]interface{}{
						"metrics": []interface{}{
							map[string]interface{}{
								"name": name,
								"gauge": map[string]interface{}{
									"dataPoints": []interface{}{
										map[string]interface{}{
											"timeUnixNano": timeNano,
											"asDouble":     value,
											"attributes":   []interface{}{},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestMetricsEndpointStoresDataPoint(t *testing.T) {
	ms := metricsstore.NewStore(2 * time.Hour)
	h := receiver.NewHandler(nil, ms, nil)

	body := otlpMetricsPayload("cpu.usage", 1_000_000_000, 0.42)
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	series := ms.Query(metricsstore.Query{Name: "cpu.usage"})
	if len(series) == 0 {
		t.Fatal("no series stored")
	}
	if series[0].Points[0].Value != 0.42 {
		t.Errorf("value = %v, want 0.42", series[0].Points[0].Value)
	}
}

func TestMetricsEndpointPartialSuccess(t *testing.T) {
	ms := metricsstore.NewStore(2 * time.Hour)
	h := receiver.NewHandler(nil, ms, nil)

	body := otlpMetricsPayload("cpu.usage", 1_000_000_000, 0.5)
	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if _, ok := resp["partialSuccess"]; !ok {
		t.Errorf("response missing partialSuccess: %v", resp)
	}
}

func TestMetricsEndpointBadJSON(t *testing.T) {
	ms := metricsstore.NewStore(2 * time.Hour)
	h := receiver.NewHandler(nil, ms, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ── Logs endpoint tests ───────────────────────────────────────────────────────

func otlpLogsPayload(service, severity, body string, timeNano int64, traceID string) []byte {
	payload := map[string]interface{}{
		"resourceLogs": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": service}},
					},
				},
				"scopeLogs": []interface{}{
					map[string]interface{}{
						"logRecords": []interface{}{
							map[string]interface{}{
								"timeUnixNano":   timeNano,
								"severityText":   severity,
								"severityNumber": 9,
								"body":           map[string]interface{}{"stringValue": body},
								"attributes":     []interface{}{},
								"traceId":        traceID,
								"spanId":         "",
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestLogsEndpointStoresRecord(t *testing.T) {
	ls := logstore.NewStore(1000)
	h := receiver.NewHandler(nil, nil, ls)

	body := otlpLogsPayload("frontend", "INFO", "user logged in", 1_000_000_000, "abc123")
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	records := ls.Query(logstore.Query{})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Body != "user logged in" {
		t.Errorf("body = %q", records[0].Body)
	}
	if records[0].TraceID != "abc123" {
		t.Errorf("traceID = %q, want abc123", records[0].TraceID)
	}
}

func TestLogsEndpointPartialSuccess(t *testing.T) {
	ls := logstore.NewStore(1000)
	h := receiver.NewHandler(nil, nil, ls)

	body := otlpLogsPayload("svc", "INFO", "msg", 1_000_000_000, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if _, ok := resp["partialSuccess"]; !ok {
		t.Errorf("response missing partialSuccess: %v", resp)
	}
}

func TestLogsEndpointBadJSON(t *testing.T) {
	ls := logstore.NewStore(1000)
	h := receiver.NewHandler(nil, nil, ls)

	req := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// Ensure model import is satisfied (used via model.SpanKindServer in traces test)
var _ = model.SpanKindServer
