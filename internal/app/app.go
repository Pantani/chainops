package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/backend"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/doctor"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/planner"
	"github.com/Pantani/gorchestrator/internal/registry"
	"github.com/Pantani/gorchestrator/internal/renderer"
	"github.com/Pantani/gorchestrator/internal/spec"
	"github.com/Pantani/gorchestrator/internal/state"
	"github.com/Pantani/gorchestrator/internal/validate"
)

type Options struct {
	StateDir string
}

type ApplyOptions struct {
	OutputDir string
	DryRun    bool
}

type ApplyResult struct {
	ClusterName      string      `json:"clusterName"`
	Backend          string      `json:"backend"`
	DryRun           bool        `json:"dryRun"`
	Plan             domain.Plan `json:"plan"`
	ArtifactsWritten int         `json:"artifactsWritten"`
	SnapshotUpdated  bool        `json:"snapshotUpdated"`
	LockPath         string      `json:"lockPath,omitempty"`
}

type StatusResult struct {
	ClusterName      string          `json:"clusterName"`
	Backend          string          `json:"backend"`
	SnapshotPath     string          `json:"snapshotPath"`
	SnapshotExists   bool            `json:"snapshotExists"`
	Snapshot         *state.Snapshot `json:"snapshot,omitempty"`
	DesiredServices  int             `json:"desiredServices"`
	DesiredArtifacts int             `json:"desiredArtifacts"`
	Plan             domain.Plan     `json:"plan"`
	Observations     []string        `json:"observations,omitempty"`
}

type App struct {
	registries *registry.Registries
	stateStore *state.Store
}

func New(opts Options) *App {
	if opts.StateDir == "" {
		opts.StateDir = ".bgorch/state"
	}
	return &App{
		registries: registry.NewDefault(),
		stateStore: state.NewStore(opts.StateDir),
	}
}

func (a *App) LoadSpec(path string) (*v1alpha1.ChainCluster, error) {
	return spec.LoadFromFile(path)
}

func (a *App) ValidateSpec(path string) (*v1alpha1.ChainCluster, []domain.Diagnostic, error) {
	cluster, err := spec.LoadFromFile(path)
	if err != nil {
		return nil, nil, err
	}
	plugin, backendImpl, diags := a.resolve(cluster)
	if plugin != nil {
		diags = append(diags, plugin.Validate(cluster)...)
	}
	if backendImpl != nil {
		diags = append(diags, backendImpl.ValidateTarget(cluster)...)
	}
	diags = append(diags, validate.Cluster(cluster)...)
	return cluster, diags, nil
}

func (a *App) Render(ctx context.Context, specPath, outputDir string, writeState bool) (domain.DesiredState, []domain.Diagnostic, error) {
	_, desired, diags, err := a.buildDesired(ctx, specPath)
	if err != nil {
		return domain.DesiredState{}, nil, err
	}
	if HasErrors(diags) {
		return domain.DesiredState{}, diags, nil
	}

	if err := renderer.WriteArtifacts(outputDir, desired.Artifacts); err != nil {
		return domain.DesiredState{}, diags, fmt.Errorf("write artifacts: %w", err)
	}
	if writeState {
		snap := state.FromDesired(desired)
		if err := a.stateStore.Save(snap); err != nil {
			return domain.DesiredState{}, diags, fmt.Errorf("save state snapshot: %w", err)
		}
	}
	return desired, diags, nil
}

func (a *App) Plan(ctx context.Context, specPath string) (domain.Plan, []domain.Diagnostic, error) {
	cluster, desired, diags, err := a.buildDesired(ctx, specPath)
	if err != nil {
		return domain.Plan{}, nil, err
	}
	if HasErrors(diags) {
		return domain.Plan{}, diags, nil
	}
	current, err := a.stateStore.Load(cluster.Metadata.Name, desired.Backend)
	if err != nil {
		return domain.Plan{}, diags, fmt.Errorf("load state snapshot: %w", err)
	}
	p := planner.Build(desired, current)
	return p, diags, nil
}

func (a *App) Apply(ctx context.Context, specPath string, opts ApplyOptions) (result ApplyResult, diags []domain.Diagnostic, err error) {
	if opts.OutputDir == "" {
		opts.OutputDir = ".bgorch/render"
	}

	cluster, desired, diags, err := a.buildDesired(ctx, specPath)
	if err != nil {
		return ApplyResult{}, nil, err
	}

	result = ApplyResult{
		ClusterName: cluster.Metadata.Name,
		Backend:     desired.Backend,
		DryRun:      opts.DryRun,
	}

	if HasErrors(diags) {
		return result, diags, nil
	}

	lock, err := a.stateStore.AcquireLock(cluster.Metadata.Name, desired.Backend)
	if err != nil {
		return result, diags, fmt.Errorf("acquire state lock: %w", err)
	}
	result.LockPath = lock.Path()
	defer func() {
		releaseErr := lock.Release()
		if releaseErr != nil && err == nil {
			err = fmt.Errorf("release state lock: %w", releaseErr)
		}
	}()

	current, err := a.stateStore.Load(cluster.Metadata.Name, desired.Backend)
	if err != nil {
		return result, diags, fmt.Errorf("load state snapshot: %w", err)
	}
	result.Plan = planner.Build(desired, current)

	if opts.DryRun {
		return result, diags, nil
	}

	if err := renderer.WriteArtifacts(opts.OutputDir, desired.Artifacts); err != nil {
		return result, diags, fmt.Errorf("write artifacts: %w", err)
	}
	if err := a.stateStore.Save(state.FromDesired(desired)); err != nil {
		return result, diags, fmt.Errorf("save state snapshot: %w", err)
	}

	result.ArtifactsWritten = len(desired.Artifacts)
	result.SnapshotUpdated = true
	return result, diags, nil
}

