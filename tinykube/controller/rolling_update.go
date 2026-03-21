package controller

import (
	"context"
	"fmt"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

// rollingUpdate replaces oldPods with new pods (using dep's current template) in batches
// bounded by dep.Spec.Strategy.MaxSurge and MaxUnavailable.
func rollingUpdate(
	ctx context.Context,
	s *store.Store,
	rt runtime.PodRuntime,
	dep *api.Deployment,
	oldPods []*api.Pod,
	newPods []*api.Pod,
) error {
	desired := dep.Spec.Replicas
	hash := templateHash(dep.Spec.Template.Spec)

	maxSurge := dep.Spec.Strategy.MaxSurge
	if maxSurge < 1 {
		maxSurge = 1
	}
	maxUnavailable := dep.Spec.Strategy.MaxUnavailable
	if maxUnavailable < 0 {
		maxUnavailable = 0
	}

	// We iterate in waves: create up to maxSurge new pods, wait for them to
	// be ready, then delete up to maxUnavailable old pods.  Repeat until all
	// old pods have been replaced.
	for len(oldPods) > 0 {
		// How many new pods can we create this wave?
		total := len(oldPods) + len(newPods)
		canCreate := (desired + maxSurge) - total
		if canCreate <= 0 {
			canCreate = 1
		}
		if canCreate > len(oldPods) {
			canCreate = len(oldPods)
		}

		// Create new pods.
		for i := 0; i < canCreate; i++ {
			pod := newPod(dep, hash)
			if err := rt.CreatePod(ctx, pod); err != nil {
				return fmt.Errorf("rolling update create pod: %w", err)
			}
			key := "pods/" + pod.Namespace + "/" + pod.Name
			s.Put(key, pod)
			newPods = append(newPods, pod)
		}

		// Wait for new pods to be ready (FakeRuntime is instant; DockerRuntime needs polling).
		for _, pod := range newPods[len(newPods)-canCreate:] {
			waitReady(ctx, rt, pod)
		}

		// Delete old pods (up to maxUnavailable or canCreate, whichever is smaller).
		toDelete := maxUnavailable
		if toDelete < 1 {
			toDelete = canCreate // ensure progress when maxUnavailable==0
		}
		if toDelete > len(oldPods) {
			toDelete = len(oldPods)
		}

		for i := 0; i < toDelete; i++ {
			pod := oldPods[0]
			oldPods = oldPods[1:]
			if err := rt.DeletePod(ctx, pod); err != nil {
				return fmt.Errorf("rolling update delete pod: %w", err)
			}
			key := "pods/" + pod.Namespace + "/" + pod.Name
			s.Delete(key)
		}
	}

	// If desired > len(newPods), scale up any remaining gap (shouldn't normally happen).
	for len(newPods) < desired {
		pod := newPod(dep, hash)
		if err := rt.CreatePod(ctx, pod); err != nil {
			return fmt.Errorf("scale up after rolling update: %w", err)
		}
		key := "pods/" + pod.Namespace + "/" + pod.Name
		s.Put(key, pod)
		newPods = append(newPods, pod)
	}

	return nil
}

// waitReady polls until the pod is ready or the context is done.
// For FakeRuntime this returns immediately; for DockerRuntime it polls.
func waitReady(ctx context.Context, rt runtime.PodRuntime, pod *api.Pod) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if rt.IsReady(ctx, pod) {
			return
		}
		// Small sleep to avoid tight-looping in real scenarios; tests will return immediately.
	}
}
