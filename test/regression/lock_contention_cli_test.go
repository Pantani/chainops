package regression_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type cliResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func TestCLIApplyFailsWhenStateLockIsHeld(t *testing.T) {
	repo := findRepoRoot(t)
	bin := buildCLI(t, repo)
	workDir := t.TempDir()
	specPath := filepath.Join(repo, "examples", "generic-single-compose.yaml")

	stateDir := filepath.Join(workDir, ".bgorch", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	lockPath := filepath.Join(stateDir, "generic-single--docker-compose.lock")
	lockData := []byte("pid=99999\ntime=2026-01-01T00:00:00Z\n")
	if err := os.WriteFile(lockPath, lockData, 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	res := runCLI(t, bin, workDir, "apply", "-f", specPath, "--dry-run", "-o", filepath.Join(workDir, "render"))
	if res.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "state lock is already held") {
		t.Fatalf("expected lock contention error in stderr, got:\n%s", res.Stderr)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected preexisting lock file to remain untouched (%q): %v", lockPath, err)
	}
	snapshotPath := filepath.Join(stateDir, "generic-single--docker-compose.json")
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("no snapshot should be written when lock is held, found %q", snapshotPath)
	}
}

func TestCLIApplyRuntimeExecFailureReleasesLock(t *testing.T) {
	repo := findRepoRoot(t)
	bin := buildCLI(t, repo)
	workDir := t.TempDir()
	specPath := filepath.Join(repo, "examples", "generic-single-ssh-systemd.yaml")
	renderDir := filepath.Join(workDir, "render")

	res := runCLI(t, bin, workDir, "apply", "-f", specPath, "--runtime-exec", "-o", renderDir)
	if res.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "runtime preflight failed") {
		t.Fatalf("expected runtime preflight failure, got:\n%s", res.Stderr)
	}

	stateDir := filepath.Join(workDir, ".bgorch", "state")
	lockPath := filepath.Join(stateDir, "generic-ssh--ssh-systemd.lock")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file should be released after failed apply, found %q", lockPath)
	}

	snapshotPath := filepath.Join(stateDir, "generic-ssh--ssh-systemd.json")
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("snapshot should not be written on runtime execution failure, found %q", snapshotPath)
	}

	// Ensure a follow-up apply can reacquire lock and proceed.
	res = runCLI(t, bin, workDir, "apply", "-f", specPath, "--dry-run", "-o", filepath.Join(workDir, "render-dry"))
	if res.ExitCode != 0 {
		t.Fatalf("expected follow-up dry-run apply to succeed, got %d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
}

func buildCLI(t *testing.T, repoRoot string) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "bgorch")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/bgorch")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}
	return binPath
}

func runCLI(t *testing.T, binaryPath, workDir string, args ...string) cliResult {
	t.Helper()
	homeDir := filepath.Join(workDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
		"XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(homeDir, ".cache"),
		"XDG_DATA_HOME="+filepath.Join(homeDir, ".local", "share"),
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	result := cliResult{Stdout: stdout.String(), Stderr: stderr.String()}
	err := cmd.Run()
	if err == nil {
		result.ExitCode = 0
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		return result
	}

	t.Fatalf("run command %q failed: %v", strings.Join(args, " "), err)
	return cliResult{}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %q", dir)
		}
		dir = parent
	}
}
