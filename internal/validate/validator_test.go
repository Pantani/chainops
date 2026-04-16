package validate

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
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

func mustParse(t *testing.T, raw string) *v1alpha1.ChainCluster {
	t.Helper()
	var c v1alpha1.ChainCluster
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	spec.ApplyDefaults(&c)
	return &c
}
