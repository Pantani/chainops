package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
)

func TestPluginRegistryRejectsNilAndEmptyName(t *testing.T) {
	t.Parallel()

	reg := New().Plugins

	if err := reg.Register(nil); err == nil {
		t.Fatalf("expected nil plugin registration to fail")
	}
	if err := reg.Register(stubPlugin{name: ""}); err == nil {
		t.Fatalf("expected empty-name plugin registration to fail")
	}
}

func TestBackendRegistryRegistersAliasesAndTrimsLookup(t *testing.T) {
	t.Parallel()

	reg := New().Backends
	composeBackend := stubBackend{name: "docker-compose"}
	if err := reg.Register(composeBackend); err != nil {
		t.Fatalf("register backend: %v", err)
	}

	got, ok := reg.Get(" compose ")
	if !ok {
		t.Fatalf("expected compose alias lookup to succeed")
	}
	if got.Name() != "docker-compose" {
		t.Fatalf("expected canonical backend, got %q", got.Name())
	}
}

func TestBackendRegistryRejectsAliasCollision(t *testing.T) {
	t.Parallel()

	reg := New().Backends
	if err := reg.Register(stubBackend{name: "compose"}); err != nil {
		t.Fatalf("register preexisting alias backend: %v", err)
	}

	err := reg.Register(stubBackend{name: "docker-compose"})
	if err == nil {
		t.Fatalf("expected alias collision error")
	}
	if !strings.Contains(err.Error(), `alias "compose"`) {
		t.Fatalf("expected alias collision detail, got: %v", err)
	}
}

type stubPlugin struct {
	name string
}

func (s stubPlugin) Name() string {
	return s.name
}

func (s stubPlugin) Family() string {
	return "test-family"
}

func (s stubPlugin) Capabilities() chain.Capabilities {
	return chain.Capabilities{}
}

func (s stubPlugin) Validate(_ *v1alpha1.ChainCluster) []domain.Diagnostic {
	return nil
}

func (s stubPlugin) Normalize(_ *v1alpha1.ChainCluster) error {
	return nil
}

func (s stubPlugin) Build(_ context.Context, _ *v1alpha1.ChainCluster) (chain.Output, error) {
	return chain.Output{}, nil
}

type stubBackend struct {
	name string
}

func (s stubBackend) Name() string {
	return s.name
}

func (s stubBackend) ValidateTarget(_ *v1alpha1.ChainCluster) []domain.Diagnostic {
	return nil
}

func (s stubBackend) BuildDesired(_ context.Context, _ *v1alpha1.ChainCluster, _ chain.Output) (domain.DesiredState, error) {
	return domain.DesiredState{}, nil
}
