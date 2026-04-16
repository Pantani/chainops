package validate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/domain"
)

var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func Cluster(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)

	if c.APIVersion != v1alpha1.APIVersion {
		diags = append(diags, domain.Error("apiVersion", "unsupported apiVersion", "use bgorch.io/v1alpha1"))
	}
	if c.Kind != v1alpha1.KindChainCluster {
		diags = append(diags, domain.Error("kind", "unsupported kind", "use ChainCluster"))
	}
	if c.Metadata.Name == "" {
		diags = append(diags, domain.Error("metadata.name", "metadata.name is required", "set a DNS-1123 compatible name"))
	} else if len(c.Metadata.Name) > 63 || !dns1123Label.MatchString(c.Metadata.Name) {
		diags = append(diags, domain.Error("metadata.name", "metadata.name must be DNS-1123 label", "use lowercase alphanumerics and '-'"))
	}

	if c.Spec.Family == "" {
		diags = append(diags, domain.Error("spec.family", "spec.family is required", "set a chain family identifier"))
	}
	if c.Spec.Plugin == "" {
		diags = append(diags, domain.Error("spec.plugin", "spec.plugin is required", "set a registered plugin name"))
	}
	if c.Spec.Runtime.Backend == "" {
		diags = append(diags, domain.Error("spec.runtime.backend", "spec.runtime.backend is required", "set docker-compose, ssh-systemd, kubernetes, etc"))
	}
	if len(c.Spec.NodePools) == 0 {
		diags = append(diags, domain.Error("spec.nodePools", "at least one nodePool is required", "define nodePools with template.workloads"))
	}

	poolNames := map[string]struct{}{}
	for i, pool := range c.Spec.NodePools {
		poolPath := fmt.Sprintf("spec.nodePools[%d]", i)
		if pool.Name == "" {
			diags = append(diags, domain.Error(poolPath+".name", "nodePool name is required", "set a unique nodePool name"))
		} else {
			if _, ok := poolNames[pool.Name]; ok {
				diags = append(diags, domain.Error(poolPath+".name", "duplicate nodePool name", "nodePool names must be unique"))
			}
			poolNames[pool.Name] = struct{}{}
		}
		if pool.Replicas <= 0 {
			diags = append(diags, domain.Error(poolPath+".replicas", "replicas must be > 0", "set replicas >= 1"))
		}
		if len(pool.Template.Workloads) == 0 {
			diags = append(diags, domain.Error(poolPath+".template.workloads", "at least one workload is required", "define at least one process per logical node"))
		}

		volumeNames := map[string]struct{}{}
		for vIdx, v := range pool.Template.Volumes {
			vPath := fmt.Sprintf("%s.template.volumes[%d]", poolPath, vIdx)
			if v.Name == "" {
				diags = append(diags, domain.Error(vPath+".name", "volume name is required", "set a unique volume name"))
				continue
			}
			if _, ok := volumeNames[v.Name]; ok {
				diags = append(diags, domain.Error(vPath+".name", "duplicate volume name", "volume names must be unique per node template"))
			}
			volumeNames[v.Name] = struct{}{}
			if v.Type == v1alpha1.VolumeTypeBind && v.Source == "" {
				diags = append(diags, domain.Error(vPath+".source", "bind volume requires source", "set host source path for bind volumes"))
			}
		}

		workloadNames := map[string]struct{}{}
		for wIdx, w := range pool.Template.Workloads {
			wPath := fmt.Sprintf("%s.template.workloads[%d]", poolPath, wIdx)
			if w.Name == "" {
				diags = append(diags, domain.Error(wPath+".name", "workload name is required", "set a unique workload name"))
				continue
			}
			if _, ok := workloadNames[w.Name]; ok {
				diags = append(diags, domain.Error(wPath+".name", "duplicate workload name", "workload names must be unique per node template"))
			}
			workloadNames[w.Name] = struct{}{}

			switch w.Mode {
			case v1alpha1.WorkloadModeContainer:
				if w.Image == "" {
					diags = append(diags, domain.Error(wPath+".image", "container workload requires image", "set workload.image for container mode"))
				}
			case v1alpha1.WorkloadModeHost:
				if w.Binary == "" {
					diags = append(diags, domain.Error(wPath+".binary", "host workload requires binary", "set workload.binary for host mode"))
				}
			default:
				diags = append(diags, domain.Error(wPath+".mode", "unsupported workload mode", "use 'container' or 'host'"))
			}

			if w.Image != "" && w.Binary != "" {
				diags = append(diags, domain.Warning(wPath, "workload has both image and binary", "binary will be ignored by container-focused backends"))
			}

			for pIdx, p := range w.Ports {
				pPath := fmt.Sprintf("%s.ports[%d]", wPath, pIdx)
				if p.ContainerPort <= 0 || p.ContainerPort > 65535 {
					diags = append(diags, domain.Error(pPath+".containerPort", "invalid container port", "valid range is 1-65535"))
				}
				if p.HostPort < 0 || p.HostPort > 65535 {
					diags = append(diags, domain.Error(pPath+".hostPort", "invalid host port", "valid range is 0-65535"))
				}
				proto := strings.ToLower(strings.TrimSpace(p.Protocol))
				if proto != "" && proto != "tcp" && proto != "udp" {
					diags = append(diags, domain.Error(pPath+".protocol", "unsupported protocol", "use tcp or udp"))
				}
			}

			for mIdx, m := range w.VolumeMounts {
				mPath := fmt.Sprintf("%s.volumeMounts[%d]", wPath, mIdx)
				if m.Volume == "" {
					diags = append(diags, domain.Error(mPath+".volume", "volumeMount.volume is required", "reference a declared template volume"))
					continue
				}
				if _, ok := volumeNames[m.Volume]; !ok {
					diags = append(diags, domain.Error(mPath+".volume", "volumeMount references unknown volume", "declare the volume in template.volumes"))
				}
				if !strings.HasPrefix(m.Path, "/") {
					diags = append(diags, domain.Error(mPath+".path", "mount path must be absolute", "use an absolute path like /var/lib/data"))
				}
			}

			for hIdx, h := range w.HealthChecks {
				hPath := fmt.Sprintf("%s.healthChecks[%d]", wPath, hIdx)
				switch h.Type {
				case v1alpha1.HealthCheckCMD:
					if h.Command == "" {
						diags = append(diags, domain.Error(hPath+".command", "cmd health check requires command", "set healthChecks.command"))
					}
				case v1alpha1.HealthCheckHTTP:
					if h.Path == "" || h.Port <= 0 {
						diags = append(diags, domain.Error(hPath, "http health check requires path and port", "set healthChecks.path and healthChecks.port"))
					}
				case v1alpha1.HealthCheckTCP:
					if h.Port <= 0 {
						diags = append(diags, domain.Error(hPath+".port", "tcp health check requires port", "set healthChecks.port"))
					}
				default:
					diags = append(diags, domain.Error(hPath+".type", "unsupported health check type", "use cmd, http, or tcp"))
				}
			}
		}

		for fIdx, f := range pool.Template.Files {
			fPath := fmt.Sprintf("%s.template.files[%d]", poolPath, fIdx)
			if f.Path == "" {
				diags = append(diags, domain.Error(fPath+".path", "file path is required", "set a relative file path"))
				continue
			}
			if filepath.IsAbs(f.Path) {
				diags = append(diags, domain.Error(fPath+".path", "absolute file path is not allowed", "use a relative path"))
			}
			clean := filepath.Clean(f.Path)
			if strings.HasPrefix(clean, "..") || strings.Contains(clean, "../") {
				diags = append(diags, domain.Error(fPath+".path", "file path escapes workspace", "avoid '..' segments"))
			}
		}
	}

	if c.Spec.Plugin != "generic-process" && c.Spec.PluginConfig.GenericProcess != nil {
		diags = append(diags, domain.Warning("spec.pluginConfig.genericProcess", "genericProcess config provided for non-generic plugin", "move plugin-specific config to the right plugin extension"))
	}

	return diags
}