func (a *App) Status(ctx context.Context, specPath string) (StatusResult, []domain.Diagnostic, error) {
	cluster, diags, err := a.ValidateSpec(specPath)
	if err != nil {
		return StatusResult{}, nil, err
	}

	result := StatusResult{
		ClusterName:  cluster.Metadata.Name,
		Backend:      strings.TrimSpace(cluster.Spec.Runtime.Backend),
		SnapshotPath: a.stateStore.SnapshotPath(cluster.Metadata.Name, strings.TrimSpace(cluster.Spec.Runtime.Backend)),
		Observations: make([]string, 0),
	}

	if HasErrors(diags) {
		result.Observations = append(result.Observations, "spec has validation errors; desired state could not be built")
		return result, diags, nil
	}

	desired, err := a.buildDesiredFromCluster(ctx, cluster)
	if err != nil {
		return result, diags, err
	}

	result.Backend = desired.Backend
	result.SnapshotPath = a.stateStore.SnapshotPath(cluster.Metadata.Name, desired.Backend)
	result.DesiredServices = len(desired.Services)
	result.DesiredArtifacts = len(desired.Artifacts)

	snap, err := a.stateStore.Load(cluster.Metadata.Name, desired.Backend)
	if err != nil {
		return result, diags, fmt.Errorf("load state snapshot: %w", err)
	}
	result.Snapshot = snap
	result.SnapshotExists = snap != nil
	result.Plan = planner.Build(desired, snap)

	if snap == nil {
		result.Observations = append(result.Observations, "no local snapshot found; run apply to initialize state")
	} else {
		result.Observations = append(result.Observations, fmt.Sprintf("snapshot last updated at %s", snap.UpdatedAt.Format(time.RFC3339)))
	}
	changes := nonNoopChanges(result.Plan)
	if changes == 0 {
		result.Observations = append(result.Observations, "desired state matches local snapshot")
	} else {
		result.Observations = append(result.Observations, fmt.Sprintf("%d change(s) detected between desired state and local snapshot", changes))
	}

	return result, diags, nil
}

func (a *App) Doctor(ctx context.Context, specPath string) (doctor.Report, error) {
	report := doctor.NewReport()

	cluster, diags, err := a.ValidateSpec(specPath)
	if err != nil {
		report.Add("spec.load", doctor.StatusFail, fmt.Sprintf("failed to load spec: %v", err), "fix file path or YAML syntax")
		if accessErr := a.stateStore.EnsureStateDirAccessible(); accessErr != nil {
			report.Add("state.access", doctor.StatusFail, fmt.Sprintf("state directory is not accessible: %v", accessErr), "check state dir permissions")
		} else {
			report.Add("state.access", doctor.StatusPass, "state directory is accessible", "")
		}
		return report, nil
	}

	report.ClusterName = cluster.Metadata.Name
	report.Backend = cluster.Spec.Runtime.Backend

	warnings, errors := diagnosticCounts(diags)
	switch {
	case errors > 0:
		report.Add("spec.validation", doctor.StatusFail, fmt.Sprintf("validation failed with %d error(s) and %d warning(s)", errors, warnings), "run `bgorch validate -f <spec>` to inspect diagnostics")
	case warnings > 0:
		report.Add("spec.validation", doctor.StatusWarn, fmt.Sprintf("validation passed with %d warning(s)", warnings), "review warnings before apply")
	default:
		report.Add("spec.validation", doctor.StatusPass, "validation passed", "")
	}

	_, pluginOK := a.registries.Plugins.Get(cluster.Spec.Plugin)
	if pluginOK {
		report.Add("plugin.resolve", doctor.StatusPass, fmt.Sprintf("plugin %q resolved", cluster.Spec.Plugin), "")
	} else {
		report.Add("plugin.resolve", doctor.StatusFail, fmt.Sprintf("plugin %q is not registered", cluster.Spec.Plugin), "register plugin or update spec.plugin")
	}

	_, backendOK := a.registries.Backends.Get(cluster.Spec.Runtime.Backend)
	if backendOK {
		report.Add("backend.resolve", doctor.StatusPass, fmt.Sprintf("backend %q resolved", cluster.Spec.Runtime.Backend), "")
	} else {
		report.Add("backend.resolve", doctor.StatusFail, fmt.Sprintf("backend %q is not registered", cluster.Spec.Runtime.Backend), "register backend or update spec.runtime.backend")
	}

	if accessErr := a.stateStore.EnsureStateDirAccessible(); accessErr != nil {
		report.Add("state.access", doctor.StatusFail, fmt.Sprintf("state directory is not accessible: %v", accessErr), "check state dir permissions")
	} else {
		report.Add("state.access", doctor.StatusPass, "state directory is accessible", "")
	}

	if errors > 0 || !pluginOK || !backendOK {
		return report, nil
	}

	desired, err := a.buildDesiredFromCluster(ctx, cluster)
	if err != nil {
		report.Add("desired.build", doctor.StatusFail, fmt.Sprintf("failed to build desired state: %v", err), "inspect plugin/backend configuration")
		return report, nil
	}
	report.Backend = desired.Backend

	snap, err := a.stateStore.Load(cluster.Metadata.Name, desired.Backend)
	if err != nil {
		report.Add("state.snapshot", doctor.StatusFail, fmt.Sprintf("failed to load snapshot: %v", err), "verify snapshot file permissions and format")
		return report, nil
	}
	if snap == nil {
		report.Add("state.snapshot", doctor.StatusWarn, "no local snapshot found", "run apply to persist desired state")
		return report, nil
	}
	report.Add("state.snapshot", doctor.StatusPass, fmt.Sprintf("snapshot loaded (%s)", snap.UpdatedAt.Format(time.RFC3339)), "")

	p := planner.Build(desired, snap)
	changes := nonNoopChanges(p)
	if changes == 0 {
		report.Add("plan.drift", doctor.StatusPass, "no drift detected against local snapshot", "")
	} else {
		report.Add("plan.drift", doctor.StatusWarn, fmt.Sprintf("%d change(s) detected between desired and snapshot", changes), "run plan/apply to reconcile")
	}

	return report, nil
}

