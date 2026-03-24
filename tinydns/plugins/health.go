package plugins

import (
	"context"
	"net"
	"net/http"
)

// Health exposes a GET /health endpoint on a configurable HTTP address.
// It is not a DNS plugin in the chain — it runs as a side-car HTTP server.
type Health struct {
	addr   string
	srv    *http.Server
	ln     net.Listener
}

// NewHealth creates a Health checker that will listen on addr (e.g.
// "127.0.0.1:8080"). Use ":0" to let the OS pick a free port.
func NewHealth(addr string) *Health {
	return &Health{addr: addr}
}

// Start binds the HTTP listener and begins serving. It returns the actual
// bound address (useful when addr was ":0").
func (h *Health) Start() (string, error) {
	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		return "", err
	}
	h.ln = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h.srv = &http.Server{Handler: mux}
	go h.srv.Serve(ln)
	return ln.Addr().String(), nil
}

// Stop shuts down the HTTP server gracefully.
func (h *Health) Stop() error {
	if h.srv != nil {
		return h.srv.Shutdown(context.Background())
	}
	return nil
}
