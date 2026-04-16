package cometbft

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

func TestValidateSingleValidatorExample(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-single-validator.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	plugin := New()
	diags := plugin.Validate(cluster)
	if hasError(diags) {
		t.Fatalf("unexpected validation errors: %#v", diags)
	}
}

func TestValidateRejectsMissingCometWorkload(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-single-validator.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	cluster.Spec.NodePools[0].Template.Workloads[0].Name = "node"
	cluster.Spec.NodePools[0].Template.Workloads[0].Image = "ghcr.io/example/chaind:v0.1.0"
	cluster.Spec.NodePools[0].Template.Workloads[0].Command = []string{"chaind"}
	cluster.Spec.NodePools[0].Template.Workloads[0].Args = []string{"start"}

	plugin := New()
	diags := plugin.Validate(cluster)
	if !hasError(diags) {
		t.Fatalf("expected validation error, got: %#v", diags)
	}
	if !hasPath(diags, "spec.nodePools[0].template.workloads") {
		t.Fatalf("expected workload-level validation error, got: %#v", diags)
	}
}

func TestNormalizeDefaultsAndRoleInference(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-sentry-rpc.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	cluster.Spec.Family = ""
	cluster.Spec.Profile = ""
	cluster.Spec.NodePools[1].Roles = nil
	cluster.Spec.NodePools[1].Template.Role = ""

	plugin := New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	if cluster.Spec.Family != FamilyName {
		t.Fatalf("expected family %q, got %q", FamilyName, cluster.Spec.Family)
	}
	if cluster.Spec.Profile != "validator-single" {
		t.Fatalf("expected default profile validator-single, got %q", cluster.Spec.Profile)
	}
	if cluster.Spec.NodePools[1].Template.Role != "sentry" {
		t.Fatalf("expected inferred sentry role, got %q", cluster.Spec.NodePools[1].Template.Role)
	}
}

func TestBuildConsumesTypedCometConfig(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-single-validator.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	cluster.Spec.PluginConfig.CometBFT = &v1alpha1.CometBFTConfig{
		ChainID:          "custom-localnet-1",
		LogLevel:         "debug",
		Pruning:          "nothing",
		MinimumGasPrices: "0.025stake",
		PersistentPeers:  []string{"nodeid@validator:26656"},
	}
	cluster.Spec.NodePools[0].Template.PluginConfig.CometBFT = &v1alpha1.CometBFTConfig{
		RPCPort:      27657,
		ProxyAppPort: 36658,
	}
	cluster.Spec.NodePools[0].Template.Workloads[0].PluginConfig.CometBFT = &v1alpha1.CometBFTConfig{
		APIEnabled:  ptrBool(false),
		GRPCEnabled: ptrBool(false),
	}

	plugin := New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	out, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	configToml, ok := findArtifact(out.Artifacts, "nodes/validator/config/config.toml")
	if !ok {
		t.Fatalf("validator config artifact not found")
	}
	if !strings.Contains(configToml, "log_level = \"debug\"") {
		t.Fatalf("expected log_level override, got:\n%s", configToml)
	}
	if !strings.Contains(configToml, "laddr = \"tcp://0.0.0.0:27657\"") {
		t.Fatalf("expected rpc port override, got:\n%s", configToml)
	}
	if !strings.Contains(configToml, "proxy_app = \"tcp://127.0.0.1:36658\"") {
		t.Fatalf("expected proxy app port override, got:\n%s", configToml)
	}
	if !strings.Contains(configToml, "persistent_peers = \"nodeid@validator:26656\"") {
		t.Fatalf("expected persistent peers override, got:\n%s", configToml)
	}

	appToml, ok := findArtifact(out.Artifacts, "nodes/validator/config/app.toml")
	if !ok {
		t.Fatalf("validator app artifact not found")
	}
	if !strings.Contains(appToml, "chain-id = \"custom-localnet-1\"") {
		t.Fatalf("expected chainID override, got:\n%s", appToml)
	}
	if !strings.Contains(appToml, "minimum-gas-prices = \"0.025stake\"") {
		t.Fatalf("expected minimum gas override, got:\n%s", appToml)
	}
	if !strings.Contains(appToml, "pruning = \"nothing\"") {
		t.Fatalf("expected pruning override, got:\n%s", appToml)
	}
	if !strings.Contains(appToml, "[api]\nenable = false") {
		t.Fatalf("expected api disable override, got:\n%s", appToml)
	}
	if !strings.Contains(appToml, "[grpc]\nenable = false") {
		t.Fatalf("expected grpc disable override, got:\n%s", appToml)
	}
}

