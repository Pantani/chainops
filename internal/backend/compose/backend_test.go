package compose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pantani/gorchestrator/internal/backend"
	"github.com/Pantani/gorchestrator/internal/chain/genericprocess"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

func TestComposeRenderGoldenSingle(t *testing.T) {
	runComposeGoldenTest(t, "generic-single-compose.yaml", "compose-single.golden.yaml")
}

func TestComposeRenderGoldenMultiProcess(t *testing.T) {
	runComposeGoldenTest(t, "generic-multiprocess-compose.yaml", "compose-multi.golden.yaml")
}

func TestComposeValidateTargetRejectsInvalidOutputFile(t *testing.T) {
	t.Parallel()

	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "generic-single-compose.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	cluster.Spec.Runtime.BackendConfig.Compose.OutputFile = "../escape.yaml"

	backendImpl := New()
	diags := backendImpl.ValidateTarget(cluster)
	found := false
	for _, d := range diags {
		if d.Severity == domain.SeverityError && d.Path == "spec.runtime.backendConfig.compose.outputFile" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected outputFile validation error, got: %#v", diags)
	}
}

func TestComposeBuildDesiredRejectsInvalidOutputFile(t *testing.T) {
	t.Parallel()

	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "generic-single-compose.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	cluster.Spec.Runtime.BackendConfig.Compose.OutputFile = "/tmp/compose.yaml"

	plugin := genericprocess.New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	pluginOut, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("plugin build: %v", err)
	}

	backendImpl := New()
	_, err = backendImpl.BuildDesired(context.Background(), cluster, pluginOut)
	if err == nil {
		t.Fatalf("expected invalid compose output file error")
	}
	if !strings.Contains(err.Error(), "invalid compose output file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func runComposeGoldenTest(t *testing.T, exampleFile, goldenFile string) {
	t.Helper()

	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", exampleFile))
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

	var compose string
	for _, a := range desired.Artifacts {
		if a.Path == "compose.yaml" {
			compose = a.Content
			break
		}
	}
	if compose == "" {
		t.Fatalf("compose artifact not found")
	}

	goldenPath := filepath.Join("..", "..", "..", "test", "golden", goldenFile)
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if compose != string(expected) {
		t.Fatalf("compose output mismatch\n--- got ---\n%s\n--- expected ---\n%s", compose, string(expected))
	}
}

func TestComposeExecuteRuntimeBuildsDockerCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: "started"}
	b := NewWithRunner(runner)
	outDir := t.TempDir()
	composePath := filepath.Join(outDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	result, err := b.ExecuteRuntime(context.Background(), backend.RuntimeApplyRequest{
		ClusterName: "runtime-cluster",
		OutputDir:   outDir,
		Desired: domain.DesiredState{
			ClusterName: "runtime-cluster",
			Backend:     b.Name(),
			Metadata:    map[string]string{"compose.project": "runtime-cluster", "compose.file": "compose.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("execute runtime: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "docker" {
		t.Fatalf("expected docker command, got %q", call.name)
	}
	gotArgs := strings.Join(call.args, " ")
	if !strings.Contains(gotArgs, "compose -p runtime-cluster -f "+composePath+" up -d") {
		t.Fatalf("unexpected args: %s", gotArgs)
	}
	if !strings.Contains(result.Command, "docker compose") {
		t.Fatalf("expected runtime command in result, got %q", result.Command)
	}
}

func TestComposeObserveRuntimeFallback(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errors.New("docker not available")}
	b := NewWithRunner(runner)
	outDir := t.TempDir()
	composePath := filepath.Join(outDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	_, err := b.ObserveRuntime(context.Background(), backend.RuntimeObserveRequest{
		ClusterName: "runtime-cluster",
		OutputDir:   outDir,
		Desired: domain.DesiredState{
			ClusterName: "runtime-cluster",
			Backend:     b.Name(),
			Metadata:    map[string]string{"compose.project": "runtime-cluster", "compose.file": "compose.yaml"},
		},
	})
	if err == nil {
		t.Fatalf("expected observe runtime error")
	}
	if !strings.Contains(err.Error(), "docker compose runtime observe failed") {
		t.Fatalf("expected actionable runtime observe error, got %v", err)
	}
}

func TestComposeFilePathFallsBackOnInvalidMetadata(t *testing.T) {
	t.Parallel()

	desired := domain.DesiredState{
		Metadata: map[string]string{"compose.file": "../escape.yaml"},
	}
	if got := composeFilePath(desired); got != "compose.yaml" {
		t.Fatalf("expected safe fallback compose path, got %q", got)
	}
}

type fakeRunner struct {
	output string
	err    error
	calls  []fakeRunnerCall
}

type fakeRunnerCall struct {
	dir  string
	name string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	r.calls = append(r.calls, fakeRunnerCall{dir: dir, name: name, args: append([]string{}, args...)})
	return r.output, r.err
}
