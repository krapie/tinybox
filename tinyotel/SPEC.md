# tinyotel

A simplified OpenTelemetry Collector — receives OTLP telemetry (traces, metrics, logs)
from instrumented services, processes it through a pipeline, and serves a query API
with a web UI for trace waterfall, metrics charts, and log search.

## Goals

- Understand the OTel data model: spans, traces, metrics data points, log records
- Understand the OTLP push model vs. the Prometheus scrape model
- Understand distributed trace context propagation (W3C `traceparent`)
- Understand the collector pipeline: receiver → processor → exporter
- Understand how traces, metrics, and logs correlate via traceID/spanID

## Architecture

```
Instrumented services (tinykube, tinyenvoy, …)
  │  POST /v1/traces    POST /v1/metrics    POST /v1/logs
  ▼
┌─────────────────────────────────────────────────────────┐
│                       tinyotel                          │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │            OTLP/HTTP Receiver  (receiver/)        │  │
│  │    /v1/traces   /v1/metrics   /v1/logs            │  │
│  └──────────────────────┬────────────────────────────┘  │
│                         │                               │
│  ┌──────────────────────▼────────────────────────────┐  │
│  │           Processor Pipeline  (processor/)        │  │
│  │     BatchProcessor → AttributeProcessor →         │  │
│  │     SamplingProcessor                             │  │
│  └──────────────────────┬────────────────────────────┘  │
│                         │                               │
│       ┌─────────────────┼──────────────────┐            │
│       ▼                 ▼                  ▼            │
│  ┌──────────┐   ┌──────────────┐   ┌────────────┐      │
│  │  Trace   │   │   Metrics    │   │    Log     │      │
│  │  Store   │   │   Store      │   │   Store    │      │
│  └────┬─────┘   └──────┬───────┘   └─────┬──────┘      │
│       └────────────────┼─────────────────┘              │
│                        ▼                                │
│  ┌─────────────────────────────────────────────────┐   │
│  │          Query API + Web UI  (api/, ui/)         │   │
│  │   trace waterfall · span search · metrics chart  │   │
│  │   log viewer with traceID correlation            │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Components

### 1. Data Model (`model/`)

```go
// TraceID and SpanID are 16-byte and 8-byte hex strings per W3C spec.
type TraceID string // 32 hex chars
type SpanID  string // 16 hex chars

// Resource describes the entity that produced the telemetry.
type Resource struct {
    Attributes map[string]string // e.g. "service.name", "host.name"
}

// Span represents a single unit of work within a distributed trace.
type Span struct {
    TraceID      TraceID
    SpanID       SpanID
    ParentSpanID SpanID // empty for root spans
    Name         string
    Kind         SpanKind // Server | Client | Producer | Consumer | Internal
    StartTimeMs  int64
    EndTimeMs    int64
    Attributes   map[string]string
    Events       []SpanEvent
    Status       SpanStatus // Unset | Ok | Error
    Resource     Resource
}

type SpanEvent struct {
    TimeMs     int64
    Name       string
    Attributes map[string]string
}

type SpanKind  int
type SpanStatus struct { Code int; Message string }

// Metric is a named measurement with a series of data points.
type Metric struct {
    Name        string
    Description string
    Unit        string
    Resource    Resource
    Data        MetricData // one of: Sum, Gauge, Histogram
}

type Sum struct {
    IsMonotonic bool
    Points      []NumberDataPoint
}

type Gauge struct {
    Points []NumberDataPoint
}

type Histogram struct {
    Points []HistogramDataPoint
}

type NumberDataPoint struct {
    TimeMs     int64
    Value      float64
    Attributes map[string]string
}

type HistogramDataPoint struct {
    TimeMs         int64
    Count          uint64
    Sum            float64
    BucketCounts   []uint64
    ExplicitBounds []float64
    Attributes     map[string]string
}

