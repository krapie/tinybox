package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingMiddleware_LogsRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewAccessLog(logger, inner)

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("X-Cluster", "api")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	output := buf.String()
	if output == "" {
		t.Fatal("access log should have written at least one log line")
	}
	if !strings.Contains(output, "GET") {
		t.Errorf("log should contain method GET, got: %s", output)
	}
	if !strings.Contains(output, "/v1/users") {
		t.Errorf("log should contain path /v1/users, got: %s", output)
	}
}

func TestLoggingMiddleware_LogsStatusCode(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := NewAccessLog(logger, inner)
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	output := buf.String()
	if !strings.Contains(output, "404") {
		t.Errorf("log should contain status 404, got: %s", output)
	}
}

func TestLoggingMiddleware_LogsLatency(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewAccessLog(logger, inner)
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	output := buf.String()
	// Should contain latency_ms or duration field
	if !strings.Contains(output, "latency") && !strings.Contains(output, "duration") {
		t.Errorf("log should contain latency field, got: %s", output)
	}
}

func TestLoggingMiddleware_PassesThrough(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("body"))
	})

	handler := NewAccessLog(logger, inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
	if rr.Body.String() != "body" {
		t.Errorf("body = %q, want body", rr.Body.String())
	}
}
