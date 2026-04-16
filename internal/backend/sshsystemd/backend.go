package sshsystemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// BackendName is the canonical registry key for the ssh-systemd backend.
	BackendName = "ssh-systemd"
	// BackendAlias is a compatibility alias accepted by validation/registry lookup.
	BackendAlias = "sshsystemd"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type osCommandRunner struct{}

func (r osCommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}
	return string(out), nil
}

// Backend renders host-mode artifacts for systemd-based deployments.
type Backend struct {
	runner Runner
}

var _ backend.Backend = (*Backend)(nil)
var _ backend.RuntimeExecutor = (*Backend)(nil)
var _ backend.RuntimeObserver = (*Backend)(nil)

// New returns a new ssh-systemd backend instance.
func New() *Backend {
	return NewWithRunner(osCommandRunner{})
}

func NewWithRunner(r Runner) *Backend {
	if r == nil {
		r = osCommandRunner{}
	}
	return &Backend{runner: r}
}

// Name returns the canonical backend registry name.
func (b *Backend) Name() string {
	return BackendName
}

// ValidateTarget ensures only host-mode workloads are used with ssh-systemd.
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

// BuildDesired renders systemd/env/layout artifacts from expanded node/workload specs.
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
	targets := parseRuntimeTargets(c.Spec.Runtime.Target)
	if len(targets) > 0 {
		desired.Metadata["ssh.targets"] = strings.Join(targets, ",")
	}
	artifacts = append(artifacts, pluginOut.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	desired.Artifacts = artifacts

	return desired, nil
}

func (b *Backend) ExecuteRuntime(ctx context.Context, req backend.RuntimeApplyRequest) (backend.RuntimeApplyResult, error) {
	if err := ensureRuntimeArtifacts(req.OutputDir, req.Desired); err != nil {
		return backend.RuntimeApplyResult{}, err
	}

	cfg, err := runtimeConfigFromDesired(req.Desired)
	if err != nil {
		return backend.RuntimeApplyResult{}, err
	}

	commands := make([]string, 0, len(cfg.Targets))
	outputs := make([]string, 0, len(cfg.Targets))
	for _, target := range cfg.Targets {
		args := []string{"-p", strconv.Itoa(cfg.Port), sshAddress(cfg.User, target), "systemctl", "--version"}
		out, runErr := b.runner.Run(ctx, "ssh", args...)
		if runErr != nil {
			return backend.RuntimeApplyResult{}, runtimeCommandError("runtime preflight", target, "ssh", args, out, runErr)
		}
		commands = append(commands, "ssh "+strings.Join(args, " "))
		if trim := strings.TrimSpace(out); trim != "" {
			outputs = append(outputs, fmt.Sprintf("[%s]\n%s", target, trim))
		}
	}

	return backend.RuntimeApplyResult{
		Command: strings.Join(commands, " && "),
		Output:  truncateOutput(strings.Join(outputs, "\n")),
	}, nil
}

func (b *Backend) ObserveRuntime(ctx context.Context, req backend.RuntimeObserveRequest) (backend.RuntimeObserveResult, error) {
	if err := ensureRuntimeArtifacts(req.OutputDir, req.Desired); err != nil {
		return backend.RuntimeObserveResult{}, err
	}

	cfg, err := runtimeConfigFromDesired(req.Desired)
	if err != nil {
		return backend.RuntimeObserveResult{}, err
	}

	units := serviceUnits(req.Desired)
	if len(units) == 0 {
		return backend.RuntimeObserveResult{}, fmt.Errorf(
			"no services found in desired state for runtime observation; run validate/render again",
		)
	}

	details := make([]string, 0, len(cfg.Targets)*len(units))
	for _, target := range cfg.Targets {
		args := []string{
			"-p", strconv.Itoa(cfg.Port),
			sshAddress(cfg.User, target),
			"systemctl", "list-units",
			"--type=service",
			"--all",
			"--no-pager",
			"--no-legend",
			"--plain",
		}
		args = append(args, units...)
		out, runErr := b.runner.Run(ctx, "ssh", args...)
		if runErr != nil {
			return backend.RuntimeObserveResult{}, runtimeCommandError("runtime observe", target, "ssh", args, out, runErr)
		}

		lines := compactLines(out)
		if len(lines) == 0 {
			details = append(details, fmt.Sprintf("%s: no matching units returned by systemctl", target))
			continue
		}
		for _, line := range lines {
			details = append(details, fmt.Sprintf("%s: %s", target, line))
		}
	}
	sort.Strings(details)

	return backend.RuntimeObserveResult{
		Summary: fmt.Sprintf("observed %d target(s) and %d unit(s)", len(cfg.Targets), len(units)),
		Details: details,
	}, nil
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
		// Accept cross-node refs ("node/workload") and in-node short refs ("workload").
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

type runtimeConfig struct {
	User    string
	Port    int
	Targets []string
}

func runtimeConfigFromDesired(desired domain.DesiredState) (runtimeConfig, error) {
	cfg := runtimeConfig{
		User: "root",
		Port: 22,
	}

	if desired.Metadata != nil {
		if user := strings.TrimSpace(desired.Metadata["ssh.user"]); user != "" {
			cfg.User = user
		}
		if portRaw := strings.TrimSpace(desired.Metadata["ssh.port"]); portRaw != "" {
			port, err := strconv.Atoi(portRaw)
			if err != nil || port <= 0 {
				return runtimeConfig{}, fmt.Errorf("invalid ssh.port metadata value %q", portRaw)
			}
			cfg.Port = port
		}
		cfg.Targets = parseRuntimeTargets(desired.Metadata["ssh.targets"])
		if len(cfg.Targets) == 0 {
			cfg.Targets = parseRuntimeTargets(desired.Metadata["ssh.target"])
		}
	}

	if len(cfg.Targets) == 0 {
		return runtimeConfig{}, fmt.Errorf(
			"runtime target is empty for ssh-systemd backend; set spec.runtime.target to one or more SSH hosts",
		)
	}

	return cfg, nil
}

func parseRuntimeTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	sort.Strings(out)
	return out
}

