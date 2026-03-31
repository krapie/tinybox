// Package receiver implements the OTLP/HTTP ingest endpoints for tinyotel.
// It accepts POST /v1/traces, /v1/metrics, and /v1/logs with OTLP JSON payloads
// and writes parsed records to the respective stores.
package receiver

import (
	"encoding/json"
	"net/http"

	"github.com/krapi0314/tinybox/tinyotel/model"
	metricsstore "github.com/krapi0314/tinybox/tinyotel/store/metrics"
	logstore "github.com/krapi0314/tinybox/tinyotel/store/logs"
	tracestore "github.com/krapi0314/tinybox/tinyotel/store/trace"
)

// Handler is an http.Handler that routes OTLP ingest endpoints.
type Handler struct {
	traces  *tracestore.Store
	metrics *metricsstore.Store
	logs    *logstore.Store
}

// NewHandler creates a Handler wired to the given stores. Any store may be nil
// (that endpoint will return 501).
func NewHandler(ts *tracestore.Store, ms *metricsstore.Store, ls *logstore.Store) http.Handler {
	h := &Handler{traces: ts, metrics: ms, logs: ls}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", h.handleTraces)
	mux.HandleFunc("/v1/metrics", h.handleMetrics)
	mux.HandleFunc("/v1/logs", h.handleLogs)
	return mux
}

// ── Traces ────────────────────────────────────────────────────────────────────

