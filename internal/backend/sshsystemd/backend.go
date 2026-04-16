package sshsystemd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/backend"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

const (
	BackendName  = "ssh-systemd"
	BackendAlias = "sshsystemd"
)

type Backend struct{}

var _ backend.Backend = (*Backend)(nil)

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return BackendName
}

func (b *Backend) ValidateTarget(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	backendName := strings.TrimSpace(c.Spec.Runtime.Backend)
	if backendName != BackendName && backendName != BackendAlias {
		diags = append(diags, domain.Error(
			"spec.runtime.backend",
			"ssh-systemd backend selected with incompatible backend name",
			"use ssh-systemd (or alias sshsystemd)",
		))
		return diags
	}

	for i, pool := range c.Spec.NodePools {
		for j, w := range pool.Template.Workloads {
			if w.Mode == v1alpha1.WorkloadModeContainer {
				diags = append(diags, domain.Error(
					fmt.Sprintf("spec.nodePools[%d].template.workloads[%d].mode", i, j),
					"ssh-systemd backend only supports host mode workloads",
					"set mode: host and configure workload.binary, or choose docker-compose backend",
				))
			}
		}
	}
	return diags
}

func (b *Backend) BuildDesired(ctx context.Context, c *v1alpha1.ChainCluster, pluginOut chain.Output) (domain.DesiredState, error) {
	_ = ctx

	sshCfg := c.Spec.Runtime.BackendConfig.SSHSystemd
	sshUser := "root"
	sshPort := 22
	if sshCfg != nil {
		if strings.TrimSpace(sshCfg.User) != "" {
			sshUser = strings.TrimSpace(sshCfg.User)
		}
		if sshCfg.Port > 0 {
			sshPort = sshCfg.Port
		}
	}

	nodes := spec.ExpandNodes(c)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	services := make([]domain.Service, 0)
	artifacts := make([]domain.Artifact, 0)

	for _, n := range nodes {
		volumeDefs := make(map[string]v1alpha1.VolumeSpec, len(n.Spec.Volumes))
		for _, v := range n.Spec.Volumes {
			volumeDefs[v.Name] = v
		}

		workloads := append([]v1alpha1.WorkloadSpec{}, n.Spec.Workloads...)
		sort.Slice(workloads, func(i, j int) bool { return workloads[i].Name < workloads[j].Name })

		unitNameByWorkload := make(map[string]string, len(workloads))
		for _, w := range workloads {
			unitNameByWorkload[w.Name] = unitName(c.Metadata.Name, n.Name, w.Name)
		}

		nodeDirs := map[string]struct{}{}
		envBaseDir := filepath.ToSlash(filepath.Join("/etc/bgorch", sanitizeName(c.Metadata.Name), sanitizeName(n.Name)))
		nodeDirs[envBaseDir] = struct{}{}

		for _, w := range workloads {
			if w.Mode == v1alpha1.WorkloadModeContainer {
				return domain.DesiredState{}, fmt.Errorf("workload %s/%s is container mode and cannot run on ssh-systemd backend", n.Name, w.Name)
			}
			if strings.TrimSpace(w.Binary) == "" {
				return domain.DesiredState{}, fmt.Errorf("workload %s/%s requires binary for host mode", n.Name, w.Name)
			}

			serviceName := unitNameByWorkload[w.Name]
			restart := systemdRestartPolicy(w.RestartPolicy)
			dependsUnits := resolveDepends(c.Metadata.Name, n.Name, w.DependsOn, unitNameByWorkload)
			cmd := buildCommand(w)

			service := domain.Service{
				Name:          serviceName,
				Node:          n.Name,
				Workload:      w.Name,
				Command:       cmd,
				Env:           envMap(w.Env),
				RestartPolicy: restart,
				DependsOn:     dependsUnits,
			}
			service.Volumes = resolveVolumes(c.Metadata.Name, n.Name, w.VolumeMounts, volumeDefs)
			sort.Slice(service.Volumes, func(i, j int) bool {
				if service.Volumes[i].Source == service.Volumes[j].Source {
					return service.Volumes[i].Target < service.Volumes[j].Target
				}
				return service.Volumes[i].Source < service.Volumes[j].Source
			})
			services = append(services, service)

			for _, vm := range service.Volumes {
				nodeDirs[vm.Source] = struct{}{}
			}

			envFileRel := ""
			if len(service.Env) > 0 {
				envFileRel = filepath.ToSlash(filepath.Join("ssh-systemd", "nodes", sanitizeName(n.Name), "env", serviceName+".env"))
				artifacts = append(artifacts, domain.Artifact{Path: envFileRel, Content: renderEnvFile(service.Env)})
			}

			servicePath := filepath.ToSlash(filepath.Join("ssh-systemd", "nodes", sanitizeName(n.Name), "systemd", serviceName+".service"))
			unitContent := renderSystemdUnit(renderInput{
				Description: fmt.Sprintf("bgorch %s/%s/%s", c.Metadata.Name, n.Name, w.Name),
				ExecStart:   joinCommand(cmd),
				After:       append([]string{"network-online.target"}, dependsUnits...),
				Wants:       append([]string{"network-online.target"}, dependsUnits...),
				Restart:     restart,
				WorkDir:     filepath.ToSlash(filepath.Join("/var/lib/bgorch", sanitizeName(c.Metadata.Name), sanitizeName(n.Name), sanitizeName(w.Name))),
				EnvFile:     remoteEnvPath(envBaseDir, serviceName),
				HasEnvFile:  envFileRel != "",
			})
			artifacts = append(artifacts, domain.Artifact{Path: servicePath, Content: unitContent})
		}

		nodeLayoutPath := filepath.ToSlash(filepath.Join("ssh-systemd", "nodes", sanitizeName(n.Name), "layout", "directories.txt"))
		artifacts = append(artifacts, domain.Artifact{Path: nodeLayoutPath, Content: renderNodeDirs(nodeDirs)})
	}

	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

	desired := domain.DesiredState{
		ClusterName: c.Metadata.Name,
		Backend:     b.Name(),
		Services:    services,
		Metadata: map[string]string{
			"ssh.user": sshUser,
			"ssh.port": strconv.Itoa(sshPort),
		},
	}
	artifacts = append(artifacts, pluginOut.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	desired.Artifacts = artifacts

	return desired, nil
}

func resolveVolumes(clusterName, nodeName string, mounts []v1alpha1.VolumeMountSpec, defs map[string]v1alpha1.VolumeSpec) []domain.VolumeMount {
	out := make([]domain.VolumeMount, 0, len(mounts))
	for _, m := range mounts {
		if strings.TrimSpace(m.Path) == "" {
			continue
		}
		source := m.Path
		vol, ok := defs[m.Volume]
		if ok && vol.Type == v1alpha1.VolumeTypeNamed {
			source = filepath.ToSlash(filepath.Join("/var/lib/bgorch", sanitizeName(clusterName), sanitizeName(nodeName), "volumes", sanitizeName(vol.Name)))
		}
		if ok && vol.Type == v1alpha1.VolumeTypeBind && strings.TrimSpace(vol.Source) != "" {
			source = filepath.ToSlash(vol.Source)
		}
		out = append(out, domain.VolumeMount{
			Source:   source,
			Target:   m.Path,
			Type:     "host-path",
			ReadOnly: m.ReadOnly,
		})
	}
	return out
}

func resolveDepends(clusterName, nodeName string, deps []string, inNode map[string]string) []string {
	if len(deps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(deps))
	out := make([]string, 0, len(deps))
	for _, dep := range deps {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if strings.Contains(dep, "/") {
			parts := strings.SplitN(dep, "/", 2)
			dep = unitName(clusterName, parts[0], parts[1])
		} else if mapped, ok := inNode[dep]; ok {
			dep = mapped
		} else {
			dep = unitName(clusterName, nodeName, dep)
		}
		if _, ok := seen[dep]; ok {
			continue
		}
		seen[dep] = struct{}{}
		out = append(out, dep)
	}
	sort.Strings(out)
	return out
}

func buildCommand(w v1alpha1.WorkloadSpec) []string {
	parts := make([]string, 0, 1+len(w.Command)+len(w.Args))
	parts = append(parts, w.Binary)
	parts = append(parts, w.Command...)
	parts = append(parts, w.Args...)
	return parts
}

func envMap(values []v1alpha1.EnvVar) map[string]string {
	out := make(map[string]string, len(values))
	for _, e := range values {
		out[e.Name] = e.Value
	}
	return out
}

func renderEnvFile(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(quoteEnvValue(env[k]))
		b.WriteString("\n")
	}
	return b.String()
}

