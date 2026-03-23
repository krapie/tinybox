package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

// newTestServer returns a test HTTP server that stubs tinykube API responses.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	dep := api.Deployment{
		Name:      "nginx",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 3,
			Template: api.PodTemplate{Spec: api.PodSpec{Image: "nginx:alpine", Port: 80}},
			Strategy: api.RollingUpdateStrategy{MaxSurge: 1, MaxUnavailable: 1},
		},
		Status: api.DeploymentStatus{Replicas: 3, ReadyReplicas: 3},
	}

	mux.HandleFunc("/apis/apps/v1/namespaces/default/deployments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]api.Deployment{dep})
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dep)
		}
	})
	mux.HandleFunc("/apis/apps/v1/namespaces/default/deployments/nginx", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dep)
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dep)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/apis/apps/v1/namespaces/default/deployments/nginx/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dep.Status)
	})
	mux.HandleFunc("/apis/v1/namespaces/default/pods", func(w http.ResponseWriter, r *http.Request) {
		pods := []api.Pod{
			{Name: "nginx-abc12", Namespace: "default", Status: api.PodRunning,
				Spec: api.PodSpec{Image: "nginx:alpine"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pods)
	})

	svc := api.Service{
		Name:      "web-svc",
		Namespace: "default",
		Spec:      api.ServiceSpec{Selector: map[string]string{"app": "web"}, Port: 80, TargetPort: 80},
	}
	mux.HandleFunc("/apis/v1/namespaces/default/services", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]api.Service{svc})
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(svc)
		}
	})
	mux.HandleFunc("/apis/v1/namespaces/default/services/web-svc", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(svc)
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(svc)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	return httptest.NewServer(mux)
}

func TestCmdGetDeployments(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{"get", "deployments", "--server", srv.URL, "--namespace", "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected nginx in output, got: %s", out)
	}
	if !strings.Contains(out, "3") {
		t.Errorf("expected replica count in output, got: %s", out)
	}
}

func TestCmdGetPods(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{"get", "pods", "--server", srv.URL, "--namespace", "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nginx-abc12") {
		t.Errorf("expected pod name in output, got: %s", out)
	}
	if !strings.Contains(out, "Running") {
		t.Errorf("expected Running status in output, got: %s", out)
	}
}

func TestCmdApplyCreate(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{
		"apply",
		"--server", srv.URL,
		"--namespace", "default",
		"--name", "nginx",
		"--image", "nginx:alpine",
		"--replicas", "3",
		"--port", "80",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected deployment name in output, got: %s", out)
	}
}

func TestCmdDeleteDeployment(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{
		"delete", "deployment", "nginx",
		"--server", srv.URL,
		"--namespace", "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got: %s", out)
	}
}

func TestCmdStatusDeployment(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{
		"status", "deployment", "nginx",
		"--server", srv.URL,
		"--namespace", "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ReadyReplicas") {
		t.Errorf("expected ReadyReplicas in output, got: %s", out)
	}
}

func TestCmdUnknownCommand(t *testing.T) {
	_, err := runCmd([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestCmdGetServices(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{"get", "services", "--server", srv.URL, "--namespace", "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "web-svc") {
		t.Errorf("expected web-svc in output, got: %s", out)
	}
	if !strings.Contains(out, "80") {
		t.Errorf("expected port 80 in output, got: %s", out)
	}
}

func TestCmdDeleteService(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	out, err := runCmd([]string{"delete", "service", "web-svc", "--server", srv.URL, "--namespace", "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got: %s", out)
	}
}

func TestCmdApplyServiceFile(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	path := writeTempManifest(t, `
kind: Service
name: web-svc
namespace: default
serviceSpec:
  selector:
    app: web
  port: 80
  targetPort: 80
`)
	out, err := runCmd([]string{"apply", "-f", path, "--server", srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "web-svc") {
		t.Errorf("expected web-svc in output, got: %s", out)
	}
}
