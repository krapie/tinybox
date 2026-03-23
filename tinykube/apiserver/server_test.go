package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/apiserver"
	"github.com/krapi0314/tinybox/tinykube/logger"
	"github.com/krapi0314/tinybox/tinykube/store"
)

func newTestServer() (*apiserver.Server, *store.Store) {
	s := store.New(logger.NewNop())
	srv := apiserver.New(s)
	return srv, s
}

func TestCreateDeployment(t *testing.T) {
	srv, _ := newTestServer()

	dep := api.Deployment{
		Name:      "web",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 3,
			Template: api.PodTemplate{
				Spec: api.PodSpec{Image: "nginx:latest", Port: 80},
			},
		},
	}
	body, _ := json.Marshal(dep)

	req := httptest.NewRequest(http.MethodPost, "/apis/apps/v1/namespaces/default/deployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result api.Deployment
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Name != "web" {
		t.Fatalf("expected name 'web', got %s", result.Name)
	}
}

func TestCreateDeploymentConflict(t *testing.T) {
	srv, s := newTestServer()

	dep := api.Deployment{Name: "web", Namespace: "default"}
	s.Put("deployments/default/web", &dep)

	body, _ := json.Marshal(dep)
	req := httptest.NewRequest(http.MethodPost, "/apis/apps/v1/namespaces/default/deployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestListDeployments(t *testing.T) {
	srv, s := newTestServer()

	dep1 := &api.Deployment{Name: "web", Namespace: "default"}
	dep2 := &api.Deployment{Name: "api", Namespace: "default"}
	s.Put("deployments/default/web", dep1)
	s.Put("deployments/default/api", dep2)

	req := httptest.NewRequest(http.MethodGet, "/apis/apps/v1/namespaces/default/deployments", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []api.Deployment
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(result))
	}
}

