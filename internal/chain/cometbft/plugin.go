package cometbft

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/chain"
	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/spec"
)

const (
	// PluginName is the registry identifier for the cometbft plugin.
	PluginName = "cometbft-family"
	// FamilyName is the chain family normalized by this plugin.
	FamilyName = "cometbft"
)

// Plugin implements chain.Plugin for cometbft-oriented defaults and artifacts.
type Plugin struct{}

var _ chain.Plugin = (*Plugin)(nil)

// New returns a cometbft-family plugin instance.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Family() string {
	return FamilyName
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

// Validate checks cometbft-family assumptions around workloads, ports, and mounts.
func (p *Plugin) Validate(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)

	if strings.TrimSpace(c.Spec.Plugin) != PluginName {
		diags = append(diags, domain.Error(
			"spec.plugin",
			"cometbft-family plugin selected with mismatched plugin name",
			"set spec.plugin to cometbft-family",
		))
	}
	if strings.TrimSpace(c.Spec.Family) == "" {
		diags = append(diags, domain.Error(
			"spec.family",
			"family must be set",
			"set family to cometbft",
		))
	} else if strings.TrimSpace(c.Spec.Family) != FamilyName {
		diags = append(diags, domain.Warning(
			"spec.family",
			"plugin is optimized for family cometbft",
			"prefer family: cometbft to avoid incompatible defaults",
		))
	}

	for i, pool := range c.Spec.NodePools {
		poolPath := fmt.Sprintf("spec.nodePools[%d].template", i)
		workloadIndex, workload := findCometWorkload(pool.Template.Workloads)
		if workloadIndex < 0 {
			diags = append(diags, domain.Error(
				poolPath+".workloads",
				"cometbft-family requires at least one cometbft workload per node template",
				"define one workload with name/image/binary/command containing cometbft",
			))
			continue
		}

		if !hasPort(workload.Ports, 26656) {
			diags = append(diags, domain.Warning(
				fmt.Sprintf("%s.workloads[%d].ports", poolPath, workloadIndex),
				"p2p port 26656 not declared",
				"declare containerPort 26656 for peer connectivity",
			))
		}
		if !hasPort(workload.Ports, 26657) {
			diags = append(diags, domain.Warning(
				fmt.Sprintf("%s.workloads[%d].ports", poolPath, workloadIndex),
				"rpc port 26657 not declared",
				"declare containerPort 26657 for RPC/health operations",
			))
		}
		if !hasMountWithPrefix(workload.VolumeMounts, "/cometbft") {
			diags = append(diags, domain.Warning(
				fmt.Sprintf("%s.workloads[%d].volumeMounts", poolPath, workloadIndex),
				"no mount under /cometbft found",
				"mount persistent data/config under /cometbft for stateful operation",
			))
		}

		if !hasRelativeFile(pool.Template.Files, "config/genesis.json") {
			diags = append(diags, domain.Warning(
				poolPath+".files",
				"genesis file is not explicitly provided",
				"plugin will render a placeholder genesis.json; override it for real networks",
			))
		}
	}

	return diags
}

// Normalize infers default family/profile/roles when omitted in spec.
func (p *Plugin) Normalize(c *v1alpha1.ChainCluster) error {
	if strings.TrimSpace(c.Spec.Family) == "" {
		c.Spec.Family = FamilyName
	}
	if strings.TrimSpace(c.Spec.Profile) == "" {
		c.Spec.Profile = "validator-single"
	}
	normalizeCometConfig(c.Spec.PluginConfig.CometBFT)

	for i := range c.Spec.NodePools {
		pool := &c.Spec.NodePools[i]
		if len(pool.Roles) == 0 {
			if role := inferRole(pool.Name); role != "" {
				pool.Roles = []string{role}
			}
		}
		if strings.TrimSpace(pool.Template.Role) == "" {
			if len(pool.Roles) > 0 {
				pool.Template.Role = pool.Roles[0]
			} else {
				pool.Template.Role = "full"
			}
		}
		normalizeCometConfig(pool.Template.PluginConfig.CometBFT)
		for j := range pool.Template.Workloads {
			normalizeCometConfig(pool.Template.Workloads[j].PluginConfig.CometBFT)
		}
	}

	return nil
}

