package runtime_test

import (
	"context"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/runtime"
)

func newPod(name string) *api.Pod {
	return &api.Pod{
		Name:      name,
		Namespace: "default",
		Spec: api.PodSpec{
			Image: "nginx:latest",
			Port:  80,
		},
		Status: api.PodPending,
	}
}

func TestFakeRuntimeCreatePodIncrementsCount(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")

	if err := fr.CreatePod(context.Background(), pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}

	if fr.CreateCount != 1 {
		t.Fatalf("expected CreateCount 1, got %d", fr.CreateCount)
	}
}

func TestFakeRuntimeCreatePodSetsPodIP(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")

	if err := fr.CreatePod(context.Background(), pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}

	if pod.PodIP == "" {
		t.Fatal("expected PodIP to be set after CreatePod")
	}
}

func TestFakeRuntimeCreatePodSetsContainerID(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")

	if err := fr.CreatePod(context.Background(), pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}

	if pod.ContainerID == "" {
		t.Fatal("expected ContainerID to be set after CreatePod")
	}
}

func TestFakeRuntimeCreatePodSetsStatusRunning(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")

	if err := fr.CreatePod(context.Background(), pod); err != nil {
		t.Fatalf("CreatePod failed: %v", err)
	}

	if pod.Status != api.PodRunning {
		t.Fatalf("expected status Running, got %s", pod.Status)
	}
}

func TestFakeRuntimeDeletePodIncrementsCount(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")
	_ = fr.CreatePod(context.Background(), pod)

	if err := fr.DeletePod(context.Background(), pod); err != nil {
		t.Fatalf("DeletePod failed: %v", err)
	}

	if fr.DeleteCount != 1 {
		t.Fatalf("expected DeleteCount 1, got %d", fr.DeleteCount)
	}
}

func TestFakeRuntimePodStatusReturnsCorrectPhase(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")
	_ = fr.CreatePod(context.Background(), pod)

	phase, err := fr.PodStatus(context.Background(), pod)
	if err != nil {
		t.Fatalf("PodStatus failed: %v", err)
	}

	if phase != api.PodRunning {
		t.Fatalf("expected Running, got %s", phase)
	}
}

func TestFakeRuntimePodStatusUnknownPod(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("unknown-pod")

	_, err := fr.PodStatus(context.Background(), pod)
	if err == nil {
		t.Fatal("expected error for unknown pod")
	}
}

func TestFakeRuntimeIsReadyTrueWhenRunning(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")
	_ = fr.CreatePod(context.Background(), pod)

	if !fr.IsReady(context.Background(), pod) {
		t.Fatal("expected IsReady to return true for running pod")
	}
}

func TestFakeRuntimeIsReadyFalseBeforeCreate(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod := newPod("pod1")

	if fr.IsReady(context.Background(), pod) {
		t.Fatal("expected IsReady to return false for pod not created yet")
	}
}

func TestFakeRuntimeReadyAfterNthCall(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	fr.ReadyAfter = 2
	pod := newPod("pod1")
	_ = fr.CreatePod(context.Background(), pod)

	// First call: not ready yet
	if fr.IsReady(context.Background(), pod) {
		t.Fatal("expected IsReady to return false on first call")
	}
	// Second call: not ready yet
	if fr.IsReady(context.Background(), pod) {
		t.Fatal("expected IsReady to return false on second call")
	}
	// Third call: ready
	if !fr.IsReady(context.Background(), pod) {
		t.Fatal("expected IsReady to return true after ReadyAfter calls")
	}
}

func TestFakeRuntimeMultiplePods(t *testing.T) {
	fr := runtime.NewFakeRuntime()
	pod1 := newPod("pod1")
	pod2 := newPod("pod2")

	_ = fr.CreatePod(context.Background(), pod1)
	_ = fr.CreatePod(context.Background(), pod2)

	if fr.CreateCount != 2 {
		t.Fatalf("expected CreateCount 2, got %d", fr.CreateCount)
	}

	if pod1.PodIP == pod2.PodIP {
		t.Fatal("expected different PodIPs for different pods")
	}
}
