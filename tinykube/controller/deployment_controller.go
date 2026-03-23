package controller

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/logger"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

// DeploymentController reconciles Deployments with their pods.
type DeploymentController struct {
	store   *store.Store
	runtime runtime.PodRuntime
	log     *logger.Logger
}

// NewDeploymentController creates a new DeploymentController.
func NewDeploymentController(s *store.Store, rt runtime.PodRuntime, log *logger.Logger) *DeploymentController {
	return &DeploymentController{store: s, runtime: rt, log: log}
}

// Start runs the reconciliation loop every interval until ctx is cancelled.
func (c *DeploymentController) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.Reconcile(ctx)
		}
	}
}

// Reconcile examines all deployments in the store and brings actual state
// in line with desired state.
func (c *DeploymentController) Reconcile(ctx context.Context) error {
	depItems := c.store.List("deployments/")
	c.log.Debug("controller: reconcile — %d deployment(s)", len(depItems))

	// Build set of active deployment names for orphan cleanup.
	activeDeployments := make(map[string]bool, len(depItems))
	for _, item := range depItems {
		dep, ok := item.(*api.Deployment)
		if !ok {
			continue
		}
		activeDeployments[dep.Namespace+"/"+dep.Name] = true
		if err := c.reconcileDeployment(ctx, dep); err != nil {
			return err
		}
	}

	// Delete orphaned pods whose deployment no longer exists.
	for _, item := range c.store.List("pods/") {
		pod, ok := item.(*api.Pod)
		if !ok {
			continue
		}
		depName := pod.Labels["deployment"]
		key := pod.Namespace + "/" + depName
		if depName != "" && !activeDeployments[key] {
			c.log.Debug("controller: deleting orphaned pod %s (deployment %s gone)", pod.Name, depName)
			if err := c.runtime.DeletePod(ctx, pod); err != nil {
				c.log.Debug("controller: delete orphan pod %s: %v", pod.Name, err)
			}
			c.store.Delete("pods/" + pod.Namespace + "/" + pod.Name)
		}
	}

	return nil
}

// reconcileDeployment handles a single deployment reconcile pass.
func (c *DeploymentController) reconcileDeployment(ctx context.Context, dep *api.Deployment) error {
	pods := c.podsForDeployment(dep)
	desiredHash := templateHash(dep.Spec.Template.Spec)
	c.log.Debug("controller: deployment=%s/%s desired=%d current=%d", dep.Namespace, dep.Name, dep.Spec.Replicas, len(pods))

	// Check if a rolling update is needed.
	oldPods := podsWithDifferentHash(pods, desiredHash)
	newPods := podsWithHash(pods, desiredHash)

	if len(oldPods) > 0 {
		// Rolling update: replace old pods with new ones.
		c.log.Debug("controller: rolling update triggered for %s/%s", dep.Namespace, dep.Name)
		if err := rollingUpdate(ctx, c.store, c.runtime, dep, oldPods, newPods, c.log); err != nil {
			return err
		}
		// Re-fetch pods after rolling update.
		pods = c.podsForDeployment(dep)
	} else {
		// Normal scale up/down.
		if err := c.scale(ctx, dep, pods); err != nil {
			return err
		}
		pods = c.podsForDeployment(dep)
	}

	// Update deployment status.
	c.updateStatus(dep, pods, desiredHash)
	return nil
}

// scale adjusts the number of pods to match dep.Spec.Replicas.
func (c *DeploymentController) scale(ctx context.Context, dep *api.Deployment, pods []*api.Pod) error {
	desired := dep.Spec.Replicas
	hash := templateHash(dep.Spec.Template.Spec)

	// Scale up.
	for len(pods) < desired {
		pod := newPod(dep, hash)
		c.log.Debug("controller: scale up — creating pod %s (%s)", pod.Name, pod.Spec.Image)
		if err := c.runtime.CreatePod(ctx, pod); err != nil {
			return fmt.Errorf("create pod: %w", err)
		}
		key := "pods/" + pod.Namespace + "/" + pod.Name
		c.store.Put(key, pod)
		runtime.StartReadinessWatcher(ctx, c.store, c.runtime, pod, c.log)
		pods = append(pods, pod)
	}

	// Scale down.
	for len(pods) > desired {
		pod := pods[len(pods)-1]
		pods = pods[:len(pods)-1]
		c.log.Debug("controller: scale down — deleting pod %s", pod.Name)
		if err := c.runtime.DeletePod(ctx, pod); err != nil {
			return fmt.Errorf("delete pod: %w", err)
		}
		key := "pods/" + pod.Namespace + "/" + pod.Name
		c.store.Delete(key)
	}
	return nil
}

// updateStatus computes and writes DeploymentStatus back to the store.
func (c *DeploymentController) updateStatus(dep *api.Deployment, pods []*api.Pod, desiredHash string) {
	var ready, updated int
	for _, p := range pods {
		if p.Status == api.PodRunning {
			ready++
		}
		if p.Labels["template-hash"] == desiredHash {
			updated++
		}
	}
	dep.Status = api.DeploymentStatus{
		Replicas:          len(pods),
		ReadyReplicas:     ready,
		AvailableReplicas: ready,
		UpdatedReplicas:   updated,
	}
	key := "deployments/" + dep.Namespace + "/" + dep.Name
	c.store.Put(key, dep)
}

// podsForDeployment returns all pods belonging to the deployment.
func (c *DeploymentController) podsForDeployment(dep *api.Deployment) []*api.Pod {
	items := c.store.List("pods/" + dep.Namespace + "/")
	var pods []*api.Pod
	for _, item := range items {
		p, ok := item.(*api.Pod)
		if !ok {
			continue
		}
		if p.Labels["deployment"] == dep.Name {
			pods = append(pods, p)
		}
	}
	return pods
}

// templateHash returns a simple hash string for a PodSpec.
func templateHash(spec api.PodSpec) string {
	return fmt.Sprintf("%v", spec)
}

// podsWithHash returns pods whose template-hash label matches hash.
func podsWithHash(pods []*api.Pod, hash string) []*api.Pod {
	var result []*api.Pod
	for _, p := range pods {
		if p.Labels["template-hash"] == hash {
			result = append(result, p)
		}
	}
	return result
}

// podsWithDifferentHash returns pods whose template-hash label does NOT match hash.
func podsWithDifferentHash(pods []*api.Pod, hash string) []*api.Pod {
	var result []*api.Pod
	for _, p := range pods {
		if p.Labels["template-hash"] != hash {
			result = append(result, p)
		}
	}
	return result
}

// newPod creates a new Pod object from a Deployment template.
func newPod(dep *api.Deployment, hash string) *api.Pod {
	labels := make(map[string]string, len(dep.Spec.Template.Labels)+2)
	for k, v := range dep.Spec.Template.Labels {
		labels[k] = v
	}
	labels["deployment"] = dep.Name
	labels["template-hash"] = hash

	return &api.Pod{
		Name:      dep.Name + "-" + randomSuffix(),
		Namespace: dep.Namespace,
		Labels:    labels,
		Spec:      dep.Spec.Template.Spec,
		Status:    api.PodPending,
	}
}

// randomSuffix generates a short random alphanumeric string.
func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 5)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return strings.ToLower(string(b))
}