// Build renders cometbft-specific node config artifacts deterministically.
func (p *Plugin) Build(ctx context.Context, c *v1alpha1.ChainCluster) (chain.Output, error) {
	_ = ctx

	artifactsByPath := make(map[string]string)
	nodes := spec.ExpandNodes(c)

	for _, n := range nodes {
		role := normalizeRole(n.Spec.Role)
		workloadIndex, workload := findCometWorkload(n.Spec.Workloads)
		var workloadCfg *v1alpha1.CometBFTConfig
		if workloadIndex >= 0 {
			workloadCfg = n.Spec.Workloads[workloadIndex].PluginConfig.CometBFT
		}

		cfg := resolveCometConfig(
			c.Metadata.Name,
			n.Name,
			role,
			workload,
			c.Spec.PluginConfig.CometBFT,
			n.Spec.PluginConfig.CometBFT,
			workloadCfg,
		)
		configToml := renderConfigToml(configInput{
			Moniker:              cfg.Moniker,
			Role:                 role,
			P2PPort:              cfg.P2PPort,
			RPCPort:              cfg.RPCPort,
			ProxyAppPort:         cfg.ProxyAppPort,
			LogLevel:             cfg.LogLevel,
			PEX:                  cfg.PEX,
			SeedMode:             cfg.SeedMode,
			PersistentPeers:      cfg.PersistentPeers,
			PrometheusEnabled:    cfg.PrometheusEnabled,
			PrometheusListenAddr: cfg.PrometheusListenAddr,
		})
		appToml := renderAppToml(appInput{
			ChainID:          cfg.ChainID,
			Pruning:          cfg.Pruning,
			MinimumGasPrices: cfg.MinimumGasPrices,
			APIEnabled:       cfg.APIEnabled,
			GRPCEnabled:      cfg.GRPCEnabled,
		})
		genesisJSON, err := renderGenesisJSON(cfg.ChainID)
		if err != nil {
			return chain.Output{}, fmt.Errorf("render genesis json for node %s: %w", n.Name, err)
		}

		addArtifact(artifactsByPath, filepath.Join("nodes", n.Name, "config", "config.toml"), configToml)
		addArtifact(artifactsByPath, filepath.Join("nodes", n.Name, "config", "app.toml"), appToml)
		if !hasRelativeFile(n.Spec.Files, "config/genesis.json") {
			addArtifact(artifactsByPath, filepath.Join("nodes", n.Name, "config", "genesis.json"), genesisJSON)
		}

		for _, f := range n.Spec.Files {
			addArtifact(artifactsByPath, filepath.Join("nodes", n.Name, filepath.Clean(f.Path)), f.Content)
		}
	}

	paths := make([]string, 0, len(artifactsByPath))
	for path := range artifactsByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	artifacts := make([]domain.Artifact, 0, len(paths))
	for _, path := range paths {
		artifacts = append(artifacts, domain.Artifact{Path: path, Content: artifactsByPath[path]})
	}

	return chain.Output{
		Artifacts: artifacts,
		Metadata: map[string]string{
			"plugin":  p.Name(),
			"family":  c.Spec.Family,
			"profile": c.Spec.Profile,
		},
	}, nil
}

func addArtifact(dst map[string]string, path, content string) {
	dst[safeRelPath(path)] = content
}

func safeRelPath(path string) string {
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/")
	for strings.HasPrefix(path, "../") {
		path = strings.TrimPrefix(path, "../")
	}
	if path == ".." || path == "." || path == "" {
		return "artifact.txt"
	}
	return path
}

func findCometWorkload(workloads []v1alpha1.WorkloadSpec) (int, *v1alpha1.WorkloadSpec) {
	for i := range workloads {
		if isCometWorkload(workloads[i]) {
			return i, &workloads[i]
		}
	}
	return -1, nil
}

func isCometWorkload(w v1alpha1.WorkloadSpec) bool {
	parts := []string{w.Name, w.Image, w.Binary}
	parts = append(parts, w.Command...)
	parts = append(parts, w.Args...)
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), "cometbft") {
			return true
		}
	}
	return false
}

func hasRelativeFile(files []v1alpha1.FileSpec, path string) bool {
	wanted := filepath.ToSlash(filepath.Clean(path))
	for _, f := range files {
		if filepath.ToSlash(filepath.Clean(f.Path)) == wanted {
			return true
		}
	}
	return false
}

func hasPort(ports []v1alpha1.PortSpec, want int) bool {
	for _, p := range ports {
		if p.ContainerPort == want {
			return true
		}
	}
	return false
}

