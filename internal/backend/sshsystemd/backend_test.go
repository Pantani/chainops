package sshsystemd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pantani/gorchestrator/internal/chain/genericprocess"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

func TestValidateTargetRejectsContainerMode(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "generic-single-compose.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	cluster.Spec.Runtime.Backend = BackendName

	backend := New()
	diags := backend.ValidateTarget(cluster)

	if !hasError(diags) {
		t.Fatalf("expected validation error for container workload, got none")
	}

	found := false
	for _, d := range diags {
		if d.Path == "spec.nodePools[0].template.workloads[0].mode" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mode diagnostic, got: %#v", diags)
	}
}

func TestValidateTargetAcceptsAliasAndHostWorkloads(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	cluster.Spec.Runtime.Backend = BackendAlias

	backend := New()
	diags := backend.ValidateTarget(cluster)
	if hasError(diags) {
		t.Fatalf("unexpected validation error: %#v", diags)
	}
}

func TestBuildDesiredRenderGolden(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	plugin := genericprocess.New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	pluginOut, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("plugin build: %v", err)
	}

	backend := New()
	desired, err := backend.BuildDesired(context.Background(), cluster, pluginOut)
	if err != nil {
		t.Fatalf("build desired: %v", err)
	}

	assertArtifactsSorted(t, desired)
	assertServicesSorted(t, desired)

	assertArtifactMatchesGolden(
		t,
		desired,
		"ssh-systemd/nodes/validator/systemd/generic-ssh-validator-daemon.service",
		"systemd-unit.golden",
	)
	assertArtifactMatchesGolden(
		t,
		desired,
		"ssh-systemd/nodes/validator/env/generic-ssh-validator-daemon.env",
		"env-file.golden",
	)
	assertArtifactMatchesGolden(
		t,
		desired,
		"ssh-systemd/nodes/validator/layout/directories.txt",
		"directories.golden",
	)
}

func assertArtifactMatchesGolden(t *testing.T, desired domain.DesiredState, artifactPath, goldenName string) {
	t.Helper()
	content, ok := findArtifact(desired, artifactPath)
	if !ok {
		t.Fatalf("artifact %q not found", artifactPath)
	}

	expected, err := os.ReadFile(filepath.Join("testdata", goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}
	if content != string(expected) {
		t.Fatalf("artifact mismatch for %s\n--- got ---\n%s\n--- expected ---\n%s", artifactPath, content, string(expected))
	}
}

func findArtifact(desired domain.DesiredState, path string) (string, bool) {
	for _, a := range desired.Artifacts {
		if a.Path == path {
			return a.Content, true
		}
	}
	return "", false
}

func hasError(diags []domain.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == domain.SeverityError {
			return true
		}
	}
	return false
}

func assertArtifactsSorted(t *testing.T, desired domain.DesiredState) {
	t.Helper()
	for i := 1; i < len(desired.Artifacts); i++ {
		if desired.Artifacts[i-1].Path > desired.Artifacts[i].Path {
			t.Fatalf("artifacts are not sorted: %s before %s", desired.Artifacts[i-1].Path, desired.Artifacts[i].Path)
		}
	}
}

func assertServicesSorted(t *testing.T, desired domain.DesiredState) {
	t.Helper()
	for i := 1; i < len(desired.Services); i++ {
		if desired.Services[i-1].Name > desired.Services[i].Name {
			t.Fatalf("services are not sorted: %s before %s", desired.Services[i-1].Name, desired.Services[i].Name)
		}
	}
}
