package controller_test

import (
	"context"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/controller"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

func setupRollingUpdate() (*controller.DeploymentController, *store.Store, *runtime.FakeRuntime) {
	s := store.New()
	fr := runtime.NewFakeRuntime()
	c := controller.NewDeploymentController(s, fr)
	return c, s, fr
}

func countPodsWithImage(s *store.Store, ns, img string) int {
	items := s.List("pods/" + ns + "/")
	count := 0
	for _, item := range items {
		if p, ok := item.(*api.Pod); ok && p.Spec.Image == img {
			count++
		}
	}
	return count
}

func TestRollingUpdateReplacesOldPods(t *testing.T) {
	c, s, _ := setupRollingUpdate()

	dep := &api.Deployment{
		Name:      "web",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 3,
			Selector: map[string]string{"app": "web"},
			Template: api.PodTemplate{
				Labels: map[string]string{"app": "web"},
				Spec:   api.PodSpec{Image: "nginx:1.0", Port: 80},
			},
			Strategy: api.RollingUpdateStrategy{MaxSurge: 1, MaxUnavailable: 1},
		},
	}
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	_ = c.Reconcile(ctx)

	// Verify initial state: 3 pods with nginx:1.0
	if n := countPodsWithImage(s, "default", "nginx:1.0"); n != 3 {
		t.Fatalf("expected 3 pods with nginx:1.0, got %d", n)
	}

	// Update the image.
	dep.Spec.Template.Spec.Image = "nginx:2.0"
	s.Put("deployments/default/web", dep)

	_ = c.Reconcile(ctx)

	// After rolling update, all pods should be on the new image.
	if n := countPodsWithImage(s, "default", "nginx:2.0"); n != 3 {
		t.Fatalf("expected 3 pods with nginx:2.0 after rolling update, got %d", n)
	}
	if n := countPodsWithImage(s, "default", "nginx:1.0"); n != 0 {
		t.Fatalf("expected 0 pods with nginx:1.0 after rolling update, got %d", n)
	}
}

func TestRollingUpdateMaxSurge(t *testing.T) {
	c, s, _ := setupRollingUpdate()

	dep := &api.Deployment{
		Name:      "web",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 3,
			Selector: map[string]string{"app": "web"},
			Template: api.PodTemplate{
				Labels: map[string]string{"app": "web"},
				Spec:   api.PodSpec{Image: "nginx:1.0", Port: 80},
			},
			Strategy: api.RollingUpdateStrategy{MaxSurge: 1, MaxUnavailable: 0},
		},
	}
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	_ = c.Reconcile(ctx)

	dep.Spec.Template.Spec.Image = "nginx:2.0"
	s.Put("deployments/default/web", dep)
	_ = c.Reconcile(ctx)

	// After update completes, exactly 3 pods should exist.
	pods := listPods(s, "default")
	if len(pods) != 3 {
		t.Fatalf("expected exactly 3 pods after rolling update, got %d", len(pods))
	}
}

func TestRollingUpdateUpdatedReplicasInStatus(t *testing.T) {
	c, s, _ := setupRollingUpdate()

	dep := &api.Deployment{
		Name:      "web",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 2,
			Selector: map[string]string{"app": "web"},
			Template: api.PodTemplate{
				Labels: map[string]string{"app": "web"},
				Spec:   api.PodSpec{Image: "nginx:1.0", Port: 80},
			},
			Strategy: api.RollingUpdateStrategy{MaxSurge: 1, MaxUnavailable: 1},
		},
	}
	s.Put("deployments/default/web", dep)

	ctx := context.Background()
	_ = c.Reconcile(ctx)

	dep.Spec.Template.Spec.Image = "nginx:2.0"
	s.Put("deployments/default/web", dep)
	_ = c.Reconcile(ctx)

	val, _ := s.Get("deployments/default/web")
	updated := val.(*api.Deployment)

	if updated.Status.UpdatedReplicas != 2 {
		t.Fatalf("expected UpdatedReplicas 2, got %d", updated.Status.UpdatedReplicas)
	}
}

func TestRollingUpdateSingleReplica(t *testing.T) {
	c, s, _ := setupRollingUpdate()

	dep := &api.Deployment{
		Name:      "web",
		Namespace: "default",
		Spec: api.DeploymentSpec{
			Replicas: 1,
			Selector: map[string]string{"app": "web"},
			Template: api.PodTemplate{
				Labels: map[string]string{"app": "web"},
				Spec:   api.PodSpec{Image: "nginx:1.0", Port: 80},
			},
			Strategy: api.RollingUpdateStrategy{MaxSurge: 1, MaxUnavailable: 0},
		},
	}
	s.Put("deployments/default/web", dep)
	ctx := context.Background()
	_ = c.Reconcile(ctx)

	dep.Spec.Template.Spec.Image = "nginx:2.0"
	s.Put("deployments/default/web", dep)
	_ = c.Reconcile(ctx)

	if n := countPodsWithImage(s, "default", "nginx:2.0"); n != 1 {
		t.Fatalf("expected 1 pod with nginx:2.0, got %d", n)
	}
	if n := countPodsWithImage(s, "default", "nginx:1.0"); n != 0 {
		t.Fatalf("expected 0 pods with nginx:1.0, got %d", n)
	}
}
