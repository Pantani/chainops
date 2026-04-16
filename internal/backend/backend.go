package backend

import (
	"context"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
)

type Backend interface {
	Name() string
	ValidateTarget(spec *v1alpha1.ChainCluster) []domain.Diagnostic
	BuildDesired(ctx context.Context, spec *v1alpha1.ChainCluster, pluginOut chain.Output) (domain.DesiredState, error)
}
