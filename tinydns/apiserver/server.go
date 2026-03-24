// Package apiserver provides the HTTP REST API for the tinydns service registry.
//
// Endpoints:
//
//	POST   /registry/services           — register a service record
//	DELETE /registry/services/{name}    — deregister all records for a name
//	GET    /registry/services           — list all non-expired records
//	GET    /health                      — health check
package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/krapi0314/tinybox/tinydns/registry"
)

// NewHandler returns an http.Handler wired to reg.
func NewHandler(reg *registry.Registry) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/registry/services", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			registerService(w, r, reg)
		case http.MethodGet:
			listServices(w, r, reg)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/registry/services/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/registry/services/")
		if name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		reg.Deregister(name)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

func registerService(w http.ResponseWriter, r *http.Request, reg *registry.Registry) {
	var rec registry.ServiceRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	reg.Register(rec)
	w.WriteHeader(http.StatusCreated)
}

func listServices(w http.ResponseWriter, _ *http.Request, reg *registry.Registry) {
	records := reg.ListAll()
	if records == nil {
		records = []registry.ServiceRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}
