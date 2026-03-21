package runtime

import (
	"context"
	"fmt"
	"sync"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

// FakeRuntime is an in-memory PodRuntime for unit tests.
// It instantly transitions pods to Running and tracks call counts.
type FakeRuntime struct {
	mu          sync.Mutex
	pods        map[string]*api.Pod
	readyCalls  map[string]int
	nextIP      int
	CreateCount int
	DeleteCount int
	ReadyAfter  int // number of IsReady calls before returning true (0 = always ready)
}

// NewFakeRuntime creates a new FakeRuntime.
func NewFakeRuntime() *FakeRuntime {
	return &FakeRuntime{
		pods:       make(map[string]*api.Pod),
		readyCalls: make(map[string]int),
		nextIP:     1,
	}
}

// CreatePod registers the pod in memory, sets PodIP and ContainerID, sets status to Running.
func (f *FakeRuntime) CreatePod(ctx context.Context, pod *api.Pod) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.CreateCount++
	pod.PodIP = fmt.Sprintf("10.0.0.%d", f.nextIP)
	f.nextIP++
	pod.ContainerID = fmt.Sprintf("fake-container-%s", pod.Name)
	pod.Status = api.PodRunning

	// Store a copy
	cp := *pod
	f.pods[pod.Name] = &cp
	return nil
}

// DeletePod removes the pod from memory.
func (f *FakeRuntime) DeletePod(ctx context.Context, pod *api.Pod) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.DeleteCount++
	delete(f.pods, pod.Name)
	delete(f.readyCalls, pod.Name)
	return nil
}

// PodStatus returns the current phase of the pod.
func (f *FakeRuntime) PodStatus(ctx context.Context, pod *api.Pod) (api.PodPhase, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	p, ok := f.pods[pod.Name]
	if !ok {
		return "", fmt.Errorf("pod %s not found", pod.Name)
	}
	return p.Status, nil
}

// IsReady returns true once the pod has been created and the ReadyAfter threshold
// of IsReady calls has been reached.
func (f *FakeRuntime) IsReady(ctx context.Context, pod *api.Pod) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	p, ok := f.pods[pod.Name]
	if !ok || p.Status != api.PodRunning {
		return false
	}

	if f.ReadyAfter == 0 {
		return true
	}

	f.readyCalls[pod.Name]++
	return f.readyCalls[pod.Name] > f.ReadyAfter
}
