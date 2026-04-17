package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveArtifactsDirRemovesSafeDirectory(t *testing.T) {
	baseDir := t.TempDir()
	targetDir := filepath.Join(baseDir, "render")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "artifact.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}

	svc := New(Options{})
	if err := svc.RemoveArtifactsDir(targetDir); err != nil {
		t.Fatalf("remove artifacts dir: %v", err)
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("expected directory removal, stat err: %v", err)
	}
}

func TestRemoveArtifactsDirRejectsCurrentWorkingDirectory(t *testing.T) {
	baseDir := t.TempDir()
	restoreWD := mustChdir(t, baseDir)
	defer restoreWD()

	svc := New(Options{})
	err := svc.RemoveArtifactsDir(".")
	if err == nil {
		t.Fatalf("expected remove current dir to fail")
	}
	if !strings.Contains(err.Error(), "current working directory") {
		t.Fatalf("expected current directory guard, got: %v", err)
	}
}

func TestRemoveArtifactsDirRejectsParentContainingCurrentWorkingDirectory(t *testing.T) {
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	restoreWD := mustChdir(t, workDir)
	defer restoreWD()

	svc := New(Options{})
	err := svc.RemoveArtifactsDir("..")
	if err == nil {
		t.Fatalf("expected parent directory removal to fail")
	}
	if !strings.Contains(err.Error(), "current working directory") {
		t.Fatalf("expected parent guard, got: %v", err)
	}
}

func mustChdir(t *testing.T, dir string) func() {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Fatalf("restore cwd %s: %v", oldWD, chdirErr)
		}
	}
}