func (h *Handler) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.traces == nil {
		http.Error(w, "trace store not configured", http.StatusNotImplemented)
		return
	}

	var env otlpTracesEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, rs := range env.ResourceSpans {
		res := parseResource(rs.Resource)
		for _, ss := range rs.ScopeSpans {
			for _, raw := range ss.Spans {
				sp := parseSpan(raw, res)
				h.traces.Append(sp)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": map[string]interface{}{}})
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.metrics == nil {
		http.Error(w, "metrics store not configured", http.StatusNotImplemented)
		return
	}

	var env otlpMetricsEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, rm := range env.ResourceMetrics {
		res := parseResource(rm.Resource)
		for _, sm := range rm.ScopeMetrics {
			for _, raw := range sm.Metrics {
				m := parseMetric(raw, res)
				h.metrics.Append(m)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": map[string]interface{}{}})
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.logs == nil {
		http.Error(w, "log store not configured", http.StatusNotImplemented)
		return
	}

	var env otlpLogsEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, rl := range env.ResourceLogs {
		res := parseResource(rl.Resource)
		for _, sl := range rl.ScopeLogs {
			for _, raw := range sl.LogRecords {
				lr := parseLogRecord(raw, res)
				h.logs.Append(lr)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": map[string]interface{}{}})
}

// ── OTLP JSON envelope types ──────────────────────────────────────────────────

type otlpAttribute struct {
	Key   string          `json:"key"`
	Value otlpAnyValue    `json:"value"`
}

type otlpAnyValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

func (v otlpAnyValue) String() string {
	if v.StringValue != nil {
		return *v.StringValue
	}
	return ""
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

// Traces envelope

type otlpTracesEnvelope struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource    `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
}

type otlpScopeSpans struct {
	Spans []otlpSpan `json:"spans"`
}

type otlpSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	Name              string          `json:"name"`
	Kind              int             `json:"kind"`
	StartTimeUnixNano int64           `json:"startTimeUnixNano"`
	EndTimeUnixNano   int64           `json:"endTimeUnixNano"`
	Attributes        []otlpAttribute `json:"attributes"`
	Events            []otlpSpanEvent `json:"events"`
	Status            otlpStatus      `json:"status"`
}

type otlpSpanEvent struct {
	TimeUnixNano int64           `json:"timeUnixNano"`
	Name         string          `json:"name"`
	Attributes   []otlpAttribute `json:"attributes"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Metrics envelope

type otlpMetricsEnvelope struct {
	ResourceMetrics []otlpResourceMetrics `json:"resourceMetrics"`
}

type otlpResourceMetrics struct {
	Resource     otlpResource      `json:"resource"`
	ScopeMetrics []otlpScopeMetrics `json:"scopeMetrics"`
}

type otlpScopeMetrics struct {
	Metrics []otlpMetric `json:"metrics"`
}

type otlpMetric struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Unit        string           `json:"unit"`
	Sum         *otlpSum         `json:"sum,omitempty"`
	Gauge       *otlpGauge       `json:"gauge,omitempty"`
	Histogram   *otlpHistogram   `json:"histogram,omitempty"`
}

type otlpSum struct {
	IsMonotonic            bool                  `json:"isMonotonic"`
	DataPoints             []otlpNumberDataPoint `json:"dataPoints"`
}

type otlpGauge struct {
	DataPoints []otlpNumberDataPoint `json:"dataPoints"`
}

type otlpHistogram struct {
	DataPoints []otlpHistogramDataPoint `json:"dataPoints"`
}

type otlpNumberDataPoint struct {
	TimeUnixNano int64           `json:"timeUnixNano"`
	AsDouble     float64         `json:"asDouble"`
	AsInt        int64           `json:"asInt"`
	Attributes   []otlpAttribute `json:"attributes"`
}

type otlpHistogramDataPoint struct {
	TimeUnixNano   int64           `json:"timeUnixNano"`
	Count          uint64          `json:"count"`
	Sum            float64         `json:"sum"`
	BucketCounts   []uint64        `json:"bucketCounts"`
	ExplicitBounds []float64       `json:"explicitBounds"`
	Attributes     []otlpAttribute `json:"attributes"`
}

// Logs envelope

type otlpLogsEnvelope struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource   otlpResource    `json:"resource"`
	ScopeLogs  []otlpScopeLogs `json:"scopeLogs"`
}

type otlpScopeLogs struct {
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type otlpLogRecord struct {
	TimeUnixNano         int64           `json:"timeUnixNano"`
	SeverityText         string          `json:"severityText"`
	SeverityNumber       int             `json:"severityNumber"`
	Body                 otlpAnyValue    `json:"body"`
	Attributes           []otlpAttribute `json:"attributes"`
	TraceID              string          `json:"traceId"`
	SpanID               string          `json:"spanId"`
}

// ── Parsing helpers ───────────────────────────────────────────────────────────

func parseResource(r otlpResource) model.Resource {
	attrs := make(map[string]string, len(r.Attributes))
	for _, a := range r.Attributes {
		attrs[a.Key] = a.Value.String()
	}
	return model.Resource{Attributes: attrs}
}

func parseAttrs(raw []otlpAttribute) map[string]string {
	m := make(map[string]string, len(raw))
	for _, a := range raw {
		m[a.Key] = a.Value.String()
	}
	return m
}

func nanoToMs(nano int64) int64 { return nano / 1_000_000 }

func parseSpan(raw otlpSpan, res model.Resource) model.Span {
	events := make([]model.SpanEvent, 0, len(raw.Events))
	for _, e := range raw.Events {
		events = append(events, model.SpanEvent{
			TimeMs:     nanoToMs(e.TimeUnixNano),
			Name:       e.Name,
			Attributes: parseAttrs(e.Attributes),
		})
	}
	return model.Span{
		TraceID:      model.TraceID(raw.TraceID),
		SpanID:       model.SpanID(raw.SpanID),
		ParentSpanID: model.SpanID(raw.ParentSpanID),
		Name:         raw.Name,
		Kind:         model.SpanKind(raw.Kind),
		StartTimeMs:  nanoToMs(raw.StartTimeUnixNano),
		EndTimeMs:    nanoToMs(raw.EndTimeUnixNano),
		Attributes:   parseAttrs(raw.Attributes),
		Events:       events,
		Status:       model.SpanStatus{Code: raw.Status.Code, Message: raw.Status.Message},
		Resource:     res,
	}
}

func parseMetric(raw otlpMetric, res model.Resource) model.Metric {
	m := model.Metric{
		Name:        raw.Name,
		Description: raw.Description,
		Unit:        raw.Unit,
		Resource:    res,
	}
	switch {
	case raw.Sum != nil:
		pts := make([]model.NumberDataPoint, 0, len(raw.Sum.DataPoints))
		for _, dp := range raw.Sum.DataPoints {
			pts = append(pts, model.NumberDataPoint{
				TimeMs:     nanoToMs(dp.TimeUnixNano),
				Value:      dp.AsDouble,
				Attributes: parseAttrs(dp.Attributes),
			})
		}
		m.Data = model.Sum{IsMonotonic: raw.Sum.IsMonotonic, Points: pts}
	case raw.Gauge != nil:
		pts := make([]model.NumberDataPoint, 0, len(raw.Gauge.DataPoints))
		for _, dp := range raw.Gauge.DataPoints {
			pts = append(pts, model.NumberDataPoint{
				TimeMs:     nanoToMs(dp.TimeUnixNano),
				Value:      dp.AsDouble,
				Attributes: parseAttrs(dp.Attributes),
			})
		}
		m.Data = model.Gauge{Points: pts}
	case raw.Histogram != nil:
		pts := make([]model.HistogramDataPoint, 0, len(raw.Histogram.DataPoints))
		for _, dp := range raw.Histogram.DataPoints {
			pts = append(pts, model.HistogramDataPoint{
				TimeMs:         nanoToMs(dp.TimeUnixNano),
				Count:          dp.Count,
				Sum:            dp.Sum,
				BucketCounts:   dp.BucketCounts,
				ExplicitBounds: dp.ExplicitBounds,
				Attributes:     parseAttrs(dp.Attributes),
			})
		}
		m.Data = model.Histogram{Points: pts}
	}
	return m
}

func parseLogRecord(raw otlpLogRecord, res model.Resource) model.LogRecord {
	return model.LogRecord{
		TimeMs:       nanoToMs(raw.TimeUnixNano),
		SeverityText: raw.SeverityText,
		SeverityNum:  raw.SeverityNumber,
		Body:         raw.Body.String(),
		Attributes:   parseAttrs(raw.Attributes),
		TraceID:      model.TraceID(raw.TraceID),
		SpanID:       model.SpanID(raw.SpanID),
		Resource:     res,
	}
}
