package compose

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

// Runner abstracts command execution for runtime operations and tests.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

type osCommandRunner struct{}

func (r osCommandRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}
	return string(out), nil
}

// Backend implements compose artifact rendering plus optional runtime actions.
type Backend struct {
	runner Runner
}

var _ backend.Backend = (*Backend)(nil)
var _ backend.RuntimeExecutor = (*Backend)(nil)
var _ backend.RuntimeObserver = (*Backend)(nil)

// New returns a compose backend using OS command execution for runtime calls.
func New() *Backend {
	return NewWithRunner(osCommandRunner{})
}

// NewWithRunner injects a custom command runner for tests or custom execution.
func NewWithRunner(r Runner) *Backend {
	if r == nil {
		r = osCommandRunner{}
	}
	return &Backend{runner: r}
}

// Name returns the canonical backend registry name.
func (b *Backend) Name() string {
	return "docker-compose"
}

// ValidateTarget enforces compose-specific workload constraints.
func (b *Backend) ValidateTarget(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	if c.Spec.Runtime.Backend != "docker-compose" && c.Spec.Runtime.Backend != "compose" {
		diags = append(diags, domain.Error("spec.runtime.backend", "compose backend selected with incompatible backend name", "use docker-compose or compose"))
		return diags
	}
	if len(c.Spec.NodePools) == 0 {
		diags = append(diags, domain.Error("spec.nodePools", "compose backend requires nodePools", "define at least one nodePool"))
		return diags
	}
	for i, pool := range c.Spec.NodePools {
		for j, w := range pool.Template.Workloads {
			if w.Mode == v1alpha1.WorkloadModeHost {
				diags = append(diags, domain.Error(
					fmt.Sprintf("spec.nodePools[%d].template.workloads[%d].mode", i, j),
					"compose backend does not support host mode",
					"use container mode or choose ssh-systemd backend",
				))
			}
		}
	}
	return diags
}