func hasMountWithPrefix(mounts []v1alpha1.VolumeMountSpec, prefix string) bool {
	for _, m := range mounts {
		if strings.HasPrefix(m.Path, prefix) {
			return true
		}
	}
	return false
}

func firstPortByNameOrDefault(ports []v1alpha1.PortSpec, names []string, fallback int) int {
	if len(ports) == 0 {
		return fallback
	}
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	for _, p := range ports {
		if _, ok := nameSet[strings.ToLower(strings.TrimSpace(p.Name))]; ok && p.ContainerPort > 0 {
			return p.ContainerPort
		}
	}
	return fallback
}

func inferRole(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "validator"):
		return "validator"
	case strings.Contains(name, "sentry"):
		return "sentry"
	case strings.Contains(name, "rpc"):
		return "rpc"
	default:
		return ""
	}
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "full"
	}
	return role
}

func sanitizeIdentifier(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	repl := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ".", "-", ":", "-")
	v = repl.Replace(v)
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return !(r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	v = strings.Join(parts, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "node"
	}
	return v
}

type resolvedCometConfig struct {
	ChainID              string
	Moniker              string
	P2PPort              int
	RPCPort              int
	ProxyAppPort         int
	LogLevel             string
	Pruning              string
	MinimumGasPrices     string
	PersistentPeers      string
	PrometheusEnabled    bool
	PrometheusListenAddr string
	APIEnabled           bool
	GRPCEnabled          bool
	PEX                  bool
	SeedMode             bool
}

func resolveCometConfig(
	clusterName string,
	nodeName string,
	role string,
	workload *v1alpha1.WorkloadSpec,
	clusterCfg *v1alpha1.CometBFTConfig,
	nodeCfg *v1alpha1.CometBFTConfig,
	workloadCfg *v1alpha1.CometBFTConfig,
) resolvedCometConfig {
	rpcPort := 26657
	p2pPort := 26656
	proxyAppPort := 26658
	if workload != nil {
		rpcPort = firstPortByNameOrDefault(workload.Ports, []string{"rpc", "rpc-laddr"}, 26657)
		p2pPort = firstPortByNameOrDefault(workload.Ports, []string{"p2p"}, 26656)
		proxyAppPort = firstPortByNameOrDefault(workload.Ports, []string{"abci", "proxy-app"}, 26658)
	}

	normalizedRole := normalizeRole(role)
	merged := mergeCometConfig(clusterCfg, nodeCfg, workloadCfg)

	cfg := resolvedCometConfig{
		ChainID:              fmt.Sprintf("%s-localnet", sanitizeIdentifier(clusterName)),
		Moniker:              sanitizeIdentifier(nodeName),
		P2PPort:              p2pPort,
		RPCPort:              rpcPort,
		ProxyAppPort:         proxyAppPort,
		LogLevel:             "info",
		Pruning:              "default",
		MinimumGasPrices:     "0stake",
		PrometheusEnabled:    true,
		PrometheusListenAddr: ":26660",
		APIEnabled:           normalizedRole != "sentry",
		GRPCEnabled:          normalizedRole != "sentry",
		PEX:                  normalizedRole != "validator",
		SeedMode:             normalizedRole == "sentry",
	}

	if merged.ChainID != "" {
		cfg.ChainID = merged.ChainID
	}
	if merged.Moniker != "" {
		cfg.Moniker = merged.Moniker
	}
	if merged.P2PPort > 0 {
		cfg.P2PPort = merged.P2PPort
	}
	if merged.RPCPort > 0 {
		cfg.RPCPort = merged.RPCPort
	}
	if merged.ProxyAppPort > 0 {
		cfg.ProxyAppPort = merged.ProxyAppPort
	}
	if merged.LogLevel != "" {
		cfg.LogLevel = merged.LogLevel
	}
	if merged.Pruning != "" {
		cfg.Pruning = merged.Pruning
	}
	if merged.MinimumGasPrices != "" {
		cfg.MinimumGasPrices = merged.MinimumGasPrices
	}
	if len(merged.PersistentPeers) > 0 {
		cfg.PersistentPeers = strings.Join(merged.PersistentPeers, ",")
	}
	if merged.PrometheusEnabled != nil {
		cfg.PrometheusEnabled = *merged.PrometheusEnabled
	}
	if merged.PrometheusListenAddr != "" {
		cfg.PrometheusListenAddr = merged.PrometheusListenAddr
	}
	if merged.APIEnabled != nil {
		cfg.APIEnabled = *merged.APIEnabled
	}
	if merged.GRPCEnabled != nil {
		cfg.GRPCEnabled = *merged.GRPCEnabled
	}

	return cfg
}

