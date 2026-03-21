package v1

// Deployment represents a desired state for a set of pods.
type Deployment struct {
	Name      string
	Namespace string
	Spec      DeploymentSpec
	Status    DeploymentStatus
}

// DeploymentSpec is the desired state of a Deployment.
type DeploymentSpec struct {
	Replicas int
	Selector map[string]string
	Template PodTemplate
	Strategy RollingUpdateStrategy
}

// PodTemplate defines the template for pods created by the deployment.
type PodTemplate struct {
	Labels map[string]string
	Spec   PodSpec
}

// PodSpec defines the spec for a pod (container).
type PodSpec struct {
	Image          string
	Env            map[string]string
	Port           int
	ReadinessProbe *HTTPProbe
}

// HTTPProbe defines an HTTP readiness probe.
type HTTPProbe struct {
	Path                string
	InitialDelaySeconds int
	PeriodSeconds       int
	FailureThreshold    int
}

// RollingUpdateStrategy configures rolling update behavior.
type RollingUpdateStrategy struct {
	MaxSurge       int // extra pods allowed during update
	MaxUnavailable int // pods allowed to be unavailable during update
}

// DeploymentStatus reports the observed state of a Deployment.
type DeploymentStatus struct {
	Replicas          int
	ReadyReplicas     int
	AvailableReplicas int
	UpdatedReplicas   int
}

// Pod represents a single running container.
type Pod struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Spec        PodSpec
	Status      PodPhase
	PodIP       string
	ContainerID string
}

// PodPhase is the phase of a pod.
type PodPhase string

const (
	PodPending     PodPhase = "Pending"
	PodRunning     PodPhase = "Running"
	PodTerminating PodPhase = "Terminating"
	PodFailed      PodPhase = "Failed"
)
