package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pantani/gorchestrator/internal/chain/genericprocess"
	"github.com/Pantani/gorchestrator/internal/spec"
)

func TestComposeRenderGoldenSingle(t *testing.T) {
	runComposeGoldenTest(t, "generic-single-compose.yaml", "compose-single.golden.yaml")
}

func TestComposeRenderGoldenMultiProcess(t *testing.T) {
	runComposeGoldenTest(t, "generic-multiprocess-compose.yaml", "compose-multi.golden.yaml")
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
