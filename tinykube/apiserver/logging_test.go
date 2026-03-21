package apiserver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingMiddlewareLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := loggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/apis/apps/v1/namespaces/default/deployments", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	line := buf.String()
	if !strings.Contains(line, "POST") {
		t.Errorf("expected method in log, got: %s", line)
	}
	if !strings.Contains(line, "/apis/apps/v1/namespaces/default/deployments") {
		t.Errorf("expected path in log, got: %s", line)
	}
	if !strings.Contains(line, "201") {
		t.Errorf("expected status 201 in log, got: %s", line)
	}
	if !strings.Contains(line, "5B") {
		t.Errorf("expected response size in log, got: %s", line)
	}
}

func TestLoggingMiddlewareRecords404(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := loggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/not/found", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !strings.Contains(buf.String(), "404") {
		t.Errorf("expected 404 in log, got: %s", buf.String())
	}
}

func TestLoggingMiddlewareDefaultStatus200(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := loggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !strings.Contains(buf.String(), "200") {
		t.Errorf("expected default 200 in log, got: %s", buf.String())
	}
}
