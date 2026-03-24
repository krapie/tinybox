package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/krapi0314/tinybox/tinydns/apiserver"
	"github.com/krapi0314/tinybox/tinydns/registry"
)

func setup(t *testing.T) (*registry.Registry, http.Handler) {
	t.Helper()
	reg := registry.New()
	return reg, apiserver.NewHandler(reg)
}

func TestRegisterService(t *testing.T) {
	reg, h := setup(t)

	body, _ := json.Marshal(registry.ServiceRecord{
		Name: "whoami.default.svc.cluster.local.",
		IP:   "10.0.0.1",
		TTL:  30,
	})
	req := httptest.NewRequest(http.MethodPost, "/registry/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	records := reg.Lookup("whoami.default.svc.cluster.local.")
	if len(records) != 1 {
		t.Errorf("registry has %d records, want 1", len(records))
	}
}

func TestListServices(t *testing.T) {
	reg, h := setup(t)
	reg.Register(registry.ServiceRecord{Name: "a.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	reg.Register(registry.ServiceRecord{Name: "b.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})

	req := httptest.NewRequest(http.MethodGet, "/registry/services", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var records []registry.ServiceRecord
	if err := json.NewDecoder(rr.Body).Decode(&records); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("records count = %d, want 2", len(records))
	}
}

func TestDeregisterService(t *testing.T) {
	reg, h := setup(t)
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})

	req := httptest.NewRequest(http.MethodDelete, "/registry/services/svc.default.svc.cluster.local.", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if len(reg.Lookup("svc.default.svc.cluster.local.")) != 0 {
		t.Error("record still in registry after deregister")
	}
}

func TestHealthEndpoint(t *testing.T) {
	_, h := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
