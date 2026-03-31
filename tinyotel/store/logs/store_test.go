package logs_test

import (
	"testing"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/store/logs"
)

func makeLog(service, severity, body string, timeMs int64, traceID model.TraceID) model.LogRecord {
	return model.LogRecord{
		TimeMs:       timeMs,
		SeverityText: severity,
		SeverityNum:  severityNum(severity),
		Body:         body,
		Resource:     model.Resource{Attributes: map[string]string{"service.name": service}},
		TraceID:      traceID,
	}
}

func severityNum(s string) int {
	switch s {
	case "TRACE":
		return 1
	case "DEBUG":
		return 5
	case "INFO":
		return 9
	case "WARN":
		return 13
	case "ERROR":
		return 17
	case "FATAL":
		return 21
	}
	return 0
}

func TestAppendAndQuery(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("svc", "INFO", "hello", 1000, ""))

	results := s.Query(logs.Query{})
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].Body != "hello" {
		t.Errorf("unexpected body %q", results[0].Body)
	}
}

func TestQueryByService(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("frontend", "INFO", "req", 1000, ""))
	s.Append(makeLog("backend", "INFO", "resp", 2000, ""))

	results := s.Query(logs.Query{Service: "frontend"})
	if len(results) != 1 || results[0].Resource.Attributes["service.name"] != "frontend" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestQueryBySeverity(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("svc", "DEBUG", "debug msg", 1000, ""))
	s.Append(makeLog("svc", "ERROR", "error msg", 2000, ""))

	// minimum severity WARN — should return only ERROR
	results := s.Query(logs.Query{Severity: "WARN"})
	if len(results) != 1 {
		t.Fatalf("expected 1 record at WARN+, got %d", len(results))
	}
	if results[0].SeverityText != "ERROR" {
		t.Errorf("unexpected severity %q", results[0].SeverityText)
	}
}

func TestQueryByTraceID(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("svc", "INFO", "trace log", 1000, "abc123"))
	s.Append(makeLog("svc", "INFO", "other log", 2000, "def456"))

	results := s.Query(logs.Query{TraceID: "abc123"})
	if len(results) != 1 || results[0].Body != "trace log" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestQueryTimeRange(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("svc", "INFO", "early", 100, ""))
	s.Append(makeLog("svc", "INFO", "mid", 500, ""))
	s.Append(makeLog("svc", "INFO", "late", 900, ""))

	results := s.Query(logs.Query{StartMs: 200, EndMs: 800})
	if len(results) != 1 || results[0].Body != "mid" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestQueryLimit(t *testing.T) {
	s := logs.NewStore(1000)
	for i := 0; i < 10; i++ {
		s.Append(makeLog("svc", "INFO", "msg", int64(i*100), ""))
	}
	results := s.Query(logs.Query{Limit: 3})
	if len(results) != 3 {
		t.Errorf("expected 3, got %d", len(results))
	}
}

func TestRingBufferEviction(t *testing.T) {
	s := logs.NewStore(3) // capacity = 3
	s.Append(makeLog("svc", "INFO", "first", 1000, ""))
	s.Append(makeLog("svc", "INFO", "second", 2000, ""))
	s.Append(makeLog("svc", "INFO", "third", 3000, ""))
	s.Append(makeLog("svc", "INFO", "fourth", 4000, "")) // evicts "first"

	results := s.Query(logs.Query{})
	if len(results) != 3 {
		t.Fatalf("expected 3 records (capacity), got %d", len(results))
	}
	// "first" should be gone
	for _, r := range results {
		if r.Body == "first" {
			t.Error("evicted record 'first' is still present")
		}
	}
}

func TestQueryMultipleFilters(t *testing.T) {
	s := logs.NewStore(1000)
	s.Append(makeLog("svc", "ERROR", "svc error", 1000, "trace1"))
	s.Append(makeLog("other", "ERROR", "other error", 2000, "trace2"))
	s.Append(makeLog("svc", "INFO", "svc info", 3000, "trace3"))

	results := s.Query(logs.Query{Service: "svc", Severity: "ERROR"})
	if len(results) != 1 || results[0].Body != "svc error" {
		t.Errorf("unexpected results: %v", results)
	}
}