func mergeCometConfig(configs ...*v1alpha1.CometBFTConfig) v1alpha1.CometBFTConfig {
	var out v1alpha1.CometBFTConfig
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		if chainID := strings.TrimSpace(cfg.ChainID); chainID != "" {
			out.ChainID = chainID
		}
		if moniker := strings.TrimSpace(cfg.Moniker); moniker != "" {
			out.Moniker = sanitizeIdentifier(moniker)
		}
		if cfg.P2PPort > 0 {
			out.P2PPort = cfg.P2PPort
		}
		if cfg.RPCPort > 0 {
			out.RPCPort = cfg.RPCPort
		}
		if cfg.ProxyAppPort > 0 {
			out.ProxyAppPort = cfg.ProxyAppPort
		}
		if logLevel := strings.ToLower(strings.TrimSpace(cfg.LogLevel)); logLevel != "" {
			out.LogLevel = logLevel
		}
		if pruning := strings.ToLower(strings.TrimSpace(cfg.Pruning)); pruning != "" {
			out.Pruning = pruning
		}
		if minGas := strings.TrimSpace(cfg.MinimumGasPrices); minGas != "" {
			out.MinimumGasPrices = minGas
		}
		if len(cfg.PersistentPeers) > 0 {
			out.PersistentPeers = filterNonEmptyPeers(cfg.PersistentPeers)
		}
		if cfg.PrometheusEnabled != nil {
			v := *cfg.PrometheusEnabled
			out.PrometheusEnabled = &v
		}
		if listen := strings.TrimSpace(cfg.PrometheusListenAddr); listen != "" {
			out.PrometheusListenAddr = listen
		}
		if cfg.APIEnabled != nil {
			v := *cfg.APIEnabled
			out.APIEnabled = &v
		}
		if cfg.GRPCEnabled != nil {
			v := *cfg.GRPCEnabled
			out.GRPCEnabled = &v
		}
	}
	return out
}

