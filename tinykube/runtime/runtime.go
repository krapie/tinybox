package runtime

import (
	"context"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

// PodRuntime is the CRI-like interface that abstracts container operations.
// The controller uses this interface; it never calls Docker directly.
type PodRuntime interface {
	// CreatePod starts a container for the pod, updating pod.PodIP and pod.ContainerID.
	CreatePod(ctx context.Context, pod *api.Pod) error

	// DeletePod stops and removes the container gracefully.
	DeletePod(ctx context.Context, pod *api.Pod) error

	// PodStatus returns the current phase of the pod by inspecting the container.
	PodStatus(ctx context.Context, pod *api.Pod) (api.PodPhase, error)

	// IsReady probes the pod's readiness endpoint.
	IsReady(ctx context.Context, pod *api.Pod) bool
}
