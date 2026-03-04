# tinyprom

A simplified Prometheus — scrapes `/metrics` endpoints, stores time series data,
supports basic PromQL-like queries, and fires threshold-based alerts.

## Goals

- Understand the Prometheus data model: metric name + labels + samples
- Understand the scrape loop and Prometheus text exposition format
- Understand TSDB basics: how time series samples are stored and queried
- Understand `rate()`, `sum()`, and `avg()` computations
- Understand alerting rules and their evaluation loop

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                       tinyprom                           │
│                                                          │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Scrape Loop                                     │   │
│  │  (scrape /metrics from configured targets)       │   │
│  └───────────────────────┬──────────────────────────┘   │
│                          │ samples                       │
│                 ┌────────▼─────────┐                    │
│                 │      TSDB        │                    │
│                 │ (in-memory time  │                    │
│                 │  series store)   │                    │
│                 └────────┬─────────┘                    │
│                          │                              │
│          ┌───────────────┼───────────────┐              │
│          │               │               │              │
│  ┌───────▼──────┐ ┌──────▼──────┐ ┌─────▼─────────┐  │
│  │  Query Engine│ │Alert Manager│ │  HTTP API +   │  │
│  │  (rate/sum/  │ │(rule eval   │ │  Web UI       │  │
│  │   avg)       │ │ loop)       │ │               │  │
│  └──────────────┘ └─────────────┘ └───────────────┘  │
└──────────────────────────────────────────────────────────┘
```

## Components

### 1. Data Model (`model/`)

```go
// A Label is a key-value pair attached to a metric.
type Label struct {
    Name  string
    Value string
}

// Labels is a sorted set of Label pairs that uniquely identify a time series.
type Labels []Label

// Sample is a single data point.
type Sample struct {
    Timestamp int64   // Unix milliseconds
    Value     float64
}

// Series is a named, labelled stream of samples.
type Series struct {
    Name    string
    Labels  Labels
    Samples []Sample
}
```

### 2. Scrape Loop (`scraper/`)

- Config: list of scrape targets `{name, url, intervalSeconds}`
- For each target on its interval:
  1. `GET {url}/metrics`
  2. Parse Prometheus text exposition format (lines like `http_requests_total{method="GET"} 42 1234567890`)
  3. Write parsed samples into the TSDB
- Handle scrape failures gracefully: record `up{job="..."}` = 0

**Exposition format parsing (subset):**
```
# HELP metric_name Description
# TYPE metric_name counter|gauge|histogram|summary
metric_name{label="value"} numeric_value [optional_timestamp]
```
Only `counter` and `gauge` types need to be supported.

### 3. TSDB (`tsdb/`)

- In-memory store: `map[SeriesKey][]Sample` where `SeriesKey = name + sorted labels`
- Append: add a new sample to a series
- Query by time range: `Select(name, labels, startMs, endMs) []Sample`
- Label matcher: `= | != | =~ (regex)`
- Retention: drop samples older than a configured retention period (default: 2h)
- No WAL or disk persistence needed for this simplified version

### 4. Query Engine (`query/`)

Supports a minimal expression language:

| Function | Description |
|----------|-------------|
| `metric_name{labels}` | Instant vector: latest sample per matching series |
| `metric_name{labels}[Xm]` | Range vector: all samples in last X minutes |
| `rate(v[Xm])` | Per-second rate of increase of a counter over X minutes |
| `sum(v)` | Sum values across all series in a vector |
| `avg(v)` | Average values across all series in a vector |
| `by(label)` | Group aggregation by a label |

Expression examples:
```
http_requests_total{method="GET"}
rate(http_requests_total[5m])
sum(rate(http_requests_total[5m])) by (job)
avg(node_memory_used_bytes)
```

The query engine does NOT need to be a full PromQL parser — a function-call API
is sufficient:

```go
engine.InstantQuery(expr string, atMs int64) ([]SeriesResult, error)
engine.RangeQuery(expr string, startMs, endMs int64, stepMs int64) ([]RangeResult, error)
```

### 5. Alert Manager (`alertmanager/`)

```go
type AlertRule struct {
    Name        string
    Expr        string        // query expression
    Threshold   float64
    Op          string        // ">" | "<" | ">=" | "<=" | "==" | "!="
    ForDuration time.Duration // must be true for this long before firing
    Labels      map[string]string
    Annotations map[string]string // e.g. "summary", "description"
}

