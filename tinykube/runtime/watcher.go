package runtime

import (
	"context"
	"time"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/logger"
	"github.com/krapi0314/tinybox/tinykube/store"
)

// StartReadinessWatcher runs a background goroutine that polls the pod's readiness
// via the runtime and updates the pod's status in the store.
//
// State transitions:
//
//	Pending → (readiness probe passes) → Running
//	Running → (container exited)       → Failed
func StartReadinessWatcher(ctx context.Context, s *store.Store, rt PodRuntime, pod *api.Pod, log *logger.Logger) {
	go func() {
		// Wait for the initial delay if configured.
		delay := 0
		if pod.Spec.ReadinessProbe != nil {
			delay = pod.Spec.ReadinessProbe.InitialDelaySeconds
		}
		if delay > 0 {
			select {
			case <-time.After(time.Duration(delay) * time.Second):
			case <-ctx.Done():
				return
			}
		}

		period := 2 * time.Second
		if pod.Spec.ReadinessProbe != nil && pod.Spec.ReadinessProbe.PeriodSeconds > 0 {
			period = time.Duration(pod.Spec.ReadinessProbe.PeriodSeconds) * time.Second
		}

		ticker := time.NewTicker(period)
		defer ticker.Stop()

		key := "pods/" + pod.Namespace + "/" + pod.Name

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check actual container status.
				phase, err := rt.PodStatus(ctx, pod)
				if err != nil {
					log.Debug("watcher: pod=%s container gone, removing from store", pod.Name)
					s.Delete(key)
					return
				}

				val, ok := s.Get(key)
				if !ok {
					log.Debug("watcher: pod=%s removed from store by controller", pod.Name)
					return
				}
				current := val.(*api.Pod)

				switch phase {
				case api.PodFailed:
					if current.Status != api.PodFailed {
						log.Debug("watcher: pod=%s → Failed", pod.Name)
						current.Status = api.PodFailed
						s.Put(key, current)
					}
					return

				case api.PodRunning:
					if rt.IsReady(ctx, pod) {
						if current.Status != api.PodRunning {
							log.Debug("watcher: pod=%s Pending → Running", pod.Name)
							current.Status = api.PodRunning
							s.Put(key, current)
						}
					}
				}
			}
		}
	}()
}
