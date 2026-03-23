package apiserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/store"
)

// Server is the tinykube API server.
type Server struct {
	store   *store.Store
	handler http.Handler
}

// New creates a new Server backed by the given store.
func New(s *store.Store) *Server {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	return NewWithLogger(s, logger)
}

// NewWithLogger creates a Server using the provided logger (useful in tests).
func NewWithLogger(s *store.Store, logger *log.Logger) *Server {
	srv := &Server{store: s}
	mux := http.NewServeMux()

	// Deployment endpoints
	mux.HandleFunc("/apis/apps/v1/namespaces/", srv.routeDeployments)

	// Pod and Service endpoints
	mux.HandleFunc("/apis/v1/namespaces/", srv.routeCore)

	srv.handler = loggingMiddleware(logger, mux)
	return srv
}

// Handler returns the HTTP handler for this server.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ListenAndServe starts the HTTP server on addr.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.handler)
}

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an error message as JSON.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// routeDeployments dispatches deployment API requests.
// Pattern: /apis/apps/v1/namespaces/{ns}/deployments[/{name}[/status]]
func (s *Server) routeDeployments(w http.ResponseWriter, r *http.Request) {
	// Strip prefix "/apis/apps/v1/namespaces/"
	path := strings.TrimPrefix(r.URL.Path, "/apis/apps/v1/namespaces/")
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")

	// parts[0] = namespace, parts[1] = "deployments", parts[2] = name (optional), parts[3] = "status" (optional)
	if len(parts) < 2 || parts[1] != "deployments" {
		http.NotFound(w, r)
		return
	}

	ns := parts[0]

	switch {
	case len(parts) == 2:
		// /apis/apps/v1/namespaces/{ns}/deployments
		switch r.Method {
		case http.MethodGet:
			s.listDeployments(w, r, ns)
		case http.MethodPost:
			s.createDeployment(w, r, ns)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case len(parts) == 3:
		// /apis/apps/v1/namespaces/{ns}/deployments/{name}
		name := parts[2]
		switch r.Method {
		case http.MethodGet:
			s.getDeployment(w, r, ns, name)
		case http.MethodPut:
			s.updateDeployment(w, r, ns, name)
		case http.MethodDelete:
			s.deleteDeployment(w, r, ns, name)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case len(parts) == 4 && parts[3] == "status":
		// /apis/apps/v1/namespaces/{ns}/deployments/{name}/status
		name := parts[2]
		if r.Method == http.MethodGet {
			s.getDeploymentStatus(w, r, ns, name)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

// routeCore dispatches pod and service API requests.
// Patterns:
//
//	/apis/v1/namespaces/{ns}/pods[/{name}]
//	/apis/v1/namespaces/{ns}/services[/{name}[/endpoints]]
func (s *Server) routeCore(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/apis/v1/namespaces/")
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")

	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	ns := parts[0]

	switch parts[1] {
	case "pods":
		s.dispatchPods(w, r, ns, parts[2:])
	case "services":
		s.dispatchServices(w, r, ns, parts[2:])
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) dispatchPods(w http.ResponseWriter, r *http.Request, ns string, rest []string) {
	switch len(rest) {
	case 0:
		if r.Method == http.MethodGet {
			s.listPods(w, r, ns)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case 1:
		if r.Method == http.MethodGet {
			s.getPod(w, r, ns, rest[0])
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) dispatchServices(w http.ResponseWriter, r *http.Request, ns string, rest []string) {
	switch len(rest) {
	case 0:
		switch r.Method {
		case http.MethodGet:
			s.listServices(w, r, ns)
		case http.MethodPost:
			s.createService(w, r, ns)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case 1:
		name := rest[0]
		switch r.Method {
		case http.MethodGet:
			s.getService(w, r, ns, name)
		case http.MethodPut:
			s.updateService(w, r, ns, name)
		case http.MethodDelete:
			s.deleteService(w, r, ns, name)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case 2:
		if rest[1] == "endpoints" && r.Method == http.MethodGet {
			s.getServiceEndpoints(w, r, ns, rest[0])
		} else {
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// --- Deployment handlers ---

func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request, ns string) {
	var dep api.Deployment
	if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dep.Namespace = ns
	key := "deployments/" + ns + "/" + dep.Name

	if _, exists := s.store.Get(key); exists {
		writeError(w, http.StatusConflict, "deployment already exists")
		return
	}

	s.store.Put(key, &dep)
	writeJSON(w, http.StatusCreated, dep)
}

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request, ns string) {
	items := s.store.List("deployments/" + ns + "/")
	deps := make([]api.Deployment, 0, len(items))
	for _, item := range items {
		if d, ok := item.(*api.Deployment); ok {
			deps = append(deps, *d)
		}
	}
	writeJSON(w, http.StatusOK, deps)
}

func (s *Server) getDeployment(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "deployments/" + ns + "/" + name
	val, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	dep := val.(*api.Deployment)
	writeJSON(w, http.StatusOK, dep)
}

func (s *Server) updateDeployment(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "deployments/" + ns + "/" + name
	if _, exists := s.store.Get(key); !exists {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}

	var dep api.Deployment
	if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dep.Namespace = ns
	dep.Name = name
	s.store.Put(key, &dep)
	writeJSON(w, http.StatusOK, dep)
}

func (s *Server) deleteDeployment(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "deployments/" + ns + "/" + name
	if _, exists := s.store.Get(key); !exists {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	s.store.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getDeploymentStatus(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "deployments/" + ns + "/" + name
	val, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	dep := val.(*api.Deployment)
	writeJSON(w, http.StatusOK, dep.Status)
}

// --- Pod handlers ---

func (s *Server) listPods(w http.ResponseWriter, r *http.Request, ns string) {
	items := s.store.List("pods/" + ns + "/")
	pods := make([]api.Pod, 0, len(items))
	for _, item := range items {
		if p, ok := item.(*api.Pod); ok {
			pods = append(pods, *p)
		}
	}
	writeJSON(w, http.StatusOK, pods)
}

func (s *Server) getPod(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "pods/" + ns + "/" + name
	val, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "pod not found")
		return
	}
	pod := val.(*api.Pod)
	writeJSON(w, http.StatusOK, pod)
}

// --- Service handlers ---

func (s *Server) createService(w http.ResponseWriter, r *http.Request, ns string) {
	var svc api.Service
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	svc.Namespace = ns
	key := "services/" + ns + "/" + svc.Name

	if _, exists := s.store.Get(key); exists {
		writeError(w, http.StatusConflict, "service already exists")
		return
	}

	s.store.Put(key, &svc)
	writeJSON(w, http.StatusCreated, svc)
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request, ns string) {
	items := s.store.List("services/" + ns + "/")
	svcs := make([]api.Service, 0, len(items))
	for _, item := range items {
		if svc, ok := item.(*api.Service); ok {
			svcs = append(svcs, *svc)
		}
	}
	writeJSON(w, http.StatusOK, svcs)
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "services/" + ns + "/" + name
	val, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	writeJSON(w, http.StatusOK, val.(*api.Service))
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "services/" + ns + "/" + name
	if _, exists := s.store.Get(key); !exists {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	var svc api.Service
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	svc.Namespace = ns
	svc.Name = name
	s.store.Put(key, &svc)
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "services/" + ns + "/" + name
	if _, exists := s.store.Get(key); !exists {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	s.store.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getServiceEndpoints(w http.ResponseWriter, r *http.Request, ns, name string) {
	key := "services/" + ns + "/" + name
	val, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	svc := val.(*api.Service)

	items := s.store.List("pods/" + ns + "/")
	endpoints := make([]api.ServiceEndpoint, 0)
	for _, item := range items {
		pod, ok := item.(*api.Pod)
		if !ok {
			continue
		}
		if pod.Status != api.PodRunning {
			continue
		}
		if pod.HostPort == 0 {
			continue
		}
		if !api.LabelsMatch(svc.Spec.Selector, pod.Labels) {
			continue
		}
		endpoints = append(endpoints, api.ServiceEndpoint{
			PodName: pod.Name,
			Addr:    "localhost:" + itoa(pod.HostPort),
		})
	}
	writeJSON(w, http.StatusOK, endpoints)
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