// LogRecord is a single structured log line.
type LogRecord struct {
    TimeMs     int64
    SeverityText string // TRACE | DEBUG | INFO | WARN | ERROR | FATAL
    SeverityNum  int
    Body         string
    Attributes   map[string]string
    TraceID      TraceID // optional correlation
    SpanID       SpanID  // optional correlation
    Resource     Resource
}
```

### 2. OTLP/HTTP Receiver (`receiver/`)

Accepts OTLP JSON payloads on three endpoints:

| Method | Path | Content-Type |
|--------|------|-------------|
| POST | `/v1/traces` | `application/json` |
| POST | `/v1/metrics` | `application/json` |
| POST | `/v1/logs` | `application/json` |

- Parse the OTLP JSON envelope: `resourceSpans[]` / `resourceMetrics[]` / `resourceLogs[]`
- Extract `Resource` attributes from each `resourceSpans.resource.attributes[]`
- Respond `200 OK` with `{"partialSuccess": {}}` on success
- On parse error: respond `400` with error message
- Pass parsed data to the processor pipeline

OTLP JSON envelope (traces example):
```json
{
  "resourceSpans": [{
    "resource": { "attributes": [{"key": "service.name", "value": {"stringValue": "tinykube"}}] },
    "scopeSpans": [{
      "spans": [{
        "traceId": "abc123...",
        "spanId": "def456...",
        "parentSpanId": "",
        "name": "reconcile",
        "kind": 1,
        "startTimeUnixNano": "1700000000000000000",
        "endTimeUnixNano":   "1700000001000000000",
        "attributes": [{"key": "k8s.deployment.name", "value": {"stringValue": "nginx"}}],
        "status": {"code": 1}
      }]
    }]
  }]
}
```

### 3. Processor Pipeline (`processor/`)

Processors are chained: each receives a batch and returns a (possibly modified) batch.

```go
type SpanProcessor interface {
    Process(spans []model.Span) []model.Span
}
```

Three built-in processors:

**BatchProcessor**
- Accumulates spans up to `maxSize` (default: 512) or `flushInterval` (default: 5s)
- Flushes to the next processor/store when either threshold is reached
- Decouples high-throughput ingestion from store writes

**AttributeProcessor**
- Config: list of `{action, key, value}` rules
- Actions: `insert` (add if absent), `update` (overwrite if present), `delete`, `rename`
- Applied to span attributes and resource attributes

```go
type AttributeRule struct {
    Action string // "insert" | "update" | "delete" | "rename"
    Key    string
    Value  string // for insert/update
    NewKey string // for rename
}
```

**SamplingProcessor**
- Head-based probability sampling: keep each trace with probability P (0.0–1.0)
- Decision is per-trace (consistent for all spans sharing a traceID)
- Config: `samplingRate float64`

### 4. Trace Store (`store/trace/`)

In-memory store indexed for fast lookup:

```go
type TraceStore struct {
    // primary: traceID → []Span
    // secondary indexes: service name, span name, status, time range
}

func (s *TraceStore) Append(span model.Span)
func (s *TraceStore) GetTrace(traceID model.TraceID) ([]model.Span, error)
func (s *TraceStore) Search(q TraceQuery) []TraceSummary
```

```go
type TraceQuery struct {
    Service     string        // filter by resource service.name
    Operation   string        // filter by span name
    MinDuration time.Duration // filter by trace duration
    MaxDuration time.Duration
    StartTime   time.Time
    EndTime     time.Time
    Tags        map[string]string // match span attributes
    Limit       int
}

type TraceSummary struct {
    TraceID     model.TraceID
    RootSpan    model.Span
    SpanCount   int
    Duration    time.Duration
    Services    []string // all service.name values in trace
    HasError    bool
}
```

- Retention: drop traces older than configured duration (default: 1h)
- No disk persistence needed

### 5. Metrics Store (`store/metrics/`)

Stores metric data points grouped by metric name + resource + attributes:

```go
type MetricsStore struct{}

func (s *MetricsStore) Append(m model.Metric)
func (s *MetricsStore) Query(q MetricQuery) []MetricSeries
```

```go
type MetricQuery struct {
    Name       string
    Attributes map[string]string // label filter
    StartMs    int64
    EndMs      int64
}

type MetricSeries struct {
    Name       string
    Attributes map[string]string
    Points     []model.NumberDataPoint
}
```

- For Sum and Gauge metrics, store `NumberDataPoint` per (metric, attributes) key
- For Histogram metrics, store `HistogramDataPoint` per key
- Retention: drop data points older than configured duration (default: 2h)

### 6. Log Store (`store/logs/`)

Append-only bounded ring buffer:

```go
type LogStore struct{}

func (s *LogStore) Append(r model.LogRecord)
func (s *LogStore) Query(q LogQuery) []model.LogRecord
```

```go
type LogQuery struct {
    Service   string
    Severity  string // minimum severity level
    TraceID   model.TraceID
    StartMs   int64
    EndMs     int64
    Limit     int
}
```

- Max capacity: 100,000 records (oldest evicted when full)
- Query supports filtering by service, minimum severity, and traceID correlation

### 7. Context Propagator (`propagator/`)

Implements W3C TraceContext and Baggage header parsing — the mechanism by which
trace context crosses process boundaries.

```go
// Extract parses inbound headers into a SpanContext.
func Extract(headers http.Header) SpanContext

// Inject writes a SpanContext into outbound headers.
func Inject(ctx SpanContext, headers http.Header)

type SpanContext struct {
    TraceID    model.TraceID
    SpanID     model.SpanID
    Sampled    bool
    Baggage    map[string]string
}
```

**`traceparent` header format:**
```
traceparent: 00-{traceID}-{spanID}-{flags}
  version:  00
  traceID:  32 hex chars (128-bit)
  spanID:   16 hex chars (64-bit)
  flags:    01 = sampled, 00 = not sampled