func TestGetDeployment(t *testing.T) {
	srv, s := newTestServer()

	dep := &api.Deployment{Name: "web", Namespace: "default", Spec: api.DeploymentSpec{Replicas: 2}}
	s.Put("deployments/default/web", dep)

	req := httptest.NewRequest(http.MethodGet, "/apis/apps/v1/namespaces/default/deployments/web", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result api.Deployment
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Spec.Replicas != 2 {
		t.Fatalf("expected 2 replicas, got %d", result.Spec.Replicas)
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/apis/apps/v1/namespaces/default/deployments/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateDeployment(t *testing.T) {
	srv, s := newTestServer()

	dep := &api.Deployment{Name: "web", Namespace: "default", Spec: api.DeploymentSpec{Replicas: 1}}
	s.Put("deployments/default/web", dep)

	updated := api.Deployment{Name: "web", Namespace: "default", Spec: api.DeploymentSpec{Replicas: 5}}
	body, _ := json.Marshal(updated)

	req := httptest.NewRequest(http.MethodPut, "/apis/apps/v1/namespaces/default/deployments/web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result api.Deployment
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Spec.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", result.Spec.Replicas)
	}
}

func TestUpdateDeploymentNotFound(t *testing.T) {
	srv, _ := newTestServer()

	dep := api.Deployment{Name: "web", Namespace: "default"}
	body, _ := json.Marshal(dep)

	req := httptest.NewRequest(http.MethodPut, "/apis/apps/v1/namespaces/default/deployments/web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteDeployment(t *testing.T) {
	srv, s := newTestServer()

	dep := &api.Deployment{Name: "web", Namespace: "default"}
	s.Put("deployments/default/web", dep)

	req := httptest.NewRequest(http.MethodDelete, "/apis/apps/v1/namespaces/default/deployments/web", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	_, ok := s.Get("deployments/default/web")
	if ok {
		t.Fatal("deployment should have been deleted from store")
	}
}

func TestDeleteDeploymentNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/apis/apps/v1/namespaces/default/deployments/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetDeploymentStatus(t *testing.T) {
	srv, s := newTestServer()

	dep := &api.Deployment{
		Name:      "web",
		Namespace: "default",
		Status:    api.DeploymentStatus{Replicas: 3, ReadyReplicas: 2},
	}
	s.Put("deployments/default/web", dep)

	req := httptest.NewRequest(http.MethodGet, "/apis/apps/v1/namespaces/default/deployments/web/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status api.DeploymentStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if status.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", status.Replicas)
	}
	if status.ReadyReplicas != 2 {
		t.Fatalf("expected 2 ready replicas, got %d", status.ReadyReplicas)
	}
}

func TestListPods(t *testing.T) {
	srv, s := newTestServer()

	pod1 := &api.Pod{Name: "pod1", Namespace: "default"}
	pod2 := &api.Pod{Name: "pod2", Namespace: "default"}
	s.Put("pods/default/pod1", pod1)
	s.Put("pods/default/pod2", pod2)
	// pod in different namespace
	s.Put("pods/other/pod3", &api.Pod{Name: "pod3", Namespace: "other"})

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/pods", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []api.Pod
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(result))
	}
}

func TestGetPod(t *testing.T) {
	srv, s := newTestServer()

	pod := &api.Pod{Name: "pod1", Namespace: "default", Status: api.PodRunning}
	s.Put("pods/default/pod1", pod)

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/pods/pod1", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result api.Pod
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.Status != api.PodRunning {
		t.Fatalf("expected Running, got %s", result.Status)
	}
}

func TestGetPodNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/pods/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// --- Service tests ---

func TestCreateService(t *testing.T) {
	srv, _ := newTestServer()

	svc := api.Service{
		Name:      "web-svc",
		Namespace: "default",
		Spec: api.ServiceSpec{
			Selector:   map[string]string{"app": "web"},
			Port:       80,
			TargetPort: 80,
		},
	}
	body, _ := json.Marshal(svc)

	req := httptest.NewRequest(http.MethodPost, "/apis/v1/namespaces/default/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result api.Service
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Name != "web-svc" {
		t.Fatalf("expected name 'web-svc', got %s", result.Name)
	}
}

func TestCreateServiceConflict(t *testing.T) {
	srv, s := newTestServer()

	svc := &api.Service{Name: "web-svc", Namespace: "default"}
	s.Put("services/default/web-svc", svc)

	body, _ := json.Marshal(svc)
	req := httptest.NewRequest(http.MethodPost, "/apis/v1/namespaces/default/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestListServices(t *testing.T) {
	srv, s := newTestServer()

	s.Put("services/default/svc1", &api.Service{Name: "svc1", Namespace: "default"})
	s.Put("services/default/svc2", &api.Service{Name: "svc2", Namespace: "default"})

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []api.Service
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 services, got %d", len(result))
	}
}

func TestGetService(t *testing.T) {
	srv, s := newTestServer()

	svc := &api.Service{
		Name:      "web-svc",
		Namespace: "default",
		Spec:      api.ServiceSpec{Selector: map[string]string{"app": "web"}, Port: 80},
	}
	s.Put("services/default/web-svc", svc)

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services/web-svc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result api.Service
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Spec.Port != 80 {
		t.Fatalf("expected port 80, got %d", result.Spec.Port)
	}
}

func TestGetServiceNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteService(t *testing.T) {
	srv, s := newTestServer()

	s.Put("services/default/web-svc", &api.Service{Name: "web-svc", Namespace: "default"})

	req := httptest.NewRequest(http.MethodDelete, "/apis/v1/namespaces/default/services/web-svc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	if _, ok := s.Get("services/default/web-svc"); ok {
		t.Fatal("service should have been deleted")
	}
}

func TestDeleteServiceNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/apis/v1/namespaces/default/services/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetServiceEndpoints(t *testing.T) {
	srv, s := newTestServer()

	svc := &api.Service{
		Name:      "web-svc",
		Namespace: "default",
		Spec: api.ServiceSpec{
			Selector:   map[string]string{"app": "web"},
			Port:       80,
			TargetPort: 80,
		},
	}
	s.Put("services/default/web-svc", svc)

	// Running pods with matching label
	pod1 := &api.Pod{
		Name:      "web-a1b2c",
		Namespace: "default",
		Labels:    map[string]string{"app": "web"},
		Status:    api.PodRunning,
		HostPort:  54321,
	}
	pod2 := &api.Pod{
		Name:      "web-d3e4f",
		Namespace: "default",
		Labels:    map[string]string{"app": "web"},
		Status:    api.PodRunning,
		HostPort:  54322,
	}
	// Pod in wrong state — should be excluded
	pod3 := &api.Pod{
		Name:      "web-pend",
		Namespace: "default",
		Labels:    map[string]string{"app": "web"},
		Status:    api.PodPending,
		HostPort:  54323,
	}
	// Pod with non-matching label — should be excluded
	pod4 := &api.Pod{
		Name:      "other-xyz",
		Namespace: "default",
		Labels:    map[string]string{"app": "other"},
		Status:    api.PodRunning,
		HostPort:  54324,
	}
	s.Put("pods/default/web-a1b2c", pod1)
	s.Put("pods/default/web-d3e4f", pod2)
	s.Put("pods/default/web-pend", pod3)
	s.Put("pods/default/other-xyz", pod4)

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services/web-svc/endpoints", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var endpoints []api.ServiceEndpoint
	if err := json.NewDecoder(w.Body).Decode(&endpoints); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	for _, ep := range endpoints {
		if ep.Addr == "" {
			t.Error("endpoint Addr should not be empty")
		}
	}
}

func TestGetServiceEndpointsNotFound(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services/nope/endpoints", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetServiceEndpointsNoRunningPods(t *testing.T) {
	srv, s := newTestServer()

	s.Put("services/default/empty-svc", &api.Service{
		Name:      "empty-svc",
		Namespace: "default",
		Spec:      api.ServiceSpec{Selector: map[string]string{"app": "ghost"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/apis/v1/namespaces/default/services/empty-svc/endpoints", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var endpoints []api.ServiceEndpoint
	if err := json.NewDecoder(w.Body).Decode(&endpoints); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected 0 endpoints, got %d", len(endpoints))
	}
}
