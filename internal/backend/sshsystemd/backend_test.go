package sshsystemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pantani/gorchestrator/internal/backend"
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

func TestExecuteRuntimeSuccess(t *testing.T) {
	desired := loadDesiredFromExample(t, filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	outputDir := t.TempDir()
	writeArtifacts(t, outputDir, desired)

	runner := &fakeRunner{
		handlers: []runHandler{
			{
				match: func(name string, args []string) bool {
					return name == "ssh" && hasArg(args, "systemctl") && hasArg(args, "--version")
				},
				output: "systemd 252 (252.30-1)\n",
			},
		},
	}

	backendImpl := NewWithRunner(runner)
	res, err := backendImpl.ExecuteRuntime(context.Background(), backend.RuntimeApplyRequest{
		ClusterName: desired.ClusterName,
		OutputDir:   outputDir,
		Desired:     desired,
	})
	if err != nil {
		t.Fatalf("execute runtime: %v", err)
	}
	if !strings.Contains(res.Command, "ssh -p 22 bgorch@validators systemctl --version") {
		t.Fatalf("unexpected command: %s", res.Command)
	}
	if !strings.Contains(res.Output, "systemd 252") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
	}
}

func TestExecuteRuntimeMissingArtifacts(t *testing.T) {
	desired := loadDesiredFromExample(t, filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	outputDir := t.TempDir()

	backendImpl := NewWithRunner(&fakeRunner{})
	_, err := backendImpl.ExecuteRuntime(context.Background(), backend.RuntimeApplyRequest{
		ClusterName: desired.ClusterName,
		OutputDir:   outputDir,
		Desired:     desired,
	})
	if err == nil {
		t.Fatalf("expected error for missing artifacts")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "rendered ssh-systemd artifact") {
		t.Fatalf("expected actionable missing artifact error, got: %v", err)
	}
}

func TestExecuteRuntimeFallbackWhenSSHMissing(t *testing.T) {
	desired := loadDesiredFromExample(t, filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	outputDir := t.TempDir()
	writeArtifacts(t, outputDir, desired)

	backendImpl := NewWithRunner(&fakeRunner{
		handlers: []runHandler{
			{
				match: func(name string, args []string) bool {
					return name == "ssh"
				},
				err: fmt.Errorf("ssh: %w", exec.ErrNotFound),
			},
		},
	})

	_, err := backendImpl.ExecuteRuntime(context.Background(), backend.RuntimeApplyRequest{
		ClusterName: desired.ClusterName,
		OutputDir:   outputDir,
		Desired:     desired,
	})
	if err == nil {
		t.Fatalf("expected error when ssh is unavailable")
	}
	if !strings.Contains(err.Error(), "install OpenSSH client") {
		t.Fatalf("expected actionable ssh missing message, got: %v", err)
	}
}

func TestObserveRuntimeSuccess(t *testing.T) {
	desired := loadDesiredFromExample(t, filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	outputDir := t.TempDir()
	writeArtifacts(t, outputDir, desired)

	runner := &fakeRunner{
		handlers: []runHandler{
			{
				match: func(name string, args []string) bool {
					return name == "ssh" && hasArg(args, "list-units")
				},
				output: "generic-ssh-validator-daemon.service loaded active running bgorch unit\n",
			},
		},
	}

	backendImpl := NewWithRunner(runner)
	res, err := backendImpl.ObserveRuntime(context.Background(), backend.RuntimeObserveRequest{
		ClusterName: desired.ClusterName,
		OutputDir:   outputDir,
		Desired:     desired,
	})
	if err != nil {
		t.Fatalf("observe runtime: %v", err)
	}
	if res.Summary != "observed 1 target(s) and 1 unit(s)" {
		t.Fatalf("unexpected summary: %s", res.Summary)
	}
	if len(res.Details) == 0 || !strings.Contains(res.Details[0], "validators:") {
		t.Fatalf("expected prefixed details, got: %#v", res.Details)
	}
}

func TestObserveRuntimeFallbackWhenSystemctlMissing(t *testing.T) {
	desired := loadDesiredFromExample(t, filepath.Join("..", "..", "..", "examples", "generic-single-ssh-systemd.yaml"))
	outputDir := t.TempDir()
	writeArtifacts(t, outputDir, desired)

	backendImpl := NewWithRunner(&fakeRunner{
		handlers: []runHandler{
			{
				match: func(name string, args []string) bool {
					return name == "ssh" && hasArg(args, "list-units")
				},
				output: "bash: systemctl: command not found\n",
				err:    errors.New("exit status 127"),
			},
		},
	})

	_, err := backendImpl.ObserveRuntime(context.Background(), backend.RuntimeObserveRequest{
		ClusterName: desired.ClusterName,
		OutputDir:   outputDir,
		Desired:     desired,
	})
	if err == nil {
		t.Fatalf("expected error when systemctl is missing on target host")
	}
	if !strings.Contains(err.Error(), "missing systemctl") {
		t.Fatalf("expected actionable systemctl error, got: %v", err)
	}
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

func loadDesiredFromExample(t *testing.T, path string) domain.DesiredState {
	t.Helper()
	cluster, err := spec.LoadFromFile(path)
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

	backendImpl := New()
	desired, err := backendImpl.BuildDesired(context.Background(), cluster, pluginOut)
	if err != nil {
		t.Fatalf("build desired: %v", err)
	}
	return desired
}

func writeArtifacts(t *testing.T, outputDir string, desired domain.DesiredState) {
	t.Helper()
	for _, artifact := range desired.Artifacts {
		path := filepath.Join(outputDir, artifact.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(artifact.Content), 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", artifact.Path, err)
		}
	}
}

type runHandler struct {
	match  func(name string, args []string) bool
	output string
	err    error
}

type runCall struct {
	name string
	args []string
}

type fakeRunner struct {
	handlers []runHandler
	calls    []runCall
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	cp := append([]string{}, args...)
	r.calls = append(r.calls, runCall{name: name, args: cp})
	for _, h := range r.handlers {
		if h.match != nil && !h.match(name, args) {
			continue
		}
		return h.output, h.err
	}
	return "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
}

func hasArg(args []string, needle string) bool {
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}
