package validate

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

func TestClusterValidationValidSpec(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  nodePools:
    - name: rpc
      replicas: 1
      template:
        volumes:
          - name: datadir
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
            volumeMounts:
              - volume: datadir
                path: /var/lib/node
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	for _, d := range diags {
		if d.Severity == "error" {
			t.Fatalf("expected no errors, got %v", diags)
		}
	}
}

func TestClusterValidationInvalidSpec(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: INVALID_NAME
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  nodePools:
    - name: rpc
      replicas: 1
      template:
        workloads:
          - name: node
            mode: container
            ports:
              - containerPort: 70000
            volumeMounts:
              - volume: missing
                path: relative/path
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if len(diags) == 0 {
		t.Fatalf("expected diagnostics, got none")
	}
	hasError := false
	for _, d := range diags {
		if d.Severity == "error" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Fatalf("expected at least one error diagnostic, got %v", diags)
	}
}

func TestClusterValidationWarnsCometConfigOnNonCometPlugin(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  pluginConfig:
    cometBFT:
      chainID: localnet-1
  nodePools:
    - name: rpc
      replicas: 1
      template:
        volumes:
          - name: datadir
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
            volumeMounts:
              - volume: datadir
                path: /var/lib/node
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "warning", "spec.pluginConfig.cometBFT") {
		t.Fatalf("expected warning for cometBFT config on non-comet plugin, got %v", diags)
	}
}

func TestClusterValidationValidatesCometTypedConfig(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: comet
spec:
  family: cometbft
  plugin: cometbft-family
  runtime:
    backend: docker-compose
  pluginConfig:
    cometBFT:
      p2pPort: 70000
      logLevel: loud
      pruning: random
  nodePools:
    - name: validator
      replicas: 1
      template:
        volumes:
          - name: datadir
        workloads:
          - name: cometbft
            mode: container
            image: ghcr.io/cometbft/cometbft:v0.38.17
            volumeMounts:
              - volume: datadir
                path: /cometbft/data
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.cometBFT.p2pPort") {
		t.Fatalf("expected p2pPort validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.cometBFT.logLevel") {
		t.Fatalf("expected logLevel validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.cometBFT.pruning") {
		t.Fatalf("expected pruning validation error, got %v", diags)
	}
}

func TestClusterValidationWarnsEVMConfigOnNonEVMPlugin(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  pluginConfig:
    evm:
      client: geth
  nodePools:
    - name: rpc
      replicas: 1
      template:
        volumes:
          - name: datadir
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
            volumeMounts:
              - volume: datadir
                path: /var/lib/node
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "warning", "spec.pluginConfig.evm") {
		t.Fatalf("expected warning for evm config on non-evm plugin, got %v", diags)
	}
}

func TestClusterValidationValidatesNewFamilyTypedConfigs(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: multichain
spec:
  family: evm
  plugin: evm-family
  runtime:
    backend: docker-compose
  pluginConfig:
    evm:
      client: unknown
      network: unknown
      syncMode: unknown
      p2pPort: 70000
      chainID: -1
  nodePools:
    - name: node
      replicas: 1
      template:
        workloads:
          - name: geth
            mode: container
            image: ethereum/client-go:v1.14.13
            pluginConfig:
              solana:
                dynamicPortRange: invalid
              bitcoin:
                network: unknown
              cosmos:
                chainID: "bad id"
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)

	if !containsDiagnostic(diags, "error", "spec.pluginConfig.evm.client") {
		t.Fatalf("expected evm client validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.evm.network") {
		t.Fatalf("expected evm network validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.evm.syncMode") {
		t.Fatalf("expected evm sync mode validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.evm.p2pPort") {
		t.Fatalf("expected evm port validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.pluginConfig.evm.chainID") {
		t.Fatalf("expected evm chainID validation error, got %v", diags)
	}
	if !containsDiagnostic(diags, "warning", "spec.nodePools[0].template.workloads[0].pluginConfig.solana") {
		t.Fatalf("expected warning for solana config on evm plugin, got %v", diags)
	}
	if !containsDiagnostic(diags, "warning", "spec.nodePools[0].template.workloads[0].pluginConfig.bitcoin") {
		t.Fatalf("expected warning for bitcoin config on evm plugin, got %v", diags)
	}
	if !containsDiagnostic(diags, "warning", "spec.nodePools[0].template.workloads[0].pluginConfig.cosmos") {
		t.Fatalf("expected warning for cosmos config on evm plugin, got %v", diags)
	}
}

func TestClusterValidationRejectsDuplicateExpandedNodeNames(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  nodePools:
    - name: validators-a
      replicas: 1
      template:
        name: validator
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
    - name: validators-b
      replicas: 1
      template:
        name: validator
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "error", "spec.nodePools[1].template.name") {
		t.Fatalf("expected duplicate expanded node name error, got %v", diags)
	}
}

func TestClusterValidationRejectsInvalidAndDuplicateEnvNames(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  nodePools:
    - name: rpc
      replicas: 1
      template:
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
            env:
              - name: VALID_NAME
                value: one
              - name: "BAD NAME"
                value: bad
              - name: VALID_NAME
                value: two
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "error", "spec.nodePools[0].template.workloads[0].env[1].name") {
		t.Fatalf("expected invalid env name error, got %v", diags)
	}
	if !containsDiagnostic(diags, "error", "spec.nodePools[0].template.workloads[0].env[2].name") {
		t.Fatalf("expected duplicate env name error, got %v", diags)
	}
}

func TestClusterValidationRejectsInvalidRestartPolicy(t *testing.T) {
	specYAML := `
apiVersion: bgorch.io/v1alpha1
kind: ChainCluster
metadata:
  name: test-chain
spec:
  family: generic
  plugin: generic-process
  runtime:
    backend: docker-compose
  nodePools:
    - name: rpc
      replicas: 1
      template:
        workloads:
          - name: node
            mode: container
            image: ghcr.io/example/node:v1
            restartPolicy: unsupported
`
	c := mustParse(t, specYAML)
	diags := Cluster(c)
	if !containsDiagnostic(diags, "error", "spec.nodePools[0].template.workloads[0].restartPolicy") {
		t.Fatalf("expected invalid restartPolicy error, got %v", diags)
	}
}

func mustParse(t *testing.T, raw string) *v1alpha1.ChainCluster {
	t.Helper()
	var c v1alpha1.ChainCluster
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	spec.ApplyDefaults(&c)
	return &c
}

func containsDiagnostic(diags []domain.Diagnostic, severity, path string) bool {
	for _, d := range diags {
		if strings.EqualFold(string(d.Severity), severity) && d.Path == path {
			return true
		}
	}
	return false
}
