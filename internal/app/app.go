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

// Options controls App construction dependencies.
type Options struct {
	StateDir   string
	Registries *registry.Registries
}

// ApplyOptions controls side effects of the apply pipeline.
type ApplyOptions struct {
	OutputDir      string
	DryRun         bool
	ExecuteRuntime bool
}

// StatusOptions configures runtime observation behavior for status.
type StatusOptions struct {
	OutputDir      string
	ObserveRuntime bool
}

// DoctorOptions configures runtime observation behavior for doctor.
type DoctorOptions struct {
	OutputDir      string
	ObserveRuntime bool
}

// ApplyResult captures the deterministic apply outcome for CLI/JSON consumers.
type ApplyResult struct {
	ClusterName      string                      `json:"clusterName"`
	Backend          string                      `json:"backend"`
	DryRun           bool                        `json:"dryRun"`
	RuntimeRequested bool                        `json:"runtimeRequested"`
	Plan             domain.Plan                 `json:"plan"`
	ArtifactsWritten int                         `json:"artifactsWritten"`
	SnapshotUpdated  bool                        `json:"snapshotUpdated"`
	LockPath         string                      `json:"lockPath,omitempty"`
	RuntimeResult    *backend.RuntimeApplyResult `json:"runtimeResult,omitempty"`
}

// StatusResult summarizes desired-vs-snapshot convergence plus optional runtime observation.
type StatusResult struct {
	ClusterName             string                        `json:"clusterName"`
	Backend                 string                        `json:"backend"`
	SnapshotPath            string                        `json:"snapshotPath"`
	SnapshotExists          bool                          `json:"snapshotExists"`
	Snapshot                *state.Snapshot               `json:"snapshot,omitempty"`
	DesiredServices         int                           `json:"desiredServices"`
	DesiredArtifacts        int                           `json:"desiredArtifacts"`
	Plan                    domain.Plan                   `json:"plan"`
	Observations            []string                      `json:"observations,omitempty"`
	RuntimeObserveRequested bool                          `json:"runtimeObserveRequested"`
	RuntimeObservation      *backend.RuntimeObserveResult `json:"runtimeObservation,omitempty"`
	RuntimeObservationError string                        `json:"runtimeObservationError,omitempty"`
}

// App orchestrates command-level workflows around plugins, backends, and state.
type App struct {
	registries *registry.Registries
	stateStore *state.Store
}

// New constructs an App with sane defaults for state directory and registries.
func New(opts Options) *App {
	if opts.StateDir == "" {
		opts.StateDir = ".bgorch/state"
	}
	if opts.Registries == nil {
		opts.Registries = registry.NewDefault()
	}
	return &App{
		registries: opts.Registries,
		stateStore: state.NewStore(opts.StateDir),
	}
}

// LoadSpec loads and defaults a v1alpha1 cluster spec from disk.
func (a *App) LoadSpec(path string) (*v1alpha1.ChainCluster, error) {
	return spec.LoadFromFile(path)
}

// ValidateSpec runs registry resolution, plugin/backend validation, and core validation.
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

// Render builds desired state and writes rendered artifacts to outputDir.
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

// Plan computes desired-vs-snapshot changes without mutating artifacts or state.
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

