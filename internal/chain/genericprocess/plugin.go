package genericprocess

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

// Plugin is the generic fallback plugin for process-oriented chain specs.
type Plugin struct{}

// New returns a generic-process plugin instance.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return "generic-process"
}

func (p *Plugin) Family() string {
	return "generic"
}

func (p *Plugin) Capabilities() chain.Capabilities {
	return chain.Capabilities{
		SupportsMultiProcess: true,
		SupportsBootstrap:    true,
		SupportsBackup:       true,
		SupportsRestore:      true,
		SupportsUpgrade:      true,
	}
}

// Validate checks generic-process specific extension fields.
func (p *Plugin) Validate(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	if c.Spec.Plugin != p.Name() {
		diags = append(diags, domain.Error("spec.plugin", "generic-process plugin selected with mismatched plugin name", "set spec.plugin to generic-process"))
	}
	if c.Spec.Family == "" {
		diags = append(diags, domain.Error("spec.family", "family must be set", "set family to generic or another registered family"))
	}
	if c.Spec.PluginConfig.GenericProcess != nil {
		for i, f := range c.Spec.PluginConfig.GenericProcess.ExtraFiles {
			if strings.TrimSpace(f.Path) == "" {
				diags = append(diags, domain.Error(fmt.Sprintf("spec.pluginConfig.genericProcess.extraFiles[%d].path", i), "extra file path is required", "set a relative path"))
			}
		}
	}
	return diags
}

// Normalize applies generic defaults used by downstream build logic.
func (p *Plugin) Normalize(c *v1alpha1.ChainCluster) error {
	if c.Spec.Profile == "" {
		c.Spec.Profile = "default"
	}
	if c.Spec.Family == "" {
		c.Spec.Family = "generic"
	}
	return nil
}

// Build emits deterministic file artifacts from node/global genericProcess config blocks.
func (p *Plugin) Build(ctx context.Context, c *v1alpha1.ChainCluster) (chain.Output, error) {
	_ = ctx

	artifacts := make([]domain.Artifact, 0)
	nodes := spec.ExpandNodes(c)
	for _, n := range nodes {
		for _, f := range n.Spec.Files {
			path := safeRelPath(filepath.Join("nodes", n.Name, filepath.Clean(f.Path)))
			artifacts = append(artifacts, domain.Artifact{Path: path, Content: f.Content})
		}
		if n.Spec.PluginConfig.GenericProcess != nil {
			for _, f := range n.Spec.PluginConfig.GenericProcess.ExtraFiles {
				path := safeRelPath(filepath.Join("nodes", n.Name, filepath.Clean(f.Path)))
				artifacts = append(artifacts, domain.Artifact{Path: path, Content: f.Content})
			}
		}
	}

	if c.Spec.PluginConfig.GenericProcess != nil {
		for _, f := range c.Spec.PluginConfig.GenericProcess.ExtraFiles {
			path := safeRelPath(filepath.Join("global", filepath.Clean(f.Path)))
			artifacts = append(artifacts, domain.Artifact{Path: path, Content: f.Content})
		}
	}

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Path < artifacts[j].Path
	})

	return chain.Output{
		Artifacts: artifacts,
		Metadata: map[string]string{
			"plugin": p.Name(),
			"family": c.Spec.Family,
		},
	}, nil
}

func safeRelPath(p string) string {
	p = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "/")
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
	if p == ".." || p == "." || p == "" {
		return "artifact.txt"
	}
	return p
}
