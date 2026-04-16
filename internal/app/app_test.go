package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pantani/gorchestrator/internal/state"
)

func TestApplyDryRunDoesNotPersistState(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "state")
	outDir := filepath.Join(baseDir, "out")
	specPath := writeSpecFile(t, baseDir, "dryrun-cluster")

	application := New(Options{StateDir: stateDir})
	result, diags, err := application.Apply(context.Background(), specPath, ApplyOptions{OutputDir: outDir, DryRun: true})
	if err != nil {
		t.Fatalf("apply dry-run failed: %v", err)
	}
	if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors: %#v", diags)
	}
	if !result.DryRun {
		t.Fatalf("expected dry-run result")
	}
	if result.SnapshotUpdated {
		t.Fatalf("snapshot should not be updated in dry-run")
	}
	if result.ArtifactsWritten != 0 {
		t.Fatalf("expected no artifacts written in dry-run, got %d", result.ArtifactsWritten)
	}

	store := state.NewStore(stateDir)
	snap, err := store.Load("dryrun-cluster", "docker-compose")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snap != nil {
		t.Fatalf("expected no snapshot for dry-run")
	}
	if _, err := os.Stat(filepath.Join(outDir, "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected compose artifact not written in dry-run, stat err: %v", err)
	}
}

func TestApplyIdempotentStateSnapshot(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "state")
	outDir := filepath.Join(baseDir, "out")
	specPath := writeSpecFile(t, baseDir, "idempotent-cluster")

	application := New(Options{StateDir: stateDir})
	first, diags, err := application.Apply(context.Background(), specPath, ApplyOptions{OutputDir: outDir})
	if err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors in first apply: %#v", diags)
	}
	if !first.Plan.HasChanges() {
		t.Fatalf("expected first apply to have changes")
	}
	if !first.SnapshotUpdated {
		t.Fatalf("expected snapshot updated in first apply")
	}
	if first.ArtifactsWritten == 0 {
		t.Fatalf("expected artifacts to be written")
	}
	if _, err := os.Stat(filepath.Join(outDir, "compose.yaml")); err != nil {
		t.Fatalf("expected compose artifact written: %v", err)
	}

	second, diags, err := application.Apply(context.Background(), specPath, ApplyOptions{OutputDir: outDir})
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors in second apply: %#v", diags)
	}
	if second.Plan.HasChanges() {
		t.Fatalf("expected second apply to be converged, but plan has changes")
	}
}

func TestStatusAndDoctorBasic(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "state")
	outDir := filepath.Join(baseDir, "out")
	specPath := writeSpecFile(t, baseDir, "status-cluster")

	application := New(Options{StateDir: stateDir})

	statusBefore, diags, err := application.Status(context.Background(), specPath)
	if err != nil {
		t.Fatalf("status before apply failed: %v", err)
	}
	if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors in status before apply: %#v", diags)
	}
	if statusBefore.SnapshotExists {
		t.Fatalf("expected no snapshot before apply")
	}
	if !statusBefore.Plan.HasChanges() {
		t.Fatalf("expected pending changes before apply")
	}

	reportBefore, err := application.Doctor(context.Background(), specPath)
	if err != nil {
		t.Fatalf("doctor before apply failed: %v", err)
	}
	if reportBefore.HasFailures() {
		t.Fatalf("doctor should not fail for valid spec without snapshot")
	}
	if !reportBefore.HasWarnings() {
		t.Fatalf("expected doctor warning before apply due to missing snapshot")
	}

	if _, diags, err := application.Apply(context.Background(), specPath, ApplyOptions{OutputDir: outDir}); err != nil {
		t.Fatalf("apply failed: %v", err)
	} else if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors in apply: %#v", diags)
	}

	statusAfter, diags, err := application.Status(context.Background(), specPath)
	if err != nil {
		t.Fatalf("status after apply failed: %v", err)
	}
	if HasErrors(diags) {
		t.Fatalf("unexpected diagnostics with errors in status after apply: %#v", diags)
	}
	if !statusAfter.SnapshotExists {
		t.Fatalf("expected snapshot after apply")
	}
	if statusAfter.Plan.HasChanges() {
		t.Fatalf("expected converged status after apply")
	}

	reportAfter, err := application.Doctor(context.Background(), specPath)
	if err != nil {
		t.Fatalf("doctor after apply failed: %v", err)
	}
	if reportAfter.HasFailures() {
		t.Fatalf("doctor should not fail after apply")
	}
}

func writeSpecFile(t *testing.T, dir, clusterName string) string {
	t.Helper()
	path := filepath.Join(dir, "cluster.yaml")
	raw := "apiVersion: bgorch.io/v1alpha1\n" +
		"kind: ChainCluster\n" +
		"metadata:\n" +
		"  name: " + clusterName + "\n" +
		"spec:\n" +
		"  family: generic\n" +
		"  plugin: generic-process\n" +
		"  runtime:\n" +
		"    backend: docker-compose\n" +
		"  nodePools:\n" +
		"    - name: nodes\n" +
		"      replicas: 1\n" +
		"      template:\n" +
		"        workloads:\n" +
		"          - name: daemon\n" +
		"            mode: container\n" +
		"            image: alpine:3.20\n" +
		"            command: [\"sh\", \"-c\"]\n" +
		"            args: [\"sleep 3600\"]\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
	return path
}
