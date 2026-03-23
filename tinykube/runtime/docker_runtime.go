package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"github.com/krapi0314/tinybox/tinykube/logger"
)

// DockerRuntime implements PodRuntime using the Docker SDK.
type DockerRuntime struct {
	cli *client.Client
	log *logger.Logger
}

// NewDockerRuntime creates a DockerRuntime with a default Docker client.
func NewDockerRuntime(log *logger.Logger) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerRuntime{cli: cli, log: log}, nil
}

// Close releases resources held by the Docker client.
func (d *DockerRuntime) Close() error {
	return d.cli.Close()
}

// networkName returns the Docker network name for a namespace.
func networkName(ns string) string {
	return "tinykube-" + ns
}

// containerName returns the Docker container name for a pod.
func containerName(pod *api.Pod) string {
	return "tinykube-" + pod.Name
}

// ensureNetwork creates the Docker bridge network for ns if it doesn't exist.
func (d *DockerRuntime) ensureNetwork(ctx context.Context, ns string) error {
	name := networkName(ns)
	f := filters.NewArgs(filters.Arg("name", name))
	networks, err := d.cli.NetworkList(ctx, dockertypes.NetworkListOptions{Filters: f})
	if err != nil {
		return fmt.Errorf("network list: %w", err)
	}
	for _, n := range networks {
		if n.Name == name {
			return nil // already exists
		}
	}
	_, err = d.cli.NetworkCreate(ctx, name, dockertypes.NetworkCreate{
		Driver: "bridge",
		Labels: map[string]string{"tinykube": "true"},
	})
	return err
}

// CreatePod starts a container for the pod and sets PodIP and ContainerID.
func (d *DockerRuntime) CreatePod(ctx context.Context, pod *api.Pod) error {
	d.log.Debug("runtime: CreatePod pod=%s image=%s", pod.Name, pod.Spec.Image)
	if err := d.ensureNetwork(ctx, pod.Namespace); err != nil {
		return fmt.Errorf("ensure network: %w", err)
	}

	// Pull image if not present.
	reader, err := d.cli.ImagePull(ctx, pod.Spec.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("image pull: %w", err)
	}
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()

	// Build env slice.
	var env []string
	for k, v := range pod.Spec.Env {
		env = append(env, k+"="+v)
	}

	// Expose port.
	exposedPorts := nat.PortSet{}
	portBinding := nat.PortMap{}
	if pod.Spec.Port > 0 {
		p := nat.Port(fmt.Sprintf("%d/tcp", pod.Spec.Port))
		exposedPorts[p] = struct{}{}
		portBinding[p] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}}
	}

	cname := containerName(pod)
	labels := map[string]string{
		"tinykube":  "true",
		"namespace": pod.Namespace,
	}
	if dep, ok := pod.Labels["deployment"]; ok {
		labels["deployment"] = dep
	}

	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        pod.Spec.Image,
			Env:          env,
			ExposedPorts: exposedPorts,
			Labels:       labels,
		},
		&container.HostConfig{
			PortBindings: portBinding,
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkName(pod.Namespace): {},
			},
		},
		nil,
		cname,
	)
	if err != nil {
		return fmt.Errorf("container create: %w", err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("container start: %w", err)
	}

	// Inspect to get IP.
	info, err := d.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return fmt.Errorf("container inspect: %w", err)
	}

	pod.ContainerID = resp.ID
	if ep, ok := info.NetworkSettings.Networks[networkName(pod.Namespace)]; ok {
		pod.PodIP = ep.IPAddress
	}

	// Capture the host-mapped port for endpoint discovery (needed on macOS where
	// container IPs are inside the Docker VM and not reachable from the host).
	if pod.Spec.Port > 0 {
		portKey := nat.Port(fmt.Sprintf("%d/tcp", pod.Spec.Port))
		if bindings, ok := info.NetworkSettings.Ports[portKey]; ok && len(bindings) > 0 {
			hostPortStr := bindings[0].HostPort
			var hp int
			_, _ = fmt.Sscanf(hostPortStr, "%d", &hp)
			pod.HostPort = hp
		}
	}

	pod.Status = api.PodPending
	return nil
}

// DeletePod stops and removes the container.
func (d *DockerRuntime) DeletePod(ctx context.Context, pod *api.Pod) error {
	d.log.Debug("runtime: DeletePod pod=%s", pod.Name)
	pod.Status = api.PodTerminating

	timeout := 5
	if err := d.cli.ContainerStop(ctx, containerName(pod), container.StopOptions{Timeout: &timeout}); err != nil {
		// Log but don't fail — may already be stopped.
		_ = err
	}

	if err := d.cli.ContainerRemove(ctx, containerName(pod), container.RemoveOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}
	return nil
}

// PodStatus returns the current phase of the pod.
func (d *DockerRuntime) PodStatus(ctx context.Context, pod *api.Pod) (api.PodPhase, error) {
	info, err := d.cli.ContainerInspect(ctx, containerName(pod))
	if err != nil {
		if client.IsErrNotFound(err) {
			return api.PodFailed, nil
		}
		return "", fmt.Errorf("container inspect: %w", err)
	}

	switch info.State.Status {
	case "running":
		return api.PodRunning, nil
	case "exited", "dead":
		return api.PodFailed, nil
	default:
		return api.PodPending, nil
	}
}

// IsReady probes the pod's readiness endpoint.
// If no readiness probe is configured, returns true once container is running.
// Uses the host-mapped port (127.0.0.1:{hostPort}) so the probe works on macOS
// where container IPs are inside the Docker VM and not reachable from the host.
func (d *DockerRuntime) IsReady(ctx context.Context, pod *api.Pod) bool {
	phase, err := d.PodStatus(ctx, pod)
	if err != nil || phase != api.PodRunning {
		return false
	}

	if pod.Spec.ReadinessProbe == nil {
		// No probe configured: ready when running.
		return true
	}

	// Inspect to find the host-mapped port.
	info, err := d.cli.ContainerInspect(ctx, containerName(pod))
	if err != nil {
		return false
	}
	portKey := nat.Port(fmt.Sprintf("%d/tcp", pod.Spec.Port))
	bindings, ok := info.NetworkSettings.Ports[portKey]
	if !ok || len(bindings) == 0 {
		return false
	}
	hostPort := bindings[0].HostPort

	url := fmt.Sprintf("http://127.0.0.1:%s%s", hostPort, pod.Spec.ReadinessProbe.Path)
	hc := &http.Client{Timeout: time.Second}
	resp, err := hc.Get(url)
	if err != nil {
		d.log.Debug("runtime: IsReady pod=%s url=%s → false (%v)", pod.Name, url, err)
		return false
	}
	_ = resp.Body.Close()
	ready := resp.StatusCode >= 200 && resp.StatusCode < 300
	d.log.Debug("runtime: IsReady pod=%s url=%s → %v (status=%d)", pod.Name, url, ready, resp.StatusCode)
	return ready
}