func TestBuildBackwardCompatibilityWithoutCometConfig(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-single-validator.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	cluster.Spec.PluginConfig.CometBFT = nil
	cluster.Spec.NodePools[0].Template.PluginConfig.CometBFT = nil
	cluster.Spec.NodePools[0].Template.Workloads[0].PluginConfig.CometBFT = nil

	plugin := New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	out, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	appToml, ok := findArtifact(out.Artifacts, "nodes/validator/config/app.toml")
	if !ok {
		t.Fatalf("validator app artifact not found")
	}
	if !strings.Contains(appToml, "chain-id = \"cometbft-single-localnet\"") {
		t.Fatalf("expected default chain-id fallback, got:\n%s", appToml)
	}
}

func TestBuildGoldenSingleValidator(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-single-validator.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	plugin := New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	out, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	assertArtifactsSorted(t, out.Artifacts)
	assertArtifactMatchesGolden(t, out.Artifacts, "nodes/validator/config/config.toml", "config-validator.golden")
	assertArtifactMatchesGolden(t, out.Artifacts, "nodes/validator/config/app.toml", "app-validator.golden")
}

func TestBuildGoldenSentryTopology(t *testing.T) {
	cluster, err := spec.LoadFromFile(filepath.Join("..", "..", "..", "examples", "cometbft-sentry-rpc.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	plugin := New()
	if err := plugin.Normalize(cluster); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	outFirst, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("build first pass: %v", err)
	}
	outSecond, err := plugin.Build(context.Background(), cluster)
	if err != nil {
		t.Fatalf("build second pass: %v", err)
	}

	if !reflect.DeepEqual(outFirst.Artifacts, outSecond.Artifacts) {
		t.Fatalf("build output is not deterministic")
	}

	assertArtifactsSorted(t, outFirst.Artifacts)
	assertArtifactMatchesGolden(t, outFirst.Artifacts, "nodes/sentry-00/config/config.toml", "config-sentry.golden")
	assertArtifactMatchesGolden(t, outFirst.Artifacts, "nodes/sentry-00/config/app.toml", "app-sentry.golden")
}

func assertArtifactMatchesGolden(t *testing.T, artifacts []domain.Artifact, artifactPath, goldenName string) {
	t.Helper()

	got, ok := findArtifact(artifacts, artifactPath)
	if !ok {
		t.Fatalf("artifact %q not found", artifactPath)
	}

	expected, err := os.ReadFile(filepath.Join("testdata", goldenName))
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenName, err)
	}

	if got != string(expected) {
		t.Fatalf("artifact mismatch for %s\n--- got ---\n%s\n--- expected ---\n%s", artifactPath, got, string(expected))
	}
}

func findArtifact(artifacts []domain.Artifact, path string) (string, bool) {
	for _, artifact := range artifacts {
		if artifact.Path == path {
			return artifact.Content, true
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

func hasPath(diags []domain.Diagnostic, path string) bool {
	for _, d := range diags {
		if d.Path == path {
			return true
		}
	}
	return false
}

func assertArtifactsSorted(t *testing.T, artifacts []domain.Artifact) {
	t.Helper()
	for i := 1; i < len(artifacts); i++ {
		if artifacts[i-1].Path > artifacts[i].Path {
			t.Fatalf("artifacts are not sorted: %s before %s", artifacts[i-1].Path, artifacts[i].Path)
		}
	}
}

func ptrBool(v bool) *bool {
	return &v
}