// BuildDesired transforms plugin output into compose services/volumes/networks artifacts.
func (b *Backend) BuildDesired(ctx context.Context, c *v1alpha1.ChainCluster, pluginOut chain.Output) (domain.DesiredState, error) {
	_ = ctx

	composeCfg := c.Spec.Runtime.BackendConfig.Compose
	if composeCfg == nil {
		composeCfg = &v1alpha1.ComposeConfig{OutputFile: "compose.yaml"}
	}
	projectName := composeCfg.ProjectName
	if projectName == "" {
		projectName = sanitizeName(c.Metadata.Name)
	}
	networkName := composeCfg.NetworkName
	if networkName == "" {
		networkName = fmt.Sprintf("%s-net", sanitizeName(c.Metadata.Name))
	}
	outputFile := composeCfg.OutputFile
	if strings.TrimSpace(outputFile) == "" {
		outputFile = "compose.yaml"
	}

	nodes := spec.ExpandNodes(c)
	services := make([]domain.Service, 0)
	volumesByName := map[string]domain.Volume{}

	for _, n := range nodes {
		volumeDefs := map[string]v1alpha1.VolumeSpec{}
		for _, v := range n.Spec.Volumes {
			volumeDefs[v.Name] = v
		}

		serviceNameByWorkload := map[string]string{}
		for _, w := range n.Spec.Workloads {
			serviceNameByWorkload[w.Name] = serviceName(c.Metadata.Name, n.Name, w.Name)
		}

		for _, w := range n.Spec.Workloads {
			if w.Mode == v1alpha1.WorkloadModeHost {
				return domain.DesiredState{}, fmt.Errorf("workload %s/%s is host mode and cannot run on compose backend", n.Name, w.Name)
			}

			svc := domain.Service{
				Name:          serviceName(c.Metadata.Name, n.Name, w.Name),
				Node:          n.Name,
				Workload:      w.Name,
				Image:         w.Image,
				Command:       append([]string{}, w.Command...),
				Args:          append([]string{}, w.Args...),
				Env:           envMap(w.Env),
				RestartPolicy: string(w.RestartPolicy),
			}

			for _, p := range w.Ports {
				svc.Ports = append(svc.Ports, domain.PortBinding{
					ContainerPort: p.ContainerPort,
					HostPort:      p.HostPort,
					Protocol:      strings.ToLower(p.Protocol),
				})
			}
			sort.Slice(svc.Ports, func(i, j int) bool {
				if svc.Ports[i].ContainerPort == svc.Ports[j].ContainerPort {
					return svc.Ports[i].HostPort < svc.Ports[j].HostPort
				}
				return svc.Ports[i].ContainerPort < svc.Ports[j].ContainerPort
			})

			for _, m := range w.VolumeMounts {
				v, ok := volumeDefs[m.Volume]
				if !ok {
					return domain.DesiredState{}, fmt.Errorf("volume %q not found for workload %s/%s", m.Volume, n.Name, w.Name)
				}
				mount := domain.VolumeMount{Target: m.Path, ReadOnly: m.ReadOnly}
				switch v.Type {
				case v1alpha1.VolumeTypeBind:
					mount.Source = v.Source
					mount.Type = string(v1alpha1.VolumeTypeBind)
				default:
					mount.Type = string(v1alpha1.VolumeTypeNamed)
					mount.Source = namedVolume(c.Metadata.Name, n.Name, v)
					volumesByName[mount.Source] = domain.Volume{Name: mount.Source}
				}
				svc.Volumes = append(svc.Volumes, mount)
			}
			sort.Slice(svc.Volumes, func(i, j int) bool {
				if svc.Volumes[i].Source == svc.Volumes[j].Source {
					return svc.Volumes[i].Target < svc.Volumes[j].Target
				}
				return svc.Volumes[i].Source < svc.Volumes[j].Source
			})

			if len(w.HealthChecks) > 0 {
				hc := healthCheckToCompose(w.HealthChecks[0])
				svc.HealthCheck = &hc
			}

			depends := make([]string, 0, len(w.DependsOn))
			for _, d := range w.DependsOn {
				// Accept cross-node refs ("node/workload") and in-node short refs ("workload").
				if strings.Contains(d, "/") {
					parts := strings.SplitN(d, "/", 2)
					depends = append(depends, serviceName(c.Metadata.Name, parts[0], parts[1]))
					continue
				}
				if svcName, ok := serviceNameByWorkload[d]; ok {
					depends = append(depends, svcName)
					continue
				}
				depends = append(depends, serviceName(c.Metadata.Name, n.Name, d))
			}
			sort.Strings(depends)
			svc.DependsOn = depends

			services = append(services, svc)
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

	volumes := make([]domain.Volume, 0, len(volumesByName))
	for _, v := range volumesByName {
		volumes = append(volumes, v)
	}
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })

	desired := domain.DesiredState{
		ClusterName: c.Metadata.Name,
		Backend:     b.Name(),
		Services:    services,
		Volumes:     volumes,
		Networks:    []domain.Network{{Name: networkName}},
		Metadata: map[string]string{
			"compose.project": projectName,
			"compose.network": networkName,
			"compose.file":    outputFile,
		},
	}

	compose := renderCompose(projectName, networkName, desired)
	desired.Artifacts = append(desired.Artifacts, domain.Artifact{Path: outputFile, Content: compose})
	desired.Artifacts = append(desired.Artifacts, pluginOut.Artifacts...)
	sort.Slice(desired.Artifacts, func(i, j int) bool { return desired.Artifacts[i].Path < desired.Artifacts[j].Path })

	return desired, nil
}

// ExecuteRuntime applies rendered compose artifacts to the local runtime.
func (b *Backend) ExecuteRuntime(ctx context.Context, req backend.RuntimeApplyRequest) (backend.RuntimeApplyResult, error) {
	composeFileRel := composeFilePath(req.Desired)
	composeFileAbs := filepath.Join(req.OutputDir, composeFileRel)
	if _, err := os.Stat(composeFileAbs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backend.RuntimeApplyResult{}, fmt.Errorf(
				"compose file not found at %s; run render/apply first or pass the correct --output-dir",
				composeFileAbs,
			)
		}
		return backend.RuntimeApplyResult{}, fmt.Errorf("check compose file at %s: %w", composeFileAbs, err)
	}

	project := composeProject(req.Desired, req.ClusterName)
	args := []string{"compose", "-p", project, "-f", composeFileAbs, "up", "-d"}
	out, err := b.runner.Run(ctx, req.OutputDir, "docker", args...)
	if err != nil {
		return backend.RuntimeApplyResult{}, fmt.Errorf(
			"docker compose runtime apply failed: %w; install Docker/Compose and ensure daemon is running",
			err,
		)
	}

	return backend.RuntimeApplyResult{
		Command: strings.Join(append([]string{"docker"}, args...), " "),
		Output:  truncateOutput(out),
	}, nil
}

