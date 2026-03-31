package trace_test

import (
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/store/trace"
)

func makeSpan(traceID, spanID, service, name string, startMs, endMs int64) model.Span {
	return model.Span{
		TraceID:     model.TraceID(traceID),
		SpanID:      model.SpanID(spanID),
		Name:        name,
		StartTimeMs: startMs,
		EndTimeMs:   endMs,
		Attributes:  map[string]string{},
		Resource:    model.Resource{Attributes: map[string]string{"service.name": service}},
	}
}

func TestAppendAndGetTrace(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("trace1", "span1", "svc-a", "op1", 1000, 2000))
	s.Append(makeSpan("trace1", "span2", "svc-a", "op2", 1500, 2500))

	spans, err := s.GetTrace("trace1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if len(spans) != 2 {
		t.Errorf("got %d spans, want 2", len(spans))
	}
}

func TestGetTraceNotFound(t *testing.T) {
	s := trace.NewStore(time.Hour)
	_, err := s.GetTrace("nonexistent")
	if err == nil {
		t.Error("expected error for missing traceID, got nil")
	}
}

func TestGetTraceSortedByStartTime(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s2", "svc", "op2", 2000, 3000))
	s.Append(makeSpan("t1", "s1", "svc", "op1", 1000, 2000))

	spans, _ := s.GetTrace("t1")
	if spans[0].SpanID != "s1" {
		t.Errorf("spans not sorted by start time: first span = %s, want s1", spans[0].SpanID)
	}
}

func TestSearchByService(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "svc-a", "op", 1000, 2000))
	s.Append(makeSpan("t2", "s2", "svc-b", "op", 1000, 2000))

	results := s.Search(trace.Query{Service: "svc-a", Limit: 10})
	if len(results) != 1 {
		t.Errorf("Search by service: got %d results, want 1", len(results))
	}
	if results[0].TraceID != "t1" {
		t.Errorf("wrong trace returned: %s", results[0].TraceID)
	}
}

func TestSearchByOperation(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "svc", "reconcile", 1000, 2000))
	s.Append(makeSpan("t2", "s2", "svc", "create-pod", 1000, 2000))

	results := s.Search(trace.Query{Operation: "reconcile", Limit: 10})
	if len(results) != 1 || results[0].TraceID != "t1" {
		t.Errorf("Search by operation failed: %+v", results)
	}
}

func TestSearchByMinDuration(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "svc", "fast", 1000, 1100))  // 100ms
	s.Append(makeSpan("t2", "s2", "svc", "slow", 1000, 2000))  // 1000ms

	results := s.Search(trace.Query{MinDuration: 500 * time.Millisecond, Limit: 10})
	if len(results) != 1 || results[0].TraceID != "t2" {
		t.Errorf("Search by min duration failed: %+v", results)
	}
}

func TestSearchByTimeRange(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "svc", "op", 1000, 2000))
	s.Append(makeSpan("t2", "s2", "svc", "op", 5000, 6000))

	results := s.Search(trace.Query{
		StartTime: time.UnixMilli(500),
		EndTime:   time.UnixMilli(3000),
		Limit:     10,
	})
	if len(results) != 1 || results[0].TraceID != "t1" {
		t.Errorf("Search by time range failed: %+v", results)
	}
}

func TestSearchByTags(t *testing.T) {
	s := trace.NewStore(time.Hour)
	sp := makeSpan("t1", "s1", "svc", "op", 1000, 2000)
	sp.Attributes["http.status_code"] = "200"
	s.Append(sp)
	s.Append(makeSpan("t2", "s2", "svc", "op", 1000, 2000))

	results := s.Search(trace.Query{
		Tags:  map[string]string{"http.status_code": "200"},
		Limit: 10,
	})
	if len(results) != 1 || results[0].TraceID != "t1" {
		t.Errorf("Search by tags failed: %+v", results)
	}
}

func TestSearchLimit(t *testing.T) {
	s := trace.NewStore(time.Hour)
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		s.Append(makeSpan("trace"+id, "span"+id, "svc", "op", 1000, 2000))
	}
	results := s.Search(trace.Query{Limit: 3})
	if len(results) != 3 {
		t.Errorf("Search limit: got %d results, want 3", len(results))
	}
}

func TestTraceSummaryFields(t *testing.T) {
	s := trace.NewStore(time.Hour)
	root := makeSpan("t1", "root", "svc-a", "root-op", 1000, 3000)
	child := makeSpan("t1", "child", "svc-b", "child-op", 1500, 2500)
	child.ParentSpanID = "root"
	child.Status = model.SpanStatus{Code: 2, Message: "error"}
	s.Append(root)
	s.Append(child)

	results := s.Search(trace.Query{Limit: 10})
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	sum := results[0]
	if sum.SpanCount != 2 {
		t.Errorf("SpanCount = %d, want 2", sum.SpanCount)
	}
	if sum.Duration != 2000*time.Millisecond {
		t.Errorf("Duration = %v, want 2000ms", sum.Duration)
	}
	if !sum.HasError {
		t.Error("HasError = false, want true")
	}
	if len(sum.Services) != 2 {
		t.Errorf("Services = %v, want 2 entries", sum.Services)
	}
}

func TestRetentionEviction(t *testing.T) {
	s := trace.NewStore(100 * time.Millisecond)

	now := time.Now().UnixMilli()
	s.Append(makeSpan("old", "s1", "svc", "op", now-200, now-100)) // already expired

	time.Sleep(150 * time.Millisecond)
	s.Evict()

	_, err := s.GetTrace("old")
	if err == nil {
		t.Error("expected evicted trace to be gone, but GetTrace succeeded")
	}
}

func TestListServices(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "alpha", "op", 1000, 2000))
	s.Append(makeSpan("t2", "s2", "beta", "op", 1000, 2000))
	s.Append(makeSpan("t3", "s3", "alpha", "op", 1000, 2000))

	svcs := s.Services()
	seen := map[string]bool{}
	for _, sv := range svcs {
		seen[sv] = true
	}
	if !seen["alpha"] || !seen["beta"] {
		t.Errorf("Services() = %v, want alpha and beta", svcs)
	}
}

func TestOperations(t *testing.T) {
	s := trace.NewStore(time.Hour)
	s.Append(makeSpan("t1", "s1", "svc-a", "reconcile", 1000, 2000))
	s.Append(makeSpan("t2", "s2", "svc-a", "create-pod", 1000, 2000))
	s.Append(makeSpan("t3", "s3", "svc-b", "forward", 1000, 2000))

	ops := s.Operations("svc-a")
	if len(ops) != 2 {
		t.Errorf("Operations(svc-a) = %v, want 2 entries", ops)
	}
}
