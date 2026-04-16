package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunApplyStatusDoctor(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	workdir := t.TempDir()
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	specPath := writeSpecFile(t, workdir, "cli-cluster")

	if code := Run([]string{"apply", "-f", specPath, "--dry-run"}); code != 0 {
		t.Fatalf("apply --dry-run returned code %d", code)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".bgorch", "render", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write compose artifact, stat err: %v", err)
	}

	if code := Run([]string{"status", "-f", specPath}); code != 0 {
		t.Fatalf("status returned code %d", code)
	}

	if code := Run([]string{"doctor", "-f", specPath}); code != 0 {
		t.Fatalf("doctor returned code %d", code)
	}
}

func TestRunApplyRejectsDryRunWithRuntimeExec(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	workdir := t.TempDir()
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	specPath := writeSpecFile(t, workdir, "cli-runtime-conflict")
	if code := Run([]string{"apply", "-f", specPath, "--dry-run", "--runtime-exec"}); code != 2 {
		t.Fatalf("expected argument error code 2, got %d", code)
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
