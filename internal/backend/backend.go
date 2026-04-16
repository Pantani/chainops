package backend

import (
	"context"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
)

// Backend translates plugin output into backend-specific desired state.
type Backend interface {
	Name() string
	ValidateTarget(spec *v1alpha1.ChainCluster) []domain.Diagnostic
	BuildDesired(ctx context.Context, spec *v1alpha1.ChainCluster, pluginOut chain.Output) (domain.DesiredState, error)
}

// RuntimeApplyRequest contains context required for optional runtime execution.
type RuntimeApplyRequest struct {
	ClusterName string
	OutputDir   string
	Desired     domain.DesiredState
}

// RuntimeApplyResult captures backend runtime command metadata for reporting.
type RuntimeApplyResult struct {
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
}

// RuntimeObserveRequest contains context required for optional runtime observation.
type RuntimeObserveRequest struct {
	ClusterName string
	OutputDir   string
	Desired     domain.DesiredState
}

// RuntimeObserveResult summarizes backend runtime observation output.
type RuntimeObserveResult struct {
	Summary string   `json:"summary"`
	Details []string `json:"details,omitempty"`
}

// RuntimeExecutor is implemented by backends that can mutate runtime state.
type RuntimeExecutor interface {
	ExecuteRuntime(ctx context.Context, req RuntimeApplyRequest) (RuntimeApplyResult, error)
}

// RuntimeObserver is implemented by backends that can inspect runtime state.
type RuntimeObserver interface {
	ObserveRuntime(ctx context.Context, req RuntimeObserveRequest) (RuntimeObserveResult, error)
}