// ObserveRuntime inspects compose runtime status from rendered artifacts.
func (b *Backend) ObserveRuntime(ctx context.Context, req backend.RuntimeObserveRequest) (backend.RuntimeObserveResult, error) {
	composeFileRel := composeFilePath(req.Desired)
	composeFileAbs := filepath.Join(req.OutputDir, composeFileRel)
	if _, err := os.Stat(composeFileAbs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backend.RuntimeObserveResult{}, fmt.Errorf(
				"compose file not found at %s; run render/apply first or pass the correct --output-dir",
				composeFileAbs,
			)
		}
		return backend.RuntimeObserveResult{}, fmt.Errorf("check compose file at %s: %w", composeFileAbs, err)
	}

	project := composeProject(req.Desired, req.ClusterName)
	args := []string{"compose", "-p", project, "-f", composeFileAbs, "ps", "--all"}
	out, err := b.runner.Run(ctx, req.OutputDir, "docker", args...)
	if err != nil {
		return backend.RuntimeObserveResult{}, fmt.Errorf(
			"docker compose runtime observe failed: %w; install Docker/Compose and ensure daemon is running",
			err,
		)
	}

	details := compactLines(out)
	summary := "docker compose reported no services"
	if len(details) > 0 {
		summary = fmt.Sprintf("docker compose returned %d line(s) from ps", len(details))
	}

	return backend.RuntimeObserveResult{
		Summary: summary,
		Details: details,
	}, nil
}

func namedVolume(clusterName, nodeName string, v v1alpha1.VolumeSpec) string {
	if strings.TrimSpace(v.Source) != "" {
		return sanitizeName(v.Source)
	}
	return sanitizeName(fmt.Sprintf("%s_%s_%s", clusterName, nodeName, v.Name))
}

