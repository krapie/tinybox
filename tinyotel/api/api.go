// Package api implements the tinyotel query HTTP API.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	logstore "github.com/krapi0314/tinybox/tinyotel/store/logs"
	metricsstore "github.com/krapi0314/tinybox/tinyotel/store/metrics"
	tracestore "github.com/krapi0314/tinybox/tinyotel/store/trace"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// Handler is the HTTP API handler for tinyotel queries.
type Handler struct {
	traces  *tracestore.Store
	metrics *metricsstore.Store
	logs    *logstore.Store
	mux     *http.ServeMux
}

// NewHandler wires all query endpoints to the given stores.
func NewHandler(ts *tracestore.Store, ms *metricsstore.Store, ls *logstore.Store) http.Handler {
	h := &Handler{traces: ts, metrics: ms, logs: ls, mux: http.NewServeMux()}
	h.mux.HandleFunc("/api/v1/traces", h.handleTraces)
	h.mux.HandleFunc("/api/v1/traces/", h.handleTraceByID)
	h.mux.HandleFunc("/api/v1/services", h.handleServices)
	h.mux.HandleFunc("/api/v1/operations", h.handleOperations)
	h.mux.HandleFunc("/api/v1/metrics", h.handleMetrics)
	h.mux.HandleFunc("/api/v1/metric-names", h.handleMetricNames)
	h.mux.HandleFunc("/api/v1/logs", h.handleLogs)
	h.mux.HandleFunc("/health", h.handleHealth)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ── Trace endpoints ──────────────────────────────────────────────────────────

// GET /api/v1/traces[?service=&operation=&minDuration=&start=&end=&limit=]
func (h *Handler) handleTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := tracestore.Query{
		Service:   q.Get("service"),
		Operation: q.Get("operation"),
		Limit:     intParam(q.Get("limit"), 100),
	}
	if v := q.Get("minDuration"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			query.MinDuration = d
		}
	}
	if v := q.Get("start"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			query.StartTime = time.UnixMilli(ms)
		}
	}
	if v := q.Get("end"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			query.EndTime = time.UnixMilli(ms)
		}
	}

	summaries := h.traces.Search(query)
	if summaries == nil {
		summaries = []tracestore.Summary{}
	}

	type respSummary struct {
		TraceID   string   `json:"traceID"`
		RootSpan  string   `json:"rootSpan"`
		SpanCount int      `json:"spanCount"`
		DurationMs int64   `json:"durationMs"`
		Services  []string `json:"services"`
		HasError  bool     `json:"hasError"`
	}
	out := make([]respSummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, respSummary{
			TraceID:    string(s.TraceID),
			RootSpan:   s.RootSpan.Name,
			SpanCount:  s.SpanCount,
			DurationMs: s.Duration.Milliseconds(),
			Services:   s.Services,
			HasError:   s.HasError,
		})
	}
	writeJSON(w, out)
}

// GET /api/v1/traces/{traceID}
func (h *Handler) handleTraceByID(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimPrefix(r.URL.Path, "/api/v1/traces/")
	if traceID == "" {
		h.handleTraces(w, r)
		return
	}

	spans, err := h.traces.GetTrace(model.TraceID(traceID))
	if err != nil {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	writeJSON(w, spans)
}

// GET /api/v1/services
func (h *Handler) handleServices(w http.ResponseWriter, r *http.Request) {
	services := h.traces.Services()
	if services == nil {
		services = []string{}
	}
	writeJSON(w, services)
}

// GET /api/v1/operations?service=
func (h *Handler) handleOperations(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	ops := h.traces.Operations(service)
	if ops == nil {
		ops = []string{}
	}
	writeJSON(w, ops)
}

// ── Metrics endpoints ────────────────────────────────────────────────────────

// GET /api/v1/metrics?name=&start=&end=
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := metricsstore.Query{
		Name: q.Get("name"),
	}
	if v := q.Get("start"); v != "" {
		query.StartMs, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("end"); v != "" {
		query.EndMs, _ = strconv.ParseInt(v, 10, 64)
	}

	series := h.metrics.Query(query)
	if series == nil {
		series = []metricsstore.Series{}
	}
	writeJSON(w, series)
}

// GET /api/v1/metric-names
func (h *Handler) handleMetricNames(w http.ResponseWriter, r *http.Request) {
	names := h.metrics.MetricNames()
	if names == nil {
		names = []string{}
	}
	writeJSON(w, names)
}

// ── Logs endpoints ───────────────────────────────────────────────────────────

// GET /api/v1/logs?service=&severity=&traceID=&start=&end=&limit=
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := logstore.Query{
		Service:  q.Get("service"),
		Severity: q.Get("severity"),
		TraceID:  model.TraceID(q.Get("traceID")),
		Limit:    intParam(q.Get("limit"), 100),
	}
	if v := q.Get("start"); v != "" {
		query.StartMs, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("end"); v != "" {
		query.EndMs, _ = strconv.ParseInt(v, 10, 64)
	}

	records := h.logs.Query(query)
	type logResp struct {
		TimeMs       int64             `json:"timeMs"`
		SeverityText string            `json:"severity"`
		Body         string            `json:"body"`
		TraceID      string            `json:"traceID,omitempty"`
		SpanID       string            `json:"spanID,omitempty"`
		Resource     map[string]string `json:"resource"`
		Attributes   map[string]string `json:"attributes,omitempty"`
	}
	out := make([]logResp, 0, len(records))
	for _, rec := range records {
		out = append(out, logResp{
			TimeMs:       rec.TimeMs,
			SeverityText: rec.SeverityText,
			Body:         rec.Body,
			TraceID:      string(rec.TraceID),
			SpanID:       string(rec.SpanID),
			Resource:     rec.Resource.Attributes,
			Attributes:   rec.Attributes,
		})
	}
	writeJSON(w, out)
}

// ── Health ───────────────────────────────────────────────────────────────────

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// ── helpers ──────────────────────────────────────────────────────────────────

func intParam(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