func sshAddress(user, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}
	if strings.Contains(target, "@") || strings.TrimSpace(user) == "" {
		return target
	}
	return user + "@" + target
}

func ensureRuntimeArtifacts(outputDir string, desired domain.DesiredState) error {
	required := runtimeArtifactPaths(desired)
	if len(required) == 0 {
		return fmt.Errorf("no ssh-systemd runtime artifacts found in desired state")
	}
	missing := make([]string, 0)
	for _, rel := range required {
		abs := filepath.Join(outputDir, rel)
		if _, err := os.Stat(abs); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, rel)
				continue
			}
			return fmt.Errorf("check runtime artifact %s: %w", abs, err)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"missing %d rendered ssh-systemd artifact(s) in %s (first: %s); run render/apply before runtime execution",
			len(missing),
			outputDir,
			missing[0],
		)
	}
	return nil
}

func runtimeArtifactPaths(desired domain.DesiredState) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, artifact := range desired.Artifacts {
		if !strings.HasPrefix(artifact.Path, "ssh-systemd/") {
			continue
		}
		if !strings.HasSuffix(artifact.Path, ".service") &&
			!strings.HasSuffix(artifact.Path, ".env") &&
			!strings.HasSuffix(artifact.Path, "directories.txt") {
			continue
		}
		if _, ok := seen[artifact.Path]; ok {
			continue
		}
		seen[artifact.Path] = struct{}{}
		out = append(out, artifact.Path)
	}
	sort.Strings(out)
	return out
}

func serviceUnits(desired domain.DesiredState) []string {
	seen := make(map[string]struct{}, len(desired.Services))
	out := make([]string, 0, len(desired.Services))
	for _, svc := range desired.Services {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		unit := name + ".service"
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		out = append(out, unit)
	}
	sort.Strings(out)
	return out
}

func runtimeCommandError(op, target, name string, args []string, out string, err error) error {
	command := strings.Join(append([]string{name}, args...), " ")
	lowerOut := strings.ToLower(out)

	if errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "executable file not found") {
		return fmt.Errorf("%s failed for %q: ssh client not found; install OpenSSH client and retry", op, target)
	}
	if strings.Contains(lowerOut, "systemctl: command not found") || strings.Contains(lowerOut, "systemctl: not found") {
		return fmt.Errorf("%s failed for %q: remote host is missing systemctl; install systemd and retry", op, target)
	}
	if strings.Contains(lowerOut, "could not resolve hostname") || strings.Contains(lowerOut, "name or service not known") {
		return fmt.Errorf("%s failed for %q: ssh hostname resolution failed; verify spec.runtime.target and DNS", op, target)
	}

	trimmed := truncateOutput(strings.TrimSpace(out))
	if trimmed == "" {
		return fmt.Errorf("%s failed for %q: %w (command: %s)", op, target, err, command)
	}
	return fmt.Errorf("%s failed for %q: %w (command: %s, output: %s)", op, target, err, command, trimmed)
}

func truncateOutput(out string) string {
	const maxLen = 4096
	out = strings.TrimSpace(out)
	if len(out) <= maxLen {
		return out
	}
	return out[:maxLen] + "...(truncated)"
}

func compactLines(out string) []string {
	lines := strings.Split(out, "\n")
	compact := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		compact = append(compact, line)
	}
	return compact
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