```

**`tracestate` header:** key=value pairs for vendor-specific trace state.

**`baggage` header:** `key=value,key2=value2` pairs propagated alongside the trace.

### 8. Query API (`api/`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/traces?service=&operation=&minDuration=&start=&end=&limit=` | Search traces, returns `[]TraceSummary` |
| GET | `/api/v1/traces/{traceID}` | Full trace: all spans sorted by start time |
| GET | `/api/v1/services` | List all observed service names |
| GET | `/api/v1/operations?service=` | List span names for a service |
| GET | `/api/v1/metrics?name=&start=&end=` | Query metric time series |
| GET | `/api/v1/metric-names` | List all observed metric names |
| GET | `/api/v1/logs?service=&severity=&traceID=&start=&end=&limit=` | Query log records |

All responses are JSON. All timestamps in query params are Unix milliseconds.

### 9. Web UI (`ui/`)

Single HTML page (no build step required):

- **Trace Search**: form with service/operation/duration filters → table of matching traces
- **Trace Waterfall**: click a trace → Gantt-style span timeline showing parent→child nesting, span attributes panel on click
- **Metrics Dashboard**: select metric name → line chart of data points over time (Chart.js)
- **Log Viewer**: filterable log stream; clicking a traceID opens its waterfall

### 10. Config (`config/`)

```yaml
receiver:
  http_port: 4318          # standard OTLP/HTTP port

pipeline:
  processors:
    - type: batch
      max_size: 512
      flush_interval: 5s
    - type: attributes
      rules:
        - action: insert
          key: tinyotel.version
          value: "0.1"
    - type: sampling
      sampling_rate: 1.0   # 1.0 = keep all, 0.1 = 10%

storage:
  trace_retention: 1h
  metrics_retention: 2h
  log_max_records: 100000

api:
  http_port: 4319
```

## Directory Structure

```
tinyotel/
├── cmd/tinyotel/
│   └── main.go
├── model/
│   └── types.go
├── receiver/
│   ├── receiver.go
│   └── receiver_test.go
├── processor/
│   ├── processor.go           # Processor interface + chain
│   ├── processor_test.go
│   ├── batch.go
│   ├── batch_test.go
│   ├── attributes.go
│   ├── attributes_test.go
│   ├── sampling.go
│   └── sampling_test.go
├── store/
│   ├── trace/
│   │   ├── store.go
│   │   └── store_test.go
│   ├── metrics/
│   │   ├── store.go
│   │   └── store_test.go
│   └── logs/
│       ├── store.go
│       └── store_test.go
├── propagator/
│   ├── propagator.go
│   └── propagator_test.go
├── api/
│   ├── api.go
│   └── api_test.go
├── ui/
│   └── static/
│       └── index.html
├── config/
│   ├── config.go
│   └── config_test.go
├── go.mod
└── SPEC.md
```

## Milestones

### M1 — Data Model + OTLP Receiver + Trace Store
- [x] Define all model types (`Span`, `Metric`, `LogRecord`, `Resource`)
- [x] OTLP/HTTP receiver: parse `resourceSpans` JSON, extract spans + resource
- [x] Trace store: append, `GetTrace`, `Search` with time-range + service filter
- [x] Retention eviction on trace store
- Tests written first: model construction, OTLP JSON parsing, store append/search/retention

### M2 — Processor Pipeline + Context Propagator
- [x] `Processor` interface and chain execution
- [x] `BatchProcessor`: accumulate + flush by size and interval
- [x] `AttributeProcessor`: insert/update/delete/rename rules
- [x] `SamplingProcessor`: consistent per-trace probability sampling
- [x] W3C `traceparent` extract and inject
- [x] W3C `baggage` extract and inject
- Tests written first: each processor independently, propagator round-trips

### M3 — Metrics + Log Receiver + Stores
- [x] OTLP receiver: parse `resourceMetrics` (Sum, Gauge, Histogram)
- [x] Metrics store: append data points, query by name + attributes + time range
- [x] OTLP receiver: parse `resourceLogs`
- [x] Log store: ring buffer append, query by service/severity/traceID
- Tests written first: metrics store append/query/retention, log store query/eviction

### M4 — Query API + Web UI
- [x] All API endpoints wired to stores
- [x] Trace search and waterfall endpoints
- [x] Metrics and log query endpoints
- [x] Web UI: trace search table, waterfall view, metrics chart, log viewer
- Tests written first for all API handlers with `httptest`

## Test Strategy

- **Unit tests**: each processor, each store, propagator — all tested independently
- **Integration tests**: receiver → processor → store → query pipeline end-to-end
- **HTTP tests**: all API endpoints tested with `httptest`
- All `_test.go` files created before their corresponding implementation files

## Dependencies

- Standard library only (`net/http`, `encoding/json`, `sync`, `time`, `regexp`)
- One UI dependency: Chart.js (CDN, no local build step)
- No protobuf library needed — OTLP JSON format is sufficient for study purposes
