package main

import (
	"fmt"
	"os"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
	"gopkg.in/yaml.v3"
)

// parseManifestFile reads a YAML manifest file and returns a Deployment.
// Only kind=Deployment is supported.
func parseManifestFile(path string) (*api.Deployment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m api.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if m.Kind != "Deployment" {
		return nil, fmt.Errorf("unsupported kind %q — only Deployment is supported", m.Kind)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("manifest missing required field: name")
	}

	return m.ToDeployment(), nil
}
