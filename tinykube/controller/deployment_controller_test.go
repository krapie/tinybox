package controller_test

import (
	"context"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/controller"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

func setupController() (*controller.DeploymentController, *store.Store, *runtime.FakeRuntime) {
	s := store.New()
	fr := runtime.NewFakeRuntime()
	c := controller.NewDeploymentController(s, fr)
	return c, s, fr
}

func makeDeployment(name, ns string, replicas int, image string) *api.Deployment {
	return &api.Deployment{
		Name:      name,
		Namespace: ns,
		Spec: api.DeploymentSpec{
			Replicas: replicas,
			Selector: map[string]string{"app": name},
			Template: api.PodTemplate{
				Labels: map[string]string{"app": name},
				Spec:   api.PodSpec{Image: image, Port: 80},
			},
		},
	}
}

func listPods(s *store.Store, ns string) []*api.Pod {
	items := s.List("pods/" + ns + "/")
	pods := make([]*api.Pod, 0, len(items))
	for _, item := range items {
		if p, ok := item.(*api.Pod); ok {
			pods = append(pods, p)
		}
	}
	return pods
}

func TestScaleUp(t *testing.T) {
	c, s, fr := setupController()

	dep := makeDeployment("web", "default", 3, "nginx:latest")
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	if err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	pods := listPods(s, "default")
	if len(pods) != 3 {
		t.Fatalf("expected 3 pods, got %d", len(pods))
	}

	if fr.CreateCount != 3 {
		t.Fatalf("expected CreateCount 3, got %d", fr.CreateCount)
	}
}

func TestScaleDown(t *testing.T) {
	c, s, fr := setupController()

	dep := makeDeployment("web", "default", 1, "nginx:latest")
	s.Put("deployments/default/web", dep)

	// Pre-create 3 pods.
	ctx := context.Background()
	dep.Spec.Replicas = 3
	s.Put("deployments/default/web", dep)
	_ = c.Reconcile(ctx)

	if fr.CreateCount != 3 {
		t.Fatalf("setup: expected 3 pods created, got %d", fr.CreateCount)
	}

	// Now scale down to 1.
	dep.Spec.Replicas = 1
	s.Put("deployments/default/web", dep)
	if err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	pods := listPods(s, "default")
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod after scale-down, got %d", len(pods))
	}

	if fr.DeleteCount != 2 {
		t.Fatalf("expected DeleteCount 2, got %d", fr.DeleteCount)
	}
}

func TestStatusUpdate(t *testing.T) {
	c, s, _ := setupController()

	dep := makeDeployment("web", "default", 2, "nginx:latest")
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	if err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	val, ok := s.Get("deployments/default/web")
	if !ok {
		t.Fatal("deployment not found in store")
	}
	updated := val.(*api.Deployment)

	if updated.Status.Replicas != 2 {
		t.Fatalf("expected Status.Replicas 2, got %d", updated.Status.Replicas)
	}

	// FakeRuntime immediately sets pods to Running, so ReadyReplicas should be 2.
	if updated.Status.ReadyReplicas != 2 {
		t.Fatalf("expected Status.ReadyReplicas 2, got %d", updated.Status.ReadyReplicas)
	}
}

func TestReconcileIdempotent(t *testing.T) {
	c, s, fr := setupController()

	dep := makeDeployment("web", "default", 2, "nginx:latest")
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	_ = c.Reconcile(ctx)
	_ = c.Reconcile(ctx)
	_ = c.Reconcile(ctx)

	pods := listPods(s, "default")
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods after 3 reconciles, got %d", len(pods))
	}
	if fr.CreateCount != 2 {
		t.Fatalf("expected CreateCount 2 (idempotent), got %d", fr.CreateCount)
	}
}

func TestScaleToZero(t *testing.T) {
	c, s, fr := setupController()

	dep := makeDeployment("web", "default", 3, "nginx:latest")
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	_ = c.Reconcile(ctx)

	dep.Spec.Replicas = 0
	s.Put("deployments/default/web", dep)
	_ = c.Reconcile(ctx)

	pods := listPods(s, "default")
	if len(pods) != 0 {
		t.Fatalf("expected 0 pods, got %d", len(pods))
	}
	if fr.DeleteCount != 3 {
		t.Fatalf("expected DeleteCount 3, got %d", fr.DeleteCount)
	}
}

func TestMultipleDeployments(t *testing.T) {
	c, s, fr := setupController()

	dep1 := makeDeployment("web", "default", 2, "nginx:latest")
	dep2 := makeDeployment("api", "default", 1, "redis:latest")
	s.Put("deployments/default/web", dep1)
	s.Put("deployments/default/api", dep2)

	ctx := context.Background()
	if err := c.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	pods := listPods(s, "default")
	if len(pods) != 3 {
		t.Fatalf("expected 3 total pods, got %d", len(pods))
	}
	if fr.CreateCount != 3 {
		t.Fatalf("expected CreateCount 3, got %d", fr.CreateCount)
	}
}
