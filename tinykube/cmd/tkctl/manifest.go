package main

import (
	"fmt"
	"os"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"gopkg.in/yaml.v3"
)

// parseManifestFile reads a YAML manifest file and returns a Deployment or Service
// depending on the `kind` field. Exactly one of the returned pointers is non-nil.
// Supported kinds: Deployment, Service.
func parseManifestFile(path string) (*api.Deployment, *api.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read manifest: %w", err)
	}

	var m api.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("parse manifest: %w", err)
	}

	if m.Name == "" {
		return nil, nil, fmt.Errorf("manifest missing required field: name")
	}

	switch m.Kind {
	case "Deployment":
		return m.ToDeployment(), nil, nil
	case "Service":
		svc := m.ToService()
		if svc == nil {
			return nil, nil, fmt.Errorf("Service manifest missing serviceSpec field")
		}
		return nil, svc, nil
	default:
		return nil, nil, fmt.Errorf("unsupported kind %q — supported: Deployment, Service", m.Kind)
	}
}
