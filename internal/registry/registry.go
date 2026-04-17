package registry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Pantani/gorchestrator/internal/backend"
	"github.com/Pantani/gorchestrator/internal/backend/ansible"
	"github.com/Pantani/gorchestrator/internal/backend/compose"
	"github.com/Pantani/gorchestrator/internal/backend/kubernetes"
	"github.com/Pantani/gorchestrator/internal/backend/sshsystemd"
	"github.com/Pantani/gorchestrator/internal/backend/terraform"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/chain/bitcoin"
	"github.com/Pantani/gorchestrator/internal/chain/cometbft"
	"github.com/Pantani/gorchestrator/internal/chain/cosmos"
	"github.com/Pantani/gorchestrator/internal/chain/evm"
	"github.com/Pantani/gorchestrator/internal/chain/genericprocess"
	"github.com/Pantani/gorchestrator/internal/chain/solana"
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
	r.MustRegisterPlugin(evm.New())
	r.MustRegisterPlugin(solana.New())
	r.MustRegisterPlugin(bitcoin.New())
	r.MustRegisterPlugin(cosmos.New())
	r.MustRegisterBackend(compose.New())
	r.MustRegisterBackend(sshsystemd.New())
	r.MustRegisterBackend(kubernetes.New())
	r.MustRegisterBackend(terraform.New())
	r.MustRegisterBackend(ansible.New())
	return r
}

func (r *PluginRegistry) Register(p chain.Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin cannot be nil")
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}
	r.plugins[name] = p
	return nil
}

func (r *PluginRegistry) MustRegister(p chain.Plugin) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

func (r *BackendRegistry) Register(b backend.Backend) error {
	if b == nil {
		return fmt.Errorf("backend cannot be nil")
	}
	name := strings.TrimSpace(b.Name())
	if name == "" {
		return fmt.Errorf("backend name cannot be empty")
	}
	aliases := backendAliases(name)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.backends[name]; exists {
		return fmt.Errorf("backend %q already registered", name)
	}

	for _, alias := range aliases {
		if alias == name {
			continue
		}
		if existing, exists := r.backends[alias]; exists && existing != b {
			return fmt.Errorf("backend alias %q already registered", alias)
		}
	}

	r.backends[name] = b
	for _, alias := range aliases {
		if alias == name {
			continue
		}
		r.backends[alias] = b
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
	p, ok := r.plugins[strings.TrimSpace(name)]
	return p, ok
}

func (r *BackendRegistry) Get(name string) (backend.Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[strings.TrimSpace(name)]
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

func backendAliases(name string) []string {
	switch name {
	case "docker-compose":
		return []string{"compose"}
	case "ssh-systemd":
		return []string{"sshsystemd"}
	case "kubernetes":
		return []string{"k8s"}
	case "terraform":
		return []string{"tf"}
	default:
		return nil
	}
}
