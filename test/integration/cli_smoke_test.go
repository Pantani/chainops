package integration_test

import (
	"bytes"
	"errors"
	"fmt"
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

func TestCLISmokeExamples(t *testing.T) {
	repo := findRepoRoot(t)
	bin := buildCLI(t, repo)

	tests := []struct {
		name              string
		specRel           string
		clusterName       string
		backend           string
		expectedArtifacts []string
	}{
		{
			name:        "compose-single",
			specRel:     "examples/generic-single-compose.yaml",
			clusterName: "generic-single",
			backend:     "docker-compose",
			expectedArtifacts: []string{
				"compose.yaml",
				"global/scripts/entrypoint.sh",
				"nodes/rpc/config/node.toml",
			},
		},
		{
			name:        "ssh-systemd-single",
			specRel:     "examples/generic-single-ssh-systemd.yaml",
			clusterName: "generic-ssh",
			backend:     "ssh-systemd",
			expectedArtifacts: []string{
				"nodes/validator/config/config.toml",
				"ssh-systemd/nodes/validator/env/generic-ssh-validator-daemon.env",
				"ssh-systemd/nodes/validator/layout/directories.txt",
				"ssh-systemd/nodes/validator/systemd/generic-ssh-validator-daemon.service",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			specPath := filepath.Join(repo, tc.specRel)

			res := runCLI(t, bin, workDir, "validate", "-f", specPath)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "validation passed")

			renderDir := filepath.Join(workDir, "render")
			res = runCLI(t, bin, workDir, "render", "-f", specPath, "-o", renderDir)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "rendered")
			for _, artifact := range tc.expectedArtifacts {
				path := filepath.Join(renderDir, artifact)
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("expected artifact %q to exist: %v", path, err)
				}
			}

			res = runCLI(t, bin, workDir, "plan", "-f", specPath)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "plan:")

			dryRunDir := filepath.Join(workDir, "render-dry")
			res = runCLI(t, bin, workDir, "apply", "-f", specPath, "--dry-run", "-o", dryRunDir)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "dry-run enabled")
			if _, err := os.Stat(dryRunDir); !os.IsNotExist(err) {
				t.Fatalf("dry-run should not write artifacts, got %q created", dryRunDir)
			}

			stateDir := filepath.Join(workDir, ".bgorch", "state")
			snapshotPath := filepath.Join(stateDir, fmt.Sprintf("%s--%s.json", tc.clusterName, tc.backend))
			if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
				t.Fatalf("dry-run should not write snapshot, found %q", snapshotPath)
			}
			lockPath := filepath.Join(stateDir, fmt.Sprintf("%s--%s.lock", tc.clusterName, tc.backend))
			if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
				t.Fatalf("lock file should be released after apply dry-run, found %q", lockPath)
			}

			res = runCLI(t, bin, workDir, "status", "-f", specPath)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "snapshot: not found")

			res = runCLI(t, bin, workDir, "doctor", "-f", specPath)
			requireExitCode(t, res, 0)
			requireContains(t, res.Stdout, "[PASS] spec.validation")
		})
	}
}