func (a *App) buildDesired(ctx context.Context, specPath string) (*v1alpha1.ChainCluster, domain.DesiredState, []domain.Diagnostic, error) {
	cluster, diags, err := a.ValidateSpec(specPath)
	if err != nil {
		return nil, domain.DesiredState{}, nil, err
	}
	if HasErrors(diags) {
		return cluster, domain.DesiredState{}, diags, nil
	}
	desired, err := a.buildDesiredFromCluster(ctx, cluster)
	if err != nil {
		return cluster, domain.DesiredState{}, diags, err
	}
	return cluster, desired, diags, nil
}

func (a *App) buildDesiredFromCluster(ctx context.Context, cluster *v1alpha1.ChainCluster) (domain.DesiredState, error) {
	plugin, backendImpl, diags := a.resolve(cluster)
	if HasErrors(diags) {
		return domain.DesiredState{}, fmt.Errorf("failed to resolve runtime dependencies")
	}
	if plugin == nil || backendImpl == nil {
		return domain.DesiredState{}, fmt.Errorf("plugin/backend resolution returned nil implementation")
	}
	if err := plugin.Normalize(cluster); err != nil {
		return domain.DesiredState{}, fmt.Errorf("normalize plugin %s: %w", plugin.Name(), err)
	}
	pluginOut, err := plugin.Build(ctx, cluster)
	if err != nil {
		return domain.DesiredState{}, fmt.Errorf("build plugin output: %w", err)
	}
	desired, err := backendImpl.BuildDesired(ctx, cluster, pluginOut)
	if err != nil {
		return domain.DesiredState{}, fmt.Errorf("build desired state: %w", err)
	}
	return desired, nil
}

func HasErrors(diags []domain.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == domain.SeverityError {
			return true
		}
	}
	return false
}

func (a *App) resolve(cluster *v1alpha1.ChainCluster) (chain.Plugin, backend.Backend, []domain.Diagnostic) {
	diags := make([]domain.Diagnostic, 0)

	plugin, ok := a.registries.Plugins.Get(cluster.Spec.Plugin)
	if !ok {
		diags = append(diags, domain.Error("spec.plugin", "plugin not registered", fmt.Sprintf("available plugins: %v", a.registries.Plugins.Names())))
	}

	backendName := cluster.Spec.Runtime.Backend
	backendImpl, ok := a.registries.Backends.Get(backendName)
	if !ok {
		diags = append(diags, domain.Error("spec.runtime.backend", "backend not registered", fmt.Sprintf("available backends: %v", a.registries.Backends.Names())))
	}

	return plugin, backendImpl, diags
}

func diagnosticCounts(diags []domain.Diagnostic) (warnings int, errors int) {
	for _, d := range diags {
		switch d.Severity {
		case domain.SeverityWarning:
			warnings++
		case domain.SeverityError:
			errors++
		}
	}
	return warnings, errors
}

func nonNoopChanges(plan domain.Plan) int {
	count := 0
	for _, c := range plan.Changes {
		if c.Type != domain.ChangeNoop {
			count++
		}
	}
	return count
}
