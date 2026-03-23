package v1

// Deployment represents a desired state for a set of pods.
type Deployment struct {
	Name      string           `yaml:"name"`
	Namespace string           `yaml:"namespace"`
	Spec      DeploymentSpec   `yaml:"spec"`
	Status    DeploymentStatus `yaml:"status,omitempty"`
}

// DeploymentSpec is the desired state of a Deployment.
type DeploymentSpec struct {
	Replicas int                    `yaml:"replicas"`
	Selector map[string]string      `yaml:"selector"`
	Template PodTemplate            `yaml:"template"`
	Strategy RollingUpdateStrategy  `yaml:"strategy"`
}

// PodTemplate defines the template for pods created by the deployment.
type PodTemplate struct {
	Labels map[string]string `yaml:"labels"`
	Spec   PodSpec           `yaml:"spec"`
}

// PodSpec defines the spec for a pod (container).
type PodSpec struct {
	Image          string            `yaml:"image"`
	Env            map[string]string `yaml:"env,omitempty"`
	Port           int               `yaml:"port"`
	ReadinessProbe *HTTPProbe        `yaml:"readinessProbe,omitempty"`
}

// HTTPProbe defines an HTTP readiness probe.
type HTTPProbe struct {
	Path                string `yaml:"path"`
	InitialDelaySeconds int    `yaml:"initialDelaySeconds"`
	PeriodSeconds       int    `yaml:"periodSeconds"`
	FailureThreshold    int    `yaml:"failureThreshold"`
}

// RollingUpdateStrategy configures rolling update behavior.
type RollingUpdateStrategy struct {
	MaxSurge       int `yaml:"maxSurge"`
	MaxUnavailable int `yaml:"maxUnavailable"`
}

// DeploymentStatus reports the observed state of a Deployment.
type DeploymentStatus struct {
	Replicas          int `yaml:"replicas,omitempty"`
	ReadyReplicas     int `yaml:"readyReplicas,omitempty"`
	AvailableReplicas int `yaml:"availableReplicas,omitempty"`
	UpdatedReplicas   int `yaml:"updatedReplicas,omitempty"`
}

// Pod represents a single running container.
type Pod struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Spec        PodSpec           `yaml:"spec"`
	Status      PodPhase          `yaml:"status,omitempty"`
	PodIP       string            `yaml:"podIP,omitempty"`
	ContainerID string            `yaml:"containerID,omitempty"`
}

// PodPhase is the phase of a pod.
type PodPhase string

const (
	PodPending     PodPhase = "Pending"
	PodRunning     PodPhase = "Running"
	PodTerminating PodPhase = "Terminating"
	PodFailed      PodPhase = "Failed"
)

// Manifest is the top-level YAML manifest envelope.
// kind must be "Deployment".
type Manifest struct {
	Kind      string     `yaml:"kind"`
	Name      string     `yaml:"name"`
	Namespace string     `yaml:"namespace"`
	Spec      DeploymentSpec `yaml:"spec"`
}

// ToDeployment converts a Manifest to a Deployment.
// Namespace defaults to "default" if empty.
func (m *Manifest) ToDeployment() *Deployment {
	ns := m.Namespace
	if ns == "" {
		ns = "default"
	}
	return &Deployment{
		Name:      m.Name,
		Namespace: ns,
		Spec:      m.Spec,
	}
}
