package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Pantani/gorchestrator/internal/backend"
	"github.com/Pantani/gorchestrator/internal/backend/compose"
	"github.com/Pantani/gorchestrator/internal/backend/sshsystemd"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/chain/cometbft"
	"github.com/Pantani/gorchestrator/internal/chain/genericprocess"
)

// PluginRegistry stores chain plugins by unique name.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]chain.Plugin
}

// BackendRegistry stores runtime backends by unique name.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]backend.Backend
}

// Registries groups plugin and backend registries used by the app layer.
type Registries struct {
	Plugins  *PluginRegistry
	Backends *BackendRegistry
}

// New creates empty registries.
func New() *Registries {
	return &Registries{
		Plugins:  &PluginRegistry{plugins: make(map[string]chain.Plugin)},
		Backends: &BackendRegistry{backends: make(map[string]backend.Backend)},
	}
}

// NewDefault registers built-in plugins and backends supported by the CLI.
func NewDefault() *Registries {
	r := New()
	r.MustRegisterPlugin(genericprocess.New())
	r.MustRegisterPlugin(cometbft.New())
	r.MustRegisterBackend(compose.New())
	r.MustRegisterBackend(sshsystemd.New())
	return r
}

func (r *PluginRegistry) Register(p chain.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin %q already registered", p.Name())
	}
	r.plugins[p.Name()] = p
	return nil
}

func (r *PluginRegistry) MustRegister(p chain.Plugin) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

func (r *BackendRegistry) Register(b backend.Backend) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.backends[b.Name()]; exists {
		return fmt.Errorf("backend %q already registered", b.Name())
	}
	r.backends[b.Name()] = b
	// Register stable aliases so specs can use short backend names.
	switch b.Name() {
	case "docker-compose":
		r.backends["compose"] = b
	case "ssh-systemd":
		r.backends["sshsystemd"] = b
	}
	return nil
}

func (r *BackendRegistry) MustRegister(b backend.Backend) {
	if err := r.Register(b); err != nil {
		panic(err)
	}
}

func (r *PluginRegistry) Get(name string) (chain.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

func (r *BackendRegistry) Get(name string) (backend.Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	return b, ok
}

func (r *PluginRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *BackendRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.backends))
	for name := range r.backends {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Registries) MustRegisterPlugin(p chain.Plugin) {
	r.Plugins.MustRegister(p)
}

func (r *Registries) MustRegisterBackend(b backend.Backend) {
	r.Backends.MustRegister(b)
}