func TestCLIRuntimeFlagsFallbackSafe(t *testing.T) {
	repo := findRepoRoot(t)
	bin := buildCLI(t, repo)

	t.Run("apply rejects conflicting runtime flags", func(t *testing.T) {
		workDir := t.TempDir()
		specPath := filepath.Join(repo, "examples", "generic-single-compose.yaml")
		res := runCLI(t, bin, workDir, "apply", "-f", specPath, "--dry-run", "--runtime-exec")
		requireExitCode(t, res, 2)
		requireContains(t, res.Stderr, "--dry-run cannot be combined with --runtime-exec")
	})

	t.Run("status observe-runtime compose degrades safely when compose file is missing", func(t *testing.T) {
		workDir := t.TempDir()
		specPath := filepath.Join(repo, "examples", "generic-single-compose.yaml")
		res := runCLI(
			t,
			bin,
			workDir,
			"status",
			"-f",
			specPath,
			"--observe-runtime",
			"-o",
			filepath.Join(workDir, "missing-render"),
		)
		requireExitCode(t, res, 0)
		requireContains(t, res.Stdout, "runtime observe error:")
		requireContains(t, res.Stdout, "compose file not found")
	})

	t.Run("doctor observe-runtime compose degrades safely when compose file is missing", func(t *testing.T) {
		workDir := t.TempDir()
		specPath := filepath.Join(repo, "examples", "generic-single-compose.yaml")

		applyDir := filepath.Join(workDir, "render-initial")
		res := runCLI(t, bin, workDir, "apply", "-f", specPath, "-o", applyDir)
		requireExitCode(t, res, 0)
		requireContains(t, res.Stdout, "state snapshot updated")

		res = runCLI(
			t,
			bin,
			workDir,
			"doctor",
			"-f",
			specPath,
			"--observe-runtime",
			"-o",
			filepath.Join(workDir, "missing-render"),
		)
		requireExitCode(t, res, 0)
		requireContains(t, res.Stdout, "[WARN] runtime.observe")
		requireContains(t, res.Stdout, "compose file not found")
	})

	t.Run("status observe-runtime for ssh-systemd degrades safely when artifacts are missing", func(t *testing.T) {
		workDir := t.TempDir()
		specPath := filepath.Join(repo, "examples", "generic-single-ssh-systemd.yaml")
		res := runCLI(t, bin, workDir, "status", "-f", specPath, "--observe-runtime")
		requireExitCode(t, res, 0)
		requireContains(t, res.Stdout, "runtime observe error:")
		requireContains(t, res.Stdout, "missing")
	})
}

func TestCLICometBFTTypedPluginConfigSmoke(t *testing.T) {
	repo := findRepoRoot(t)
	bin := buildCLI(t, repo)
	workDir := t.TempDir()

	specPath := filepath.Join(workDir, "cometbft-pluginconfig-smoke.yaml")
	spec := `apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: cometbft-pluginconfig-smoke
spec:
  family: cometbft
  plugin: cometbft-family
  profile: validator-single
  runtime:
    backend: docker-compose
    backendConfig:
      compose:
        projectName: cometbft-pluginconfig-smoke
        outputFile: compose.yaml
  pluginConfig:
    genericProcess:
      extraFiles:
        - path: ignored/by-cometbft.txt
          content: should-not-render
  nodePools:
    - name: validator
      replicas: 1
      roles: [validator]
      template:
        role: validator
        volumes:
          - name: datadir
            type: named
          - name: configdir
            type: named
        workloads:
          - name: cometbft
            mode: container
            image: ghcr.io/cometbft/cometbft:v0.38.17
            command: ["cometbft"]
            args: ["start", "--home", "/cometbft"]
            ports:
              - name: p2p
                containerPort: 26656
              - name: rpc
                containerPort: 26657
            volumeMounts:
              - volume: datadir
                path: /cometbft/data
              - volume: configdir
                path: /cometbft/config
            restartPolicy: unless-stopped
`
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	res := runCLI(t, bin, workDir, "validate", "-f", specPath)
	requireExitCode(t, res, 0)
	requireContains(t, res.Stdout, "[WARNING] genericProcess config provided for non-generic plugin")
	requireContains(t, res.Stdout, "validation passed")

	renderDir := filepath.Join(workDir, "render")
	res = runCLI(t, bin, workDir, "render", "-f", specPath, "-o", renderDir)
	requireExitCode(t, res, 0)
	requireContains(t, res.Stdout, "rendered")

	expectedArtifacts := []string{
		"compose.yaml",
		"nodes/validator/config/config.toml",
		"nodes/validator/config/app.toml",
		"nodes/validator/config/genesis.json",
	}
	for _, artifact := range expectedArtifacts {
		path := filepath.Join(renderDir, artifact)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %q to exist: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(renderDir, "ignored", "by-cometbft.txt")); !os.IsNotExist(err) {
		t.Fatalf("cometbft plugin should ignore genericProcess extraFiles, but artifact was rendered")
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

func requireExitCode(t *testing.T, got cliResult, want int) {
	t.Helper()
	if got.ExitCode != want {
		t.Fatalf("unexpected exit code: got=%d want=%d\nstdout:\n%s\nstderr:\n%s", got.ExitCode, want, got.Stdout, got.Stderr)
	}
}

func requireContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("expected output to contain %q\noutput:\n%s", want, output)
	}
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