// Apply executes the mutable pipeline under lock and optionally triggers backend runtime execution.
func (a *App) Apply(ctx context.Context, specPath string, opts ApplyOptions) (result ApplyResult, diags []domain.Diagnostic, err error) {
	if opts.OutputDir == "" {
		opts.OutputDir = ".bgorch/render"
	}
	if opts.DryRun && opts.ExecuteRuntime {
		return ApplyResult{}, nil, fmt.Errorf("--dry-run cannot be combined with runtime execution")
	}

	cluster, desired, diags, err := a.buildDesired(ctx, specPath)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	_, backendImpl, resolveDiags := a.resolve(cluster)
	if HasErrors(resolveDiags) {
		return ApplyResult{}, nil, fmt.Errorf("failed to resolve backend implementation")
	}

	result = ApplyResult{
		ClusterName:      cluster.Metadata.Name,
		Backend:          desired.Backend,
		DryRun:           opts.DryRun,
		RuntimeRequested: opts.ExecuteRuntime,
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

	if opts.ExecuteRuntime {
		executor, ok := backendImpl.(backend.RuntimeExecutor)
		if !ok {
			return result, diags, fmt.Errorf(
				"backend %q does not support runtime execution; rerun without runtime execution",
				desired.Backend,
			)
		}
		runtimeResult, execErr := executor.ExecuteRuntime(ctx, backend.RuntimeApplyRequest{
			ClusterName: cluster.Metadata.Name,
			OutputDir:   opts.OutputDir,
			Desired:     desired,
		})
		if execErr != nil {
			return result, diags, execErr
		}
		result.RuntimeResult = &runtimeResult
	}

	if err := a.stateStore.Save(state.FromDesired(desired)); err != nil {
		return result, diags, fmt.Errorf("save state snapshot: %w", err)
	}

	result.ArtifactsWritten = len(desired.Artifacts)
	result.SnapshotUpdated = true
	return result, diags, nil
}

// Status reports desired-vs-snapshot convergence and optional backend runtime observations.
func (a *App) Status(ctx context.Context, specPath string, opts StatusOptions) (StatusResult, []domain.Diagnostic, error) {
	if opts.OutputDir == "" {
		opts.OutputDir = ".bgorch/render"
	}

	cluster, diags, err := a.ValidateSpec(specPath)
	if err != nil {
		return StatusResult{}, nil, err
	}

	result := StatusResult{
		ClusterName:             cluster.Metadata.Name,
		Backend:                 strings.TrimSpace(cluster.Spec.Runtime.Backend),
		SnapshotPath:            a.stateStore.SnapshotPath(cluster.Metadata.Name, strings.TrimSpace(cluster.Spec.Runtime.Backend)),
		Observations:            make([]string, 0),
		RuntimeObserveRequested: opts.ObserveRuntime,
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

	if opts.ObserveRuntime {
		_, backendImpl, resolveDiags := a.resolve(cluster)
		if HasErrors(resolveDiags) {
			result.RuntimeObservationError = "runtime observation skipped: backend resolution failed"
			result.Observations = append(result.Observations, result.RuntimeObservationError)
			return result, diags, nil
		}
		observer, ok := backendImpl.(backend.RuntimeObserver)
		if !ok {
			result.RuntimeObservationError = fmt.Sprintf("backend %q does not support runtime observation", desired.Backend)
			result.Observations = append(result.Observations, result.RuntimeObservationError)
			return result, diags, nil
		}

		runtimeObs, observeErr := observer.ObserveRuntime(ctx, backend.RuntimeObserveRequest{
			ClusterName: cluster.Metadata.Name,
			OutputDir:   opts.OutputDir,
			Desired:     desired,
		})
		if observeErr != nil {
			// Status remains usable in local-state mode even when runtime observation fails.
			result.RuntimeObservationError = observeErr.Error()
			result.Observations = append(result.Observations, "runtime observation failed: "+observeErr.Error())
			return result, diags, nil
		}
		result.RuntimeObservation = &runtimeObs
		result.Observations = append(result.Observations, "runtime observation: "+runtimeObs.Summary)
	}

	return result, diags, nil
}

// Doctor runs operational checks and returns a structured report with pass/warn/fail statuses.
func (a *App) Doctor(ctx context.Context, specPath string, opts DoctorOptions) (doctor.Report, error) {
	if opts.OutputDir == "" {
		opts.OutputDir = ".bgorch/render"
	}

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

	if opts.ObserveRuntime {
		_, backendImpl, resolveDiags := a.resolve(cluster)
		if HasErrors(resolveDiags) {
			report.Add("runtime.observe", doctor.StatusWarn, "runtime observation skipped because backend resolution failed", "run validate and fix backend resolution issues")
			return report, nil
		}
		observer, ok := backendImpl.(backend.RuntimeObserver)
		if !ok {
			report.Add(
				"runtime.observe",
				doctor.StatusWarn,
				fmt.Sprintf("backend %q does not support runtime observation", desired.Backend),
				"rerun without --observe-runtime or choose a backend with runtime observation support",
			)
			return report, nil
		}

		runtimeObs, observeErr := observer.ObserveRuntime(ctx, backend.RuntimeObserveRequest{
			ClusterName: cluster.Metadata.Name,
			OutputDir:   opts.OutputDir,
			Desired:     desired,
		})
		if observeErr != nil {
			// Doctor intentionally degrades to warning to preserve troubleshooting output.
			report.Add(
				"runtime.observe",
				doctor.StatusWarn,
				fmt.Sprintf("runtime observation failed: %v", observeErr),
				"ensure runtime tools are installed and output-dir points to rendered artifacts",
			)
			return report, nil
		}
		report.Add("runtime.observe", doctor.StatusPass, runtimeObs.Summary, "")
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

// HasErrors reports whether diagnostics contain at least one error-level entry.
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