func filterNonEmptyPeers(peers []string) []string {
	cleaned := make([]string, 0, len(peers))
	for _, peer := range peers {
		if trimmed := strings.TrimSpace(peer); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func normalizeCometConfig(cfg *v1alpha1.CometBFTConfig) {
	if cfg == nil {
		return
	}

	cfg.ChainID = strings.TrimSpace(cfg.ChainID)
	cfg.Moniker = strings.TrimSpace(cfg.Moniker)
	if cfg.Moniker != "" {
		cfg.Moniker = sanitizeIdentifier(cfg.Moniker)
	}
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.Pruning = strings.ToLower(strings.TrimSpace(cfg.Pruning))
	cfg.MinimumGasPrices = strings.TrimSpace(cfg.MinimumGasPrices)
	cfg.PrometheusListenAddr = strings.TrimSpace(cfg.PrometheusListenAddr)
	cfg.PersistentPeers = filterNonEmptyPeers(cfg.PersistentPeers)
}

type configInput struct {
	Moniker              string
	Role                 string
	P2PPort              int
	RPCPort              int
	ProxyAppPort         int
	LogLevel             string
	PEX                  bool
	SeedMode             bool
	PersistentPeers      string
	PrometheusEnabled    bool
	PrometheusListenAddr string
}

func renderConfigToml(in configInput) string {
	var b strings.Builder
	b.WriteString("# Generated by bgorch cometbft-family plugin\n")
	b.WriteString("proxy_app = ")
	b.WriteString(strconv.Quote(fmt.Sprintf("tcp://127.0.0.1:%d", in.ProxyAppPort)))
	b.WriteString("\n")
	b.WriteString("moniker = ")
	b.WriteString(strconv.Quote(in.Moniker))
	b.WriteString("\n")
	b.WriteString("mode = ")
	b.WriteString(strconv.Quote(in.Role))
	b.WriteString("\n")
	b.WriteString("db_backend = \"goleveldb\"\n")
	b.WriteString("db_dir = \"data\"\n")
	b.WriteString("log_level = ")
	b.WriteString(strconv.Quote(in.LogLevel))
	b.WriteString("\n\n")

	b.WriteString("[rpc]\n")
	b.WriteString("laddr = ")
	b.WriteString(strconv.Quote(fmt.Sprintf("tcp://0.0.0.0:%d", in.RPCPort)))
	b.WriteString("\n\n")

	b.WriteString("[p2p]\n")
	b.WriteString("laddr = ")
	b.WriteString(strconv.Quote(fmt.Sprintf("tcp://0.0.0.0:%d", in.P2PPort)))
	b.WriteString("\n")
	b.WriteString("persistent_peers = ")
	b.WriteString(strconv.Quote(in.PersistentPeers))
	b.WriteString("\n")
	b.WriteString("pex = ")
	b.WriteString(strconv.FormatBool(in.PEX))
	b.WriteString("\n")
	b.WriteString("seed_mode = ")
	b.WriteString(strconv.FormatBool(in.SeedMode))
	b.WriteString("\n\n")

	b.WriteString("[consensus]\n")
	b.WriteString("timeout_commit = \"5s\"\n\n")

	b.WriteString("[instrumentation]\n")
	b.WriteString("prometheus = ")
	b.WriteString(strconv.FormatBool(in.PrometheusEnabled))
	b.WriteString("\n")
	b.WriteString("prometheus_listen_addr = ")
	b.WriteString(strconv.Quote(in.PrometheusListenAddr))
	b.WriteString("\n")

	return b.String()
}

type appInput struct {
	ChainID          string
	Pruning          string
	MinimumGasPrices string
	APIEnabled       bool
	GRPCEnabled      bool
}

func renderAppToml(in appInput) string {
	var b strings.Builder
	b.WriteString("# Generated by bgorch cometbft-family plugin\n")
	b.WriteString("chain-id = ")
	b.WriteString(strconv.Quote(in.ChainID))
	b.WriteString("\n")
	b.WriteString("minimum-gas-prices = ")
	b.WriteString(strconv.Quote(in.MinimumGasPrices))
	b.WriteString("\n")
	b.WriteString("pruning = ")
	b.WriteString(strconv.Quote(in.Pruning))
	b.WriteString("\n\n")

	b.WriteString("[api]\n")
	b.WriteString("enable = ")
	b.WriteString(strconv.FormatBool(in.APIEnabled))
	b.WriteString("\n")
	b.WriteString("address = \"tcp://0.0.0.0:1317\"\n\n")

	b.WriteString("[grpc]\n")
	b.WriteString("enable = ")
	b.WriteString(strconv.FormatBool(in.GRPCEnabled))
	b.WriteString("\n")
	b.WriteString("address = \"0.0.0.0:9090\"\n")

	return b.String()
}

func renderGenesisJSON(chainID string) (string, error) {
	type blockParams struct {
		MaxBytes string `json:"max_bytes"`
		MaxGas   string `json:"max_gas"`
	}
	type evidenceParams struct {
		MaxAgeNumBlocks string `json:"max_age_num_blocks"`
		MaxAgeDuration  string `json:"max_age_duration"`
		MaxBytes        string `json:"max_bytes"`
	}
	type validatorParams struct {
		PubKeyTypes []string `json:"pub_key_types"`
	}
	type versionParams struct {
		App string `json:"app"`
	}
	type consensusParams struct {
		Block     blockParams     `json:"block"`
		Evidence  evidenceParams  `json:"evidence"`
		Validator validatorParams `json:"validator"`
		Version   versionParams   `json:"version"`
	}
	type genesisDoc struct {
		GenesisTime     string          `json:"genesis_time"`
		ChainID         string          `json:"chain_id"`
		InitialHeight   string          `json:"initial_height"`
		ConsensusParams consensusParams `json:"consensus_params"`
		Validators      []any           `json:"validators"`
		AppHash         string          `json:"app_hash"`
	}

	doc := genesisDoc{
		GenesisTime:   "2026-01-01T00:00:00Z",
		ChainID:       chainID,
		InitialHeight: "1",
		ConsensusParams: consensusParams{
			Block: blockParams{MaxBytes: "22020096", MaxGas: "-1"},
			Evidence: evidenceParams{
				MaxAgeNumBlocks: "100000",
				MaxAgeDuration:  "172800000000000",
				MaxBytes:        "1048576",
			},
			Validator: validatorParams{PubKeyTypes: []string{"ed25519"}},
			Version:   versionParams{App: "0"},
		},
		Validators: []any{},
		AppHash:    "",
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}
