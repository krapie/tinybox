package main

import (
	"os"
	"path/filepath"
	"testing"

	api "github.com/krapi0314/tinybox/tinykube/api/v1"
)

const sampleManifest = `
kind: Deployment
name: nginx
namespace: default
spec:
  replicas: 3
  selector:
    app: nginx
  template:
    labels:
      app: nginx
    spec:
      image: nginx:alpine
      port: 80
      readinessProbe:
        path: /
        initialDelaySeconds: 2
        periodSeconds: 2
        failureThreshold: 3
  strategy:
    maxSurge: 1
    maxUnavailable: 1
`

func writeTempManifest(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return f.Name()
}

func TestParseManifestDeployment(t *testing.T) {
	path := writeTempManifest(t, sampleManifest)
	dep, err := parseManifestFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Name != "nginx" {
		t.Errorf("expected name nginx, got %s", dep.Name)
	}
	if dep.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", dep.Namespace)
	}
	if dep.Spec.Replicas != 3 {
		t.Errorf("expected replicas 3, got %d", dep.Spec.Replicas)
	}
	if dep.Spec.Template.Spec.Image != "nginx:alpine" {
		t.Errorf("expected image nginx:alpine, got %s", dep.Spec.Template.Spec.Image)
	}
	if dep.Spec.Template.Spec.Port != 80 {
		t.Errorf("expected port 80, got %d", dep.Spec.Template.Spec.Port)
	}
	if dep.Spec.Strategy.MaxSurge != 1 {
		t.Errorf("expected maxSurge 1, got %d", dep.Spec.Strategy.MaxSurge)
	}
	if dep.Spec.Strategy.MaxUnavailable != 1 {
		t.Errorf("expected maxUnavailable 1, got %d", dep.Spec.Strategy.MaxUnavailable)
	}
	probe := dep.Spec.Template.Spec.ReadinessProbe
	if probe == nil {
		t.Fatal("expected readinessProbe to be set")
	}
	if probe.Path != "/" {
		t.Errorf("expected probe path /, got %s", probe.Path)
	}
	if probe.InitialDelaySeconds != 2 {
		t.Errorf("expected initialDelaySeconds 2, got %d", probe.InitialDelaySeconds)
	}
}

func TestParseManifestDefaultNamespace(t *testing.T) {
	path := writeTempManifest(t, `
kind: Deployment
name: app
spec:
  replicas: 1
  template:
    spec:
      image: nginx:alpine
      port: 80
  strategy:
    maxSurge: 1
    maxUnavailable: 0
`)
	dep, err := parseManifestFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Namespace != "default" {
		t.Errorf("expected namespace default when omitted, got %s", dep.Namespace)
	}
}

func TestParseManifestUnknownKind(t *testing.T) {
	path := writeTempManifest(t, `kind: Service\nname: foo`)
	_, err := parseManifestFile(path)
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestParseManifestFileNotFound(t *testing.T) {
	_, err := parseManifestFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCmdApplyFile(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	path := writeTempManifest(t, sampleManifest)
	out, err := runCmd([]string{"apply", "-f", path, "--server", srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "deployment/nginx created\n" && out != "deployment/nginx updated\n" {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestApplyFileMutuallyExclusiveWithFlags(t *testing.T) {
	_, err := runCmd([]string{"apply", "-f", "some.yaml", "--name", "nginx"})
	if err == nil {
		t.Error("expected error when both -f and --name provided")
	}
}

func TestApplyRequiresFileOrName(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	_, err := runCmd([]string{"apply", "--server", srv.URL})
	if err == nil {
		t.Error("expected error when neither -f nor --name given")
	}
}

// Verify that the api/v1 types round-trip through yaml correctly.
func TestTypesYAMLRoundTrip(t *testing.T) {
	path := writeTempManifest(t, sampleManifest)
	dep, err := parseManifestFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// selector should be decoded
	if dep.Spec.Selector["app"] != "nginx" {
		t.Errorf("selector app expected nginx, got %v", dep.Spec.Selector)
	}
	// template labels should be decoded
	if dep.Spec.Template.Labels["app"] != "nginx" {
		t.Errorf("template label app expected nginx, got %v", dep.Spec.Template.Labels)
	}
	// status should be zero value (omitempty)
	if dep.Status != (api.DeploymentStatus{}) {
		t.Errorf("expected empty status, got %+v", dep.Status)
	}
}
