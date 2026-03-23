package v1_test

import (
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

func TestLabelsMatch(t *testing.T) {
	tests := []struct {
		name     string
		selector map[string]string
		labels   map[string]string
		want     bool
	}{
		{
			name:     "exact match",
			selector: map[string]string{"app": "web"},
			labels:   map[string]string{"app": "web"},
			want:     true,
		},
		{
			name:     "subset match — extra label on pod",
			selector: map[string]string{"app": "web"},
			labels:   map[string]string{"app": "web", "env": "prod"},
			want:     true,
		},
		{
			name:     "value mismatch",
			selector: map[string]string{"app": "web"},
			labels:   map[string]string{"app": "api"},
			want:     false,
		},
		{
			name:     "missing key",
			selector: map[string]string{"app": "web"},
			labels:   map[string]string{"env": "prod"},
			want:     false,
		},
		{
			name:     "empty selector matches anything",
			selector: map[string]string{},
			labels:   map[string]string{"app": "web"},
			want:     true,
		},
		{
			name:     "nil selector matches anything",
			selector: nil,
			labels:   map[string]string{"app": "web"},
			want:     true,
		},
		{
			name:     "multi-key all match",
			selector: map[string]string{"app": "web", "tier": "frontend"},
			labels:   map[string]string{"app": "web", "tier": "frontend", "env": "prod"},
			want:     true,
		},
		{
			name:     "multi-key partial miss",
			selector: map[string]string{"app": "web", "tier": "frontend"},
			labels:   map[string]string{"app": "web", "tier": "backend"},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := api.LabelsMatch(tc.selector, tc.labels)
			if got != tc.want {
				t.Errorf("LabelsMatch(%v, %v) = %v; want %v", tc.selector, tc.labels, got, tc.want)
			}
		})
	}
}

func TestManifestToService(t *testing.T) {
	m := &api.Manifest{
		Kind:      "Service",
		Name:      "web-svc",
		Namespace: "default",
		ServiceSpec: &api.ServiceSpec{
			Selector:   map[string]string{"app": "web"},
			Port:       80,
			TargetPort: 80,
		},
	}

	svc := m.ToService()
	if svc == nil {
		t.Fatal("ToService() returned nil")
	}
	if svc.Name != "web-svc" {
		t.Errorf("Name = %q; want %q", svc.Name, "web-svc")
	}
	if svc.Namespace != "default" {
		t.Errorf("Namespace = %q; want %q", svc.Namespace, "default")
	}
	if svc.Spec.Port != 80 {
		t.Errorf("Port = %d; want 80", svc.Spec.Port)
	}
	if svc.Spec.Selector["app"] != "web" {
		t.Errorf("Selector[app] = %q; want %q", svc.Spec.Selector["app"], "web")
	}
}

func TestManifestToServiceNamespaceDefault(t *testing.T) {
	m := &api.Manifest{
		Kind: "Service",
		Name: "api-svc",
		ServiceSpec: &api.ServiceSpec{
			Selector:   map[string]string{"app": "api"},
			Port:       8080,
			TargetPort: 8080,
		},
	}

	svc := m.ToService()
	if svc.Namespace != "default" {
		t.Errorf("Namespace = %q; want %q", svc.Namespace, "default")
	}
}

func TestManifestToServiceNilServiceSpec(t *testing.T) {
	m := &api.Manifest{
		Kind: "Deployment",
		Name: "web",
	}
	if m.ToService() != nil {
		t.Error("ToService() should return nil when ServiceSpec is nil")
	}
}