func serviceName(clusterName, nodeName, workloadName string) string {
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

func envMap(values []v1alpha1.EnvVar) map[string]string {
	out := make(map[string]string, len(values))
	for _, e := range values {
		out[e.Name] = e.Value
	}
	return out
}

func healthCheckToCompose(h v1alpha1.HealthCheckSpec) domain.HealthCheck {
	hc := domain.HealthCheck{
		IntervalSec:    positiveOr(h.PeriodSec, 10),
		TimeoutSec:     5,
		Retries:        positiveOr(h.FailureThreshold, 3),
		StartPeriodSec: max(h.InitialDelaySec, 0),
	}
	switch h.Type {
	case v1alpha1.HealthCheckCMD:
		hc.Test = []string{"CMD-SHELL", h.Command}
	case v1alpha1.HealthCheckHTTP:
		hc.Test = []string{"CMD-SHELL", fmt.Sprintf("curl -fsS http://localhost:%d%s || exit 1", h.Port, h.Path)}
	case v1alpha1.HealthCheckTCP:
		hc.Test = []string{"CMD-SHELL", fmt.Sprintf("nc -z localhost %d || exit 1", h.Port)}
	default:
		hc.Test = []string{"NONE"}
	}
	return hc
}

func positiveOr(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderCompose(projectName, networkName string, desired domain.DesiredState) string {
	var b strings.Builder
	b.WriteString("name: ")
	b.WriteString(quote(projectName))
	b.WriteString("\n")
	b.WriteString("services:\n")

	for _, svc := range desired.Services {
		b.WriteString("  ")
		b.WriteString(svc.Name)
		b.WriteString(":\n")
		b.WriteString("    image: ")
		b.WriteString(quote(svc.Image))
		b.WriteString("\n")

		command := append([]string{}, svc.Command...)
		command = append(command, svc.Args...)
		if len(command) > 0 {
			b.WriteString("    command:\n")
			for _, part := range command {
				b.WriteString("      - ")
				b.WriteString(quote(part))
				b.WriteString("\n")
			}
		}

		if len(svc.Env) > 0 {
			b.WriteString("    environment:\n")
			keys := make([]string, 0, len(svc.Env))
			for k := range svc.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("      ")
				b.WriteString(k)
				b.WriteString(": ")
				b.WriteString(quote(svc.Env[k]))
				b.WriteString("\n")
			}
		}

		if len(svc.Ports) > 0 {
			b.WriteString("    ports:\n")
			for _, p := range svc.Ports {
				b.WriteString("      - ")
				b.WriteString(quote(formatPort(p)))
				b.WriteString("\n")
			}
		}

		if len(svc.Volumes) > 0 {
			b.WriteString("    volumes:\n")
			for _, v := range svc.Volumes {
				b.WriteString("      - ")
				b.WriteString(quote(formatVolume(v)))
				b.WriteString("\n")
			}
		}

		if len(svc.DependsOn) > 0 {
			b.WriteString("    depends_on:\n")
			for _, dep := range svc.DependsOn {
				b.WriteString("      - ")
				b.WriteString(dep)
				b.WriteString("\n")
			}
		}

		if svc.RestartPolicy != "" {
			b.WriteString("    restart: ")
			b.WriteString(quote(svc.RestartPolicy))
			b.WriteString("\n")
		}

		if svc.HealthCheck != nil {
			b.WriteString("    healthcheck:\n")
			b.WriteString("      test:\n")
			for _, t := range svc.HealthCheck.Test {
				b.WriteString("        - ")
				b.WriteString(quote(t))
				b.WriteString("\n")
			}
			if svc.HealthCheck.IntervalSec > 0 {
				b.WriteString("      interval: ")
				b.WriteString(quote(fmt.Sprintf("%ds", svc.HealthCheck.IntervalSec)))
				b.WriteString("\n")
			}
			if svc.HealthCheck.TimeoutSec > 0 {
				b.WriteString("      timeout: ")
				b.WriteString(quote(fmt.Sprintf("%ds", svc.HealthCheck.TimeoutSec)))
				b.WriteString("\n")
			}
			if svc.HealthCheck.Retries > 0 {
				b.WriteString("      retries: ")
				b.WriteString(strconv.Itoa(svc.HealthCheck.Retries))
				b.WriteString("\n")
			}
			if svc.HealthCheck.StartPeriodSec > 0 {
				b.WriteString("      start_period: ")
				b.WriteString(quote(fmt.Sprintf("%ds", svc.HealthCheck.StartPeriodSec)))
				b.WriteString("\n")
			}
		}

		b.WriteString("    networks:\n")
		b.WriteString("      - ")
		b.WriteString(networkName)
		b.WriteString("\n")
	}

	if len(desired.Volumes) > 0 {
		b.WriteString("volumes:\n")
		for _, v := range desired.Volumes {
			b.WriteString("  ")
			b.WriteString(v.Name)
			b.WriteString(": {}\n")
		}
	}

	b.WriteString("networks:\n")
	b.WriteString("  ")
	b.WriteString(networkName)
	b.WriteString(": {}\n")

	return b.String()
}

func formatPort(p domain.PortBinding) string {
	proto := strings.ToLower(strings.TrimSpace(p.Protocol))
	if proto == "" {
		proto = "tcp"
	}
	if p.HostPort > 0 {
		return fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, proto)
	}
	return fmt.Sprintf("%d/%s", p.ContainerPort, proto)
}

func formatVolume(v domain.VolumeMount) string {
	mode := ""
	if v.ReadOnly {
		mode = ":ro"
	}
	return fmt.Sprintf("%s:%s%s", v.Source, v.Target, mode)
}

func quote(v string) string {
	return strconv.Quote(v)
}

func composeFilePath(desired domain.DesiredState) string {
	if desired.Metadata != nil {
		if file := strings.TrimSpace(desired.Metadata["compose.file"]); file != "" {
			return file
		}
	}
	for _, a := range desired.Artifacts {
		if strings.EqualFold(filepath.Base(a.Path), "compose.yaml") || strings.EqualFold(filepath.Base(a.Path), "docker-compose.yaml") {
			return a.Path
		}
	}
	return "compose.yaml"
}

func composeProject(desired domain.DesiredState, fallback string) string {
	if desired.Metadata != nil {
		if project := strings.TrimSpace(desired.Metadata["compose.project"]); project != "" {
			return project
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return sanitizeName(fallback)
	}
	return "chainops"
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
