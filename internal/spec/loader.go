package spec

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
)

// ResolvedNode is a concrete node instance expanded from a nodePool template.
type ResolvedNode struct {
	Name     string
	PoolName string
	Spec     v1alpha1.NodeSpec
}

// LoadFromFile reads YAML and applies defaulting before returning the cluster spec.
func LoadFromFile(path string) (*v1alpha1.ChainCluster, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec file: %w", err)
	}

	var c v1alpha1.ChainCluster
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	ApplyDefaults(&c)
	return &c, nil
}

// ApplyDefaults mutates a cluster spec with implementation defaults expected by the pipeline.
func ApplyDefaults(c *v1alpha1.ChainCluster) {
	if c.APIVersion == "" {
		c.APIVersion = v1alpha1.APIVersion
	}
	if c.Kind == "" {
		c.Kind = v1alpha1.KindChainCluster
	}
	if c.Spec.Plugin == "" {
		if c.Spec.Family == "generic" {
			c.Spec.Plugin = "generic-process"
		} else {
			c.Spec.Plugin = c.Spec.Family
		}
	}
	c.Spec.Runtime.Backend = strings.TrimSpace(c.Spec.Runtime.Backend)

	isCompose := c.Spec.Runtime.Backend == "docker-compose" || c.Spec.Runtime.Backend == "compose"
	if isCompose {
		if c.Spec.Runtime.BackendConfig.Compose == nil {
			c.Spec.Runtime.BackendConfig.Compose = &v1alpha1.ComposeConfig{}
		}
		if c.Spec.Runtime.BackendConfig.Compose.OutputFile == "" {
			c.Spec.Runtime.BackendConfig.Compose.OutputFile = "compose.yaml"
		}
	}

	for i := range c.Spec.NodePools {
		pool := &c.Spec.NodePools[i]
		if pool.Replicas <= 0 {
			pool.Replicas = 1
		}
		if pool.Template.Name == "" && pool.Replicas == 1 {
			pool.Template.Name = pool.Name
		}
		for j := range pool.Template.Volumes {
			if pool.Template.Volumes[j].Type == "" {
				pool.Template.Volumes[j].Type = v1alpha1.VolumeTypeNamed
			}
		}
		for j := range pool.Template.Workloads {
			w := &pool.Template.Workloads[j]
			if w.Mode == "" {
				w.Mode = v1alpha1.WorkloadModeContainer
			}
			if w.RestartPolicy == "" {
				w.RestartPolicy = v1alpha1.RestartUnlessStopped
			}
			for k := range w.Ports {
				if w.Ports[k].Protocol == "" {
					w.Ports[k].Protocol = "tcp"
				}
			}
		}
	}
}

// ExpandNodes expands replicated nodePools into concrete node entries with stable names.
func ExpandNodes(c *v1alpha1.ChainCluster) []ResolvedNode {
	var out []ResolvedNode
	for _, pool := range c.Spec.NodePools {
		prefix := pool.Template.Name
		if prefix == "" {
			prefix = pool.Name
		}
		for i := 0; i < pool.Replicas; i++ {
			name := prefix
			if pool.Replicas > 1 {
				name = fmt.Sprintf("%s-%02d", prefix, i)
			}
			n := pool.Template
			n.Name = name
			if n.Role == "" && len(pool.Roles) > 0 {
				n.Role = pool.Roles[0]
			}
			out = append(out, ResolvedNode{Name: name, PoolName: pool.Name, Spec: n})
		}
	}
	return out
}
