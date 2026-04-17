package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/app"
	"github.com/Pantani/gorchestrator/internal/doctor"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/registry"
	"github.com/Pantani/gorchestrator/internal/state"
)

// Options configures the engine façade.
type Options struct {
	StateDir   string
	Registries *registry.Registries
}

// Service exposes core pipelines to CLI/UI layers.
type Service struct {
	app        *app.App
	registries *registry.Registries
	stateStore *state.Store
}

// New builds an engine service with default registries when omitted.
func New(opts Options) *Service {
	regs := opts.Registries
	if regs == nil {
		regs = registry.NewDefault()
	}
	stateDir := strings.TrimSpace(opts.StateDir)
	if stateDir == "" {
		stateDir = ".chainops/state"
	}
	return &Service{
		app:        app.New(app.Options{StateDir: stateDir, Registries: regs}),
		registries: regs,
		stateStore: state.NewStore(stateDir),
	}
}

// Validate loads + validates a spec and returns diagnostics.
func (s *Service) Validate(specPath string) (*v1alpha1.ChainCluster, []domain.Diagnostic, error) {
	return s.app.ValidateSpec(specPath)
}

// LoadSpec loads the spec with defaulting only.
func (s *Service) LoadSpec(specPath string) (*v1alpha1.ChainCluster, error) {
	return s.app.LoadSpec(specPath)
}

// RenderArtifacts writes backend/plugin artifacts to outputDir.
func (s *Service) RenderArtifacts(ctx context.Context, specPath, outputDir string, writeState bool) (domain.DesiredState, []domain.Diagnostic, error) {
	return s.app.Render(ctx, specPath, outputDir, writeState)
}

// Plan computes desired-vs-observed changes against local snapshot.
func (s *Service) Plan(ctx context.Context, specPath string) (domain.Plan, []domain.Diagnostic, error) {
	return s.app.Plan(ctx, specPath)
}

// Apply reconciles desired state and optionally executes runtime hooks.
func (s *Service) Apply(ctx context.Context, specPath string, opts app.ApplyOptions) (app.ApplyResult, []domain.Diagnostic, error) {
	return s.app.Apply(ctx, specPath, opts)
}

// Status returns desired vs observed convergence details.
func (s *Service) Status(ctx context.Context, specPath string, opts app.StatusOptions) (app.StatusResult, []domain.Diagnostic, error) {
	return s.app.Status(ctx, specPath, opts)
}

// Doctor runs preflight and convergence checks.
func (s *Service) Doctor(ctx context.Context, specPath string, opts app.DoctorOptions) (doctor.Report, error) {
	return s.app.Doctor(ctx, specPath, opts)
}

// DeleteStateSnapshot removes the persisted snapshot for a cluster/backend pair.
func (s *Service) DeleteStateSnapshot(clusterName, backend string) error {
	path := s.stateStore.SnapshotPath(clusterName, backend)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove snapshot %q: %w", path, err)
	}
	return nil
}

// ResolveSnapshotPath returns the current snapshot location.
func (s *Service) ResolveSnapshotPath(clusterName, backend string) string {
	return s.stateStore.SnapshotPath(clusterName, backend)
}

// StateDir returns the engine state directory path.
func (s *Service) StateDir() string {
	return s.stateStore.Dir
}

// RemoveArtifactsDir removes rendered artifacts directory.
func (s *Service) RemoveArtifactsDir(dir string) error {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return nil
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return fmt.Errorf("resolve artifacts directory %q: %w", clean, err)
	}
	root := filepath.VolumeName(abs) + string(filepath.Separator)
	if abs == root {
		return fmt.Errorf("refusing to remove root directory")
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil && isSameOrParentPath(abs, cwd) {
		return fmt.Errorf("refusing to remove %q because it contains the current working directory", abs)
	}
	err = os.RemoveAll(abs)
	if err != nil {
		return fmt.Errorf("remove artifacts directory %q: %w", abs, err)
	}
	return nil
}

func isSameOrParentPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// PluginNames returns sorted plugin names.
func (s *Service) PluginNames() []string {
	return s.registries.Plugins.Names()
}

// BackendNames returns sorted backend names.
func (s *Service) BackendNames() []string {
	return s.registries.Backends.Names()
}

// Registries exposes plugin/backend registries for explain/list commands.
func (s *Service) Registries() *registry.Registries {
	return s.registries
}
