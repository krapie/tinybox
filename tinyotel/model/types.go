// Package model defines the core data types for tinyotel telemetry signals:
// traces (spans), metrics, and logs.
package model

// TraceID is a 32 hex-character (128-bit) trace identifier per W3C TraceContext.
type TraceID string

// SpanID is a 16 hex-character (64-bit) span identifier per W3C TraceContext.
type SpanID string

// SpanKind describes the role of a span within a distributed trace.
type SpanKind int

const (
	SpanKindUnspecified SpanKind = 0
	SpanKindInternal    SpanKind = 1
	SpanKindServer      SpanKind = 2
	SpanKindClient      SpanKind = 3
	SpanKindProducer    SpanKind = 4
	SpanKindConsumer    SpanKind = 5
)

// SpanStatus holds the final status of a span.
type SpanStatus struct {
	Code    int    // 0=Unset, 1=Ok, 2=Error
	Message string
}

// SpanEvent is a time-stamped annotation attached to a span.
type SpanEvent struct {
	TimeMs     int64
	Name       string
	Attributes map[string]string
}

// Resource describes the entity (service, host) that produced telemetry.
type Resource struct {
	Attributes map[string]string // e.g. "service.name", "host.name"
}

// Span represents a single unit of work within a distributed trace.
type Span struct {
	TraceID      TraceID
	SpanID       SpanID
	ParentSpanID SpanID // empty string for root spans
	Name         string
	Kind         SpanKind
	StartTimeMs  int64
	EndTimeMs    int64
	Attributes   map[string]string
	Events       []SpanEvent
	Status       SpanStatus
	Resource     Resource
}

// DurationMs returns the span duration in milliseconds.
func (s Span) DurationMs() int64 { return s.EndTimeMs - s.StartTimeMs }

// ServiceName returns the service.name resource attribute, or empty string.
func (s Span) ServiceName() string { return s.Resource.Attributes["service.name"] }

// IsRoot returns true if this span has no parent.
func (s Span) IsRoot() bool { return s.ParentSpanID == "" }

// HasError returns true if the span status indicates an error.
func (s Span) HasError() bool { return s.Status.Code == 2 }

// --- Metrics ---

// MetricData is implemented by Sum, Gauge, and Histogram.
type MetricData interface{ metricData() }

// Sum represents a cumulative or delta counter metric.
type Sum struct {
	IsMonotonic bool
	Points      []NumberDataPoint
}

func (Sum) metricData() {}

// Gauge represents an instantaneous value metric.
type Gauge struct {
	Points []NumberDataPoint
}

func (Gauge) metricData() {}

// Histogram represents a distribution metric.
type Histogram struct {
	Points []HistogramDataPoint
}

func (Histogram) metricData() {}

// NumberDataPoint is a single value observation for Sum or Gauge metrics.
type NumberDataPoint struct {
	TimeMs     int64
	Value      float64
	Attributes map[string]string
}

// HistogramDataPoint is a single distribution observation.
type HistogramDataPoint struct {
	TimeMs         int64
	Count          uint64
	Sum            float64
	BucketCounts   []uint64
	ExplicitBounds []float64
	Attributes     map[string]string
}

// Metric is a named measurement with a series of data points.
type Metric struct {
	Name        string
	Description string
	Unit        string
	Resource    Resource
	Data        MetricData // Sum | Gauge | Histogram
}

// --- Logs ---

// LogRecord is a single structured log entry.
type LogRecord struct {
	TimeMs       int64
	SeverityText string // TRACE | DEBUG | INFO | WARN | ERROR | FATAL
	SeverityNum  int
	Body         string
	Attributes   map[string]string
	TraceID      TraceID // optional correlation
	SpanID       SpanID  // optional correlation
	Resource     Resource
}
