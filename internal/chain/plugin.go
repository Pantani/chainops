package chain

import (
	"context"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/domain"
)

type Capabilities struct {
	SupportsMultiProcess bool
	SupportsBootstrap    bool
	SupportsBackup       bool
	SupportsRestore      bool
	SupportsUpgrade      bool
}

type Output struct {
	Artifacts []domain.Artifact
	Metadata  map[string]string
}

type Plugin interface {
	Name() string
	Family() string
	Capabilities() Capabilities

	Validate(spec *v1alpha1.ChainCluster) []domain.Diagnostic
	Normalize(spec *v1alpha1.ChainCluster) error
	Build(ctx context.Context, spec *v1alpha1.ChainCluster) (Output, error)
}