type Alert struct {
    Rule      AlertRule
    State     AlertState // Inactive | Pending | Firing
    FiredAt   time.Time
    ResolvedAt time.Time
}
```

- Alert evaluation loop: run every N seconds (default: 15s)
- For each rule: execute the query, compare result to threshold
- State machine: `Inactive → Pending → Firing → Inactive`
  - Enter `Pending` when condition first becomes true
  - Enter `Firing` after `ForDuration` elapses
  - Re-enter `Inactive` when condition is false
- Webhook notification: POST alert payload to configured URLs when state changes

### 6. HTTP API + Web UI (`api/`, `ui/`)

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/v1/query?query=&time=` | Instant query |
| GET    | `/api/v1/query_range?query=&start=&end=&step=` | Range query |
| GET    | `/api/v1/series?match=` | List matching series |
| GET    | `/api/v1/labels` | List all label names |
| GET    | `/api/v1/alerts` | List current alerts |
| GET    | `/api/v1/targets` | List scrape targets + up/down status |
| POST   | `/api/v1/rules` | Add an alert rule |

**Web UI:**
- Single HTML page with Chart.js
- Input box for metric name → renders a line chart
- Alerts table showing active/pending alerts
- Targets table showing scrape status

### 7. Config (`config/`)

```yaml
global:
  scrape_interval: 15s
  retention: 2h

scrape_configs:
  - job_name: tinykube
    static_configs:
      - targets: ["localhost:8080"]
  - job_name: tinydns
    static_configs:
      - targets: ["localhost:9090"]

alerting:
  eval_interval: 15s
  webhook_url: "http://localhost:9999/alerts"

rules:
  - name: HighErrorRate
    expr: 'rate(http_errors_total[5m])'
    threshold: 0.1
    op: ">"
    for: 1m
    labels:
      severity: warning
    annotations:
      summary: "High error rate detected"
```

## Directory Structure

```
tinyprom/
├── cmd/tinyprom/
│   └── main.go
├── model/
│   └── types.go
├── scraper/
│   ├── scraper.go
│   ├── scraper_test.go
│   ├── parser.go           # exposition format parser
│   └── parser_test.go
├── tsdb/
│   ├── tsdb.go
│   └── tsdb_test.go
├── query/
│   ├── engine.go
│   └── engine_test.go
├── alertmanager/
│   ├── alertmanager.go
│   └── alertmanager_test.go
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

### M1 — Scraper + TSDB
- [ ] Parse Prometheus text exposition format
- [ ] Scrape loop with configurable interval
- [ ] TSDB: append and range query
- [ ] `up` metric tracking per target
- Tests written first: parser, TSDB append/query/retention

### M2 — Query Engine
- [ ] Instant vector query (latest sample)
- [ ] Range vector query
- [ ] `rate()`, `sum()`, `avg()` functions
- [ ] Label matcher (`=`, `!=`, `=~`)
- Tests written first with synthetic series data

### M3 — Alert Manager
- [ ] Rule evaluation loop
- [ ] `Inactive → Pending → Firing` state machine
- [ ] Webhook notification on state change
- Tests written first for state transitions

### M4 — HTTP API + Web UI
- [ ] All API endpoints wired to TSDB and query engine
- [ ] Alerts and targets endpoints
- [ ] Single-page web UI with live chart
- Tests written first for API handlers

## Test Strategy

- **Unit tests**: parser, TSDB, query engine, alert state machine all tested independently
- **Integration tests**: scraper → TSDB → query → alert pipeline
- **HTTP tests**: API endpoints tested with `httptest`
- All test files created before their corresponding implementation files
