//go:build integration

package runtime_test

import (
	"context"
	"testing"
	"time"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/logger"
	"github.com/krapi0314/tinybox/tinykube/runtime"
)

// Integration tests require a running Docker daemon.
// Run with: go test -tags=integration ./runtime/...

func TestDockerRuntimeCreateAndDeletePod(t *testing.T) {
	dr, err := runtime.NewDockerRuntime(logger.New(true))
	if err != nil {
		t.Fatalf("failed to create DockerRuntime: %v", err)
	}
	defer dr.Close()

	ctx := context.Background()
	pod := &api.Pod{
		Name:      "integ-test-pod",
		Namespace: "integ-test",
		Spec: api.PodSpec{
			Image: "nginx:alpine",
			Port:  80,
		},
		Status: api.PodPending,
	}

	t.Log("Creating pod...")
	if err := dr.CreatePod(ctx, pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}

	if pod.PodIP == "" {
		t.Error("expected PodIP to be set")
	}
	if pod.ContainerID == "" {
		t.Error("expected ContainerID to be set")
	}

	t.Logf("Pod created: containerID=%s podIP=%s", pod.ContainerID, pod.PodIP)

	// Check status
	phase, err := dr.PodStatus(ctx, pod)
	if err != nil {
		t.Fatalf("PodStatus failed: %v", err)
	}
	if phase != api.PodRunning {
		t.Fatalf("expected Running, got %s", phase)
	}

	// Cleanup
	t.Log("Deleting pod...")
	if err := dr.DeletePod(ctx, pod); err != nil {
		t.Fatalf("DeletePod failed: %v", err)
	}
}

func TestDockerRuntimeIsReadyNoProbe(t *testing.T) {
	dr, err := runtime.NewDockerRuntime(logger.New(true))
	if err != nil {
		t.Fatalf("failed to create DockerRuntime: %v", err)
	}
	defer dr.Close()

	ctx := context.Background()
	pod := &api.Pod{
		Name:      "integ-ready-pod",
		Namespace: "integ-test",
		Spec: api.PodSpec{
			Image: "nginx:alpine",
			Port:  80,
		},
		Status: api.PodPending,
	}

	if err := dr.CreatePod(ctx, pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}
	defer func() { _ = dr.DeletePod(ctx, pod) }()

	// Allow container time to start
	time.Sleep(500 * time.Millisecond)

	// No readiness probe → ready when running
	if !dr.IsReady(ctx, pod) {
		t.Fatal("expected IsReady to return true (no probe configured)")
	}
}
