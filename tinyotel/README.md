# tinyotel

A toy OpenTelemetry Collector — receives OTLP telemetry (traces, metrics, logs), processes it through a configurable pipeline, and serves a query API with a web UI for trace waterfall, metrics charts, and log search.

Built as part of [tinybox](../README.md), a collection of simplified infrastructure and tooling implementations for study.

## What it teaches

| Concept | Where |
|---|---|
| OTel data model: spans, metrics, log records | `model/types.go` |
| OTLP/HTTP push model vs Prometheus scrape | `receiver/receiver.go` |
| Collector pipeline: receiver → processor → store | `processor/`, `receiver/` |
| BatchProcessor: flush by size and interval | `processor/batch.go` |
| AttributeProcessor: insert/update/delete/rename | `processor/attributes.go` |
| Consistent head-based sampling (FNV-32a per traceID) | `processor/sampling.go` |
| W3C traceparent + baggage context propagation | `propagator/propagator.go` |
| In-memory trace store with secondary indexes | `store/trace/store.go` |
| Metrics store: gauge/sum/histogram time series | `store/metrics/store.go` |
| Log store: bounded ring-buffer with severity filter | `store/logs/store.go` |
| Query API with JSON responses | `api/api.go` |
| Embedded single-page web UI (no build step) | `ui/static/index.html` |
| YAML config parser (stdlib only) | `config/config.go` |

## Architecture

```
Instrumented services
  │  POST /v1/traces    POST /v1/metrics    POST /v1/logs
  ▼
┌─────────────────────────────────────────────────────────┐
│                       tinyotel                          │
│                                                         │
│  OTLP/HTTP Receiver (:4318)                             │
│    /v1/traces  /v1/metrics  /v1/logs                    │
│         │                                               │
│  Processor Pipeline                                     │
│    Batch → Attributes → Sampling                        │
│         │                                               │
│   ┌─────┼──────────┐                                    │
│   ▼     ▼          ▼                                    │
│  Trace  Metrics   Log                                   │
│  Store  Store     Store                                 │
│   └─────┴──────────┘                                    │
│         │                                               │
│  Query API + Web UI (:4319)                             │
│    /api/v1/traces  /api/v1/metrics  /api/v1/logs        │
│    /ui/  →  trace waterfall, metrics chart, log viewer  │
└─────────────────────────────────────────────────────────┘
```

## Usage

```bash
cd tinyotel
go build ./cmd/tinyotel/

# Start with defaults (receiver :4318, API :4319)
./tinyotel

# Start with a config file
./tinyotel -config config.yaml
```

### Send test telemetry

```bash
# Send a trace span
curl -s -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"mysvc"}}]},"scopeSpans":[{"spans":[{"traceId":"aabbccddeeff00112233445566778899","spanId":"aabbccdd11223344","name":"my-op","kind":2,"startTimeUnixNano":1700000000000000000,"endTimeUnixNano":1700000001000000000,"attributes":[],"status":{"code":1}}]}]}]}'

# Send a metric
curl -s -X POST http://localhost:4318/v1/metrics \
  -H "Content-Type: application/json" \
  -d '{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"mysvc"}}]},"scopeMetrics":[{"metrics":[{"name":"cpu.usage","gauge":{"dataPoints":[{"timeUnixNano":1700000000000000000,"asDouble":0.42,"attributes":[]}]}}]}]}]}'

# Send a log record
curl -s -X POST http://localhost:4318/v1/logs \
  -H "Content-Type: application/json" \
  -d '{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"mysvc"}}]},"scopeLogs":[{"logRecords":[{"timeUnixNano":1700000000000000000,"severityText":"INFO","severityNumber":9,"body":{"stringValue":"hello world"},"attributes":[],"traceId":"aabbccddeeff00112233445566778899","spanId":""}]}]}]}'

# Query traces
curl -s http://localhost:4319/api/v1/traces | jq .

# Open web UI
open http://localhost:4319/ui/
```

### W3C Context Propagation

```go
import "github.com/krapi0314/tinybox/tinyotel/propagator"

// Extract inbound context
ctx := propagator.Extract(req.Header)
// → ctx.TraceID, ctx.SpanID, ctx.Sampled, ctx.Baggage

// Inject outbound context
propagator.Inject(ctx, outboundReq.Header)
// → sets "traceparent: 00-{traceID}-{spanID}-01" + "baggage: key=val"
```

### Config file format

```yaml
receiver:
  http_port: 4318

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

## Directory structure

```
tinyotel/
├── cmd/tinyotel/main.go        # binary entry point
├── model/types.go              # Span, Metric, LogRecord, Resource
├── receiver/receiver.go        # OTLP/HTTP ingest endpoints
├── processor/
│   ├── processor.go            # SpanProcessor interface + Chain
│   ├── batch.go                # BatchProcessor
│   ├── attributes.go           # AttributeProcessor
│   └── sampling.go             # SamplingProcessor
├── store/
│   ├── trace/store.go          # trace store (traceID index + search)
│   ├── metrics/store.go        # metrics store (name+attrs series)
│   └── logs/store.go           # log ring buffer
├── propagator/propagator.go    # W3C traceparent + baggage
├── api/api.go                  # query REST API
├── ui/static/index.html        # single-page web UI (Chart.js)
├── config/config.go            # YAML config parser
└── SPEC.md
```

## Running tests

```bash
go test ./...
```

All 9 packages have tests. All `_test.go` files were written before their implementation files (strict TDD).