func renderNodeDirs(dirs map[string]struct{}) string {
	list := make([]string, 0, len(dirs))
	for d := range dirs {
		list = append(list, d)
	}
	sort.Strings(list)

	var b strings.Builder
	for _, d := range list {
		b.WriteString(d)
		b.WriteString("\n")
	}
	return b.String()
}

type renderInput struct {
	Description string
	ExecStart   string
	After       []string
	Wants       []string
	Restart     string
	WorkDir     string
	EnvFile     string
	HasEnvFile  bool
}

func renderSystemdUnit(in renderInput) string {
	after := uniqueSorted(in.After)
	wants := uniqueSorted(in.Wants)

	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=")
	b.WriteString(in.Description)
	b.WriteString("\n")
	if len(after) > 0 {
		b.WriteString("After=")
		b.WriteString(strings.Join(after, " "))
		b.WriteString("\n")
	}
	if len(wants) > 0 {
		b.WriteString("Wants=")
		b.WriteString(strings.Join(wants, " "))
		b.WriteString("\n")
	}

	b.WriteString("\n[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("WorkingDirectory=")
	b.WriteString(in.WorkDir)
	b.WriteString("\n")
	if in.HasEnvFile {
		b.WriteString("EnvironmentFile=-")
		b.WriteString(in.EnvFile)
		b.WriteString("\n")
	}
	b.WriteString("ExecStart=")
	b.WriteString(in.ExecStart)
	b.WriteString("\n")
	b.WriteString("Restart=")
	b.WriteString(in.Restart)
	b.WriteString("\n")
	b.WriteString("RestartSec=5s\n")
	b.WriteString("LimitNOFILE=65535\n")

	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

func remoteEnvPath(baseDir, serviceName string) string {
	return filepath.ToSlash(filepath.Join(baseDir, serviceName+".env"))
}

func joinCommand(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		quoted = append(quoted, quoteArg(p))
	}
	return strings.Join(quoted, " ")
}

func quoteArg(v string) string {
	if v == "" {
		return "\"\""
	}
	if strings.ContainsAny(v, " \t\n\"'\\") {
		return strconv.Quote(v)
	}
	return v
}

func quoteEnvValue(v string) string {
	if v == "" {
		return "\"\""
	}
	if strings.ContainsAny(v, " \t\n\"'\\") {
		return strconv.Quote(v)
	}
	return v
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func unitName(clusterName, nodeName, workloadName string) string {
	return sanitizeName(fmt.Sprintf("%s-%s-%s", clusterName, nodeName, workloadName))
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ".", "-", ":", "-")
	s = repl.Replace(s)
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return !(r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	s = strings.Join(parts, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "unnamed"
	}
	return s
}

func systemdRestartPolicy(rp v1alpha1.RestartPolicy) string {
	switch rp {
	case v1alpha1.RestartOnFailure:
		return "on-failure"
	case v1alpha1.RestartAlways:
		return "always"
	case v1alpha1.RestartUnlessStopped:
		return "always"
	default:
		return "always"
	}
}
