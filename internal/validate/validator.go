package validate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Pantani/gorchestrator/internal/api/v1alpha1"
	"github.com/Pantani/gorchestrator/internal/domain"
)

var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Cluster performs core semantic validation independent from plugin/backend internals.
func Cluster(c *v1alpha1.ChainCluster) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	pluginName := strings.TrimSpace(c.Spec.Plugin)
	isCometBFTPlugin := pluginName == "cometbft-family"
	isEVMPlugin := pluginName == "evm-family"
	isSolanaPlugin := pluginName == "solana-family"
	isBitcoinPlugin := pluginName == "bitcoin-family"
	isCosmosPlugin := pluginName == "cosmos-family"

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
	expandedNodeNames := map[string]string{}
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
		namePrefix := strings.TrimSpace(pool.Template.Name)
		namePath := poolPath + ".template.name"
		if namePrefix == "" {
			namePrefix = strings.TrimSpace(pool.Name)
			namePath = poolPath + ".name"
		}
		if namePrefix != "" && pool.Replicas > 0 {
			for replica := 0; replica < pool.Replicas; replica++ {
				resolvedName := namePrefix
				if pool.Replicas > 1 {
					resolvedName = fmt.Sprintf("%s-%02d", namePrefix, replica)
				}
				if firstPath, exists := expandedNodeNames[resolvedName]; exists {
					diags = append(diags, domain.Error(
						namePath,
						"duplicate expanded node name",
						fmt.Sprintf("resolved node name %q already produced by %s", resolvedName, firstPath),
					))
					continue
				}
				expandedNodeNames[resolvedName] = namePath
			}
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
			if rp := strings.TrimSpace(string(w.RestartPolicy)); rp != "" && !isSupportedRestartPolicy(w.RestartPolicy) {
				diags = append(diags, domain.Error(
					wPath+".restartPolicy",
					"unsupported restart policy",
					"use one of: always, unless-stopped, on-failure",
				))
			}

			if w.Image != "" && w.Binary != "" {
				diags = append(diags, domain.Warning(wPath, "workload has both image and binary", "binary will be ignored by container-focused backends"))
			}
			envNames := map[string]struct{}{}
			for eIdx, env := range w.Env {
				envPath := fmt.Sprintf("%s.env[%d]", wPath, eIdx)
				name := strings.TrimSpace(env.Name)
				if name == "" {
					diags = append(diags, domain.Error(envPath+".name", "env name is required", "set a non-empty environment variable name"))
					continue
				}
				if strings.Contains(name, "=") || strings.ContainsAny(name, " \t\r\n") {
					diags = append(diags, domain.Error(envPath+".name", "invalid env name", "avoid whitespace and '=' in environment variable names"))
				}
				if _, exists := envNames[name]; exists {
					diags = append(diags, domain.Error(envPath+".name", "duplicate env name", "environment variable names must be unique per workload"))
					continue
				}
				envNames[name] = struct{}{}
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

			cometWorkloadCfgPath := fmt.Sprintf("%s.pluginConfig.cometBFT", wPath)
			if w.PluginConfig.CometBFT != nil {
				if isCometBFTPlugin {
					diags = append(diags, validateCometBFTConfig(cometWorkloadCfgPath, w.PluginConfig.CometBFT)...)
				} else {
					diags = append(diags, domain.Warning(cometWorkloadCfgPath, "cometBFT config provided for non-cometbft plugin", "move cometBFT config to a spec that uses plugin cometbft-family"))
				}
			}

			evmWorkloadCfgPath := fmt.Sprintf("%s.pluginConfig.evm", wPath)
			if w.PluginConfig.EVM != nil {
				if isEVMPlugin {
					diags = append(diags, validateEVMConfig(evmWorkloadCfgPath, w.PluginConfig.EVM)...)
				} else {
					diags = append(diags, domain.Warning(evmWorkloadCfgPath, "evm config provided for non-evm plugin", "move evm config to a spec that uses plugin evm-family"))
				}
			}

			solanaWorkloadCfgPath := fmt.Sprintf("%s.pluginConfig.solana", wPath)
			if w.PluginConfig.Solana != nil {
				if isSolanaPlugin {
					diags = append(diags, validateSolanaConfig(solanaWorkloadCfgPath, w.PluginConfig.Solana)...)
				} else {
					diags = append(diags, domain.Warning(solanaWorkloadCfgPath, "solana config provided for non-solana plugin", "move solana config to a spec that uses plugin solana-family"))
				}
			}

			bitcoinWorkloadCfgPath := fmt.Sprintf("%s.pluginConfig.bitcoin", wPath)
			if w.PluginConfig.Bitcoin != nil {
				if isBitcoinPlugin {
					diags = append(diags, validateBitcoinConfig(bitcoinWorkloadCfgPath, w.PluginConfig.Bitcoin)...)
				} else {
					diags = append(diags, domain.Warning(bitcoinWorkloadCfgPath, "bitcoin config provided for non-bitcoin plugin", "move bitcoin config to a spec that uses plugin bitcoin-family"))
				}
			}

			cosmosWorkloadCfgPath := fmt.Sprintf("%s.pluginConfig.cosmos", wPath)
			if w.PluginConfig.Cosmos != nil {
				if isCosmosPlugin {
					diags = append(diags, validateCosmosConfig(cosmosWorkloadCfgPath, w.PluginConfig.Cosmos)...)
				} else {
					diags = append(diags, domain.Warning(cosmosWorkloadCfgPath, "cosmos config provided for non-cosmos plugin", "move cosmos config to a spec that uses plugin cosmos-family"))
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

		cometNodeCfgPath := fmt.Sprintf("%s.template.pluginConfig.cometBFT", poolPath)
		if pool.Template.PluginConfig.CometBFT != nil {
			if isCometBFTPlugin {
				diags = append(diags, validateCometBFTConfig(cometNodeCfgPath, pool.Template.PluginConfig.CometBFT)...)
			} else {
				diags = append(diags, domain.Warning(cometNodeCfgPath, "cometBFT config provided for non-cometbft plugin", "move cometBFT config to a spec that uses plugin cometbft-family"))
			}
		}

		evmNodeCfgPath := fmt.Sprintf("%s.template.pluginConfig.evm", poolPath)
		if pool.Template.PluginConfig.EVM != nil {
			if isEVMPlugin {
				diags = append(diags, validateEVMConfig(evmNodeCfgPath, pool.Template.PluginConfig.EVM)...)
			} else {
				diags = append(diags, domain.Warning(evmNodeCfgPath, "evm config provided for non-evm plugin", "move evm config to a spec that uses plugin evm-family"))
			}
		}

		solanaNodeCfgPath := fmt.Sprintf("%s.template.pluginConfig.solana", poolPath)
		if pool.Template.PluginConfig.Solana != nil {
			if isSolanaPlugin {
				diags = append(diags, validateSolanaConfig(solanaNodeCfgPath, pool.Template.PluginConfig.Solana)...)
			} else {
				diags = append(diags, domain.Warning(solanaNodeCfgPath, "solana config provided for non-solana plugin", "move solana config to a spec that uses plugin solana-family"))
			}
		}

		bitcoinNodeCfgPath := fmt.Sprintf("%s.template.pluginConfig.bitcoin", poolPath)
		if pool.Template.PluginConfig.Bitcoin != nil {
			if isBitcoinPlugin {
				diags = append(diags, validateBitcoinConfig(bitcoinNodeCfgPath, pool.Template.PluginConfig.Bitcoin)...)
			} else {
				diags = append(diags, domain.Warning(bitcoinNodeCfgPath, "bitcoin config provided for non-bitcoin plugin", "move bitcoin config to a spec that uses plugin bitcoin-family"))
			}
		}

		cosmosNodeCfgPath := fmt.Sprintf("%s.template.pluginConfig.cosmos", poolPath)
		if pool.Template.PluginConfig.Cosmos != nil {
			if isCosmosPlugin {
				diags = append(diags, validateCosmosConfig(cosmosNodeCfgPath, pool.Template.PluginConfig.Cosmos)...)
			} else {
				diags = append(diags, domain.Warning(cosmosNodeCfgPath, "cosmos config provided for non-cosmos plugin", "move cosmos config to a spec that uses plugin cosmos-family"))
			}
		}
	}

	if c.Spec.Plugin != "generic-process" && c.Spec.PluginConfig.GenericProcess != nil {
		diags = append(diags, domain.Warning("spec.pluginConfig.genericProcess", "genericProcess config provided for non-generic plugin", "move plugin-specific config to the right plugin extension"))
	}
	if c.Spec.PluginConfig.CometBFT != nil {
		if isCometBFTPlugin {
			diags = append(diags, validateCometBFTConfig("spec.pluginConfig.cometBFT", c.Spec.PluginConfig.CometBFT)...)
		} else {
			diags = append(diags, domain.Warning("spec.pluginConfig.cometBFT", "cometBFT config provided for non-cometbft plugin", "move cometBFT config to a spec that uses plugin cometbft-family"))
		}
	}
	if c.Spec.PluginConfig.EVM != nil {
		if isEVMPlugin {
			diags = append(diags, validateEVMConfig("spec.pluginConfig.evm", c.Spec.PluginConfig.EVM)...)
		} else {
			diags = append(diags, domain.Warning("spec.pluginConfig.evm", "evm config provided for non-evm plugin", "move evm config to a spec that uses plugin evm-family"))
		}
	}
	if c.Spec.PluginConfig.Solana != nil {
		if isSolanaPlugin {
			diags = append(diags, validateSolanaConfig("spec.pluginConfig.solana", c.Spec.PluginConfig.Solana)...)
		} else {
			diags = append(diags, domain.Warning("spec.pluginConfig.solana", "solana config provided for non-solana plugin", "move solana config to a spec that uses plugin solana-family"))
		}
	}
	if c.Spec.PluginConfig.Bitcoin != nil {
		if isBitcoinPlugin {
			diags = append(diags, validateBitcoinConfig("spec.pluginConfig.bitcoin", c.Spec.PluginConfig.Bitcoin)...)
		} else {
			diags = append(diags, domain.Warning("spec.pluginConfig.bitcoin", "bitcoin config provided for non-bitcoin plugin", "move bitcoin config to a spec that uses plugin bitcoin-family"))
		}
	}
	if c.Spec.PluginConfig.Cosmos != nil {
		if isCosmosPlugin {
			diags = append(diags, validateCosmosConfig("spec.pluginConfig.cosmos", c.Spec.PluginConfig.Cosmos)...)
		} else {
			diags = append(diags, domain.Warning("spec.pluginConfig.cosmos", "cosmos config provided for non-cosmos plugin", "move cosmos config to a spec that uses plugin cosmos-family"))
		}
	}

	return diags
}

func validateCometBFTConfig(path string, cfg *v1alpha1.CometBFTConfig) []domain.Diagnostic {
	if cfg == nil {
		return nil
	}

	diags := make([]domain.Diagnostic, 0)
	if strings.Contains(strings.TrimSpace(cfg.ChainID), " ") {
		diags = append(diags, domain.Error(path+".chainID", "chainID must not contain spaces", "use canonical chain IDs like cometbft-localnet"))
	}

	diags = append(diags, validateOptionalPort(path+".p2pPort", cfg.P2PPort)...)
	diags = append(diags, validateOptionalPort(path+".rpcPort", cfg.RPCPort)...)
	diags = append(diags, validateOptionalPort(path+".proxyAppPort", cfg.ProxyAppPort)...)

	if cfg.LogLevel != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.LogLevel)) {
		case "trace", "debug", "info", "warn", "error":
		default:
			diags = append(diags, domain.Error(path+".logLevel", "unsupported cometBFT logLevel", "use one of: trace, debug, info, warn, error"))
		}
	}

	if cfg.Pruning != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Pruning)) {
		case "default", "nothing", "everything", "custom":
		default:
			diags = append(diags, domain.Error(path+".pruning", "unsupported cometBFT pruning mode", "use one of: default, nothing, everything, custom"))
		}
	}

	for i, peer := range cfg.PersistentPeers {
		if strings.TrimSpace(peer) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.persistentPeers[%d]", path, i), "persistent peer entry cannot be empty", "remove empty entries or provide nodeID@host:port"))
		}
	}

	if cfg.PrometheusListenAddr != "" && !strings.Contains(cfg.PrometheusListenAddr, ":") {
		diags = append(diags, domain.Error(path+".prometheusListenAddr", "prometheus listen address is invalid", "use host:port or :port"))
	}

	return diags
}

func validateEVMConfig(path string, cfg *v1alpha1.EVMConfig) []domain.Diagnostic {
	if cfg == nil {
		return nil
	}

	diags := make([]domain.Diagnostic, 0)

	if cfg.Client != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Client)) {
		case "geth", "go-ethereum", "erigon", "nethermind", "besu", "reth":
		default:
			diags = append(diags, domain.Error(path+".client", "unsupported evm client", "use geth, erigon, nethermind, besu, or reth"))
		}
	}
	if cfg.Network != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Network)) {
		case "mainnet", "sepolia", "holesky", "devnet", "local":
		default:
			diags = append(diags, domain.Error(path+".network", "unsupported evm network", "use mainnet, sepolia, holesky, devnet, or local"))
		}
	}
	if cfg.SyncMode != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.SyncMode)) {
		case "full", "snap", "archive", "light":
		default:
			diags = append(diags, domain.Error(path+".syncMode", "unsupported evm sync mode", "use one of: full, snap, archive, light"))
		}
	}
	if cfg.ChainID < 0 {
		diags = append(diags, domain.Error(path+".chainID", "chainID must be >= 0", "set a positive chain ID or omit to use defaults"))
	}

	diags = append(diags, validateOptionalPort(path+".p2pPort", cfg.P2PPort)...)
	diags = append(diags, validateOptionalPort(path+".httpPort", cfg.HTTPPort)...)
	diags = append(diags, validateOptionalPort(path+".wsPort", cfg.WSPort)...)
	diags = append(diags, validateOptionalPort(path+".authRPCPort", cfg.AuthRPCPort)...)
	diags = append(diags, validateOptionalPort(path+".metricsPort", cfg.MetricsPort)...)

	for i, bootnode := range cfg.Bootnodes {
		if strings.TrimSpace(bootnode) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.bootnodes[%d]", path, i), "bootnode entry cannot be empty", "remove empty entries or provide valid bootnode URLs"))
		}
	}

	return diags
}

func validateSolanaConfig(path string, cfg *v1alpha1.SolanaConfig) []domain.Diagnostic {
	if cfg == nil {
		return nil
	}

	diags := make([]domain.Diagnostic, 0)

	if cfg.Client != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Client)) {
		case "agave", "solana-validator", "jito":
		default:
			diags = append(diags, domain.Error(path+".client", "unsupported solana client", "use agave or jito"))
		}
	}
	if cfg.Cluster != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Cluster)) {
		case "mainnet-beta", "testnet", "devnet", "localnet":
		default:
			diags = append(diags, domain.Error(path+".cluster", "unsupported solana cluster", "use mainnet-beta, testnet, devnet, or localnet"))
		}
	}

	diags = append(diags, validateOptionalPort(path+".rpcPort", cfg.RPCPort)...)
	diags = append(diags, validateOptionalPort(path+".wsPort", cfg.WSPort)...)
	diags = append(diags, validateOptionalPort(path+".gossipPort", cfg.GossipPort)...)

	if cfg.DynamicPortRange != "" {
		parts := strings.Split(strings.TrimSpace(cfg.DynamicPortRange), "-")
		if len(parts) != 2 {
			diags = append(diags, domain.Error(path+".dynamicPortRange", "dynamicPortRange must be start-end", "use values like 8000-8020"))
		} else {
			start, startErr := strconv.Atoi(strings.TrimSpace(parts[0]))
			end, endErr := strconv.Atoi(strings.TrimSpace(parts[1]))
			if startErr != nil || endErr != nil || start < 1 || end < start || end > 65535 {
				diags = append(diags, domain.Error(path+".dynamicPortRange", "dynamicPortRange is invalid", "use values like 8000-8020 within valid port range"))
			}
		}
	}

	for i, entrypoint := range cfg.EntryPoints {
		if strings.TrimSpace(entrypoint) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.entryPoints[%d]", path, i), "entrypoint cannot be empty", "remove empty entries or provide host:port"))
		}
	}

	return diags
}

func validateBitcoinConfig(path string, cfg *v1alpha1.BitcoinConfig) []domain.Diagnostic {
	if cfg == nil {
		return nil
	}

	diags := make([]domain.Diagnostic, 0)

	if cfg.Client != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Client)) {
		case "bitcoin-core", "bitcoind", "btcd":
		default:
			diags = append(diags, domain.Error(path+".client", "unsupported bitcoin client", "use bitcoin-core or btcd"))
		}
	}
	if cfg.Network != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Network)) {
		case "mainnet", "testnet", "signet", "regtest":
		default:
			diags = append(diags, domain.Error(path+".network", "unsupported bitcoin network", "use mainnet, testnet, signet, or regtest"))
		}
	}

	diags = append(diags, validateOptionalPort(path+".rpcPort", cfg.RPCPort)...)
	diags = append(diags, validateOptionalPort(path+".p2pPort", cfg.P2PPort)...)

	if cfg.PruneMB < 0 {
		diags = append(diags, domain.Error(path+".pruneMB", "pruneMB must be >= 0", "set 0 to disable pruning or a positive MB value"))
	}

	if cfg.ZMQBlockAddr != "" && !validZMQAddress(cfg.ZMQBlockAddr) {
		diags = append(diags, domain.Error(path+".zmqBlockAddr", "invalid ZMQ block address", "use tcp://host:port or ipc://path"))
	}
	if cfg.ZMQTxAddr != "" && !validZMQAddress(cfg.ZMQTxAddr) {
		diags = append(diags, domain.Error(path+".zmqTxAddr", "invalid ZMQ tx address", "use tcp://host:port or ipc://path"))
	}

	for i, arg := range cfg.ExtraArgs {
		if strings.TrimSpace(arg) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.extraArgs[%d]", path, i), "extra arg cannot be empty", "remove empty entries"))
		}
	}

	return diags
}

func validateCosmosConfig(path string, cfg *v1alpha1.CosmosConfig) []domain.Diagnostic {
	if cfg == nil {
		return nil
	}

	diags := make([]domain.Diagnostic, 0)

	if cfg.Client != "" && strings.Contains(strings.TrimSpace(cfg.Client), " ") {
		diags = append(diags, domain.Error(path+".client", "client must not contain spaces", "use compact identifiers like cosmos-sdk, gaiad, osmosisd"))
	}
	if strings.Contains(strings.TrimSpace(cfg.ChainID), " ") {
		diags = append(diags, domain.Error(path+".chainID", "chainID must not contain spaces", "use canonical chain IDs like cosmoshub-4"))
	}
	if cfg.DaemonBinary != "" && strings.Contains(strings.TrimSpace(cfg.DaemonBinary), " ") {
		diags = append(diags, domain.Error(path+".daemonBinary", "daemonBinary must not contain spaces", "set the binary name only, e.g. gaiad"))
	}
	if cfg.HomeDir != "" && !strings.HasPrefix(strings.TrimSpace(cfg.HomeDir), "/") {
		diags = append(diags, domain.Error(path+".homeDir", "homeDir must be an absolute path", "use paths like /var/lib/cosmos"))
	}

	diags = append(diags, validateOptionalPort(path+".p2pPort", cfg.P2PPort)...)
	diags = append(diags, validateOptionalPort(path+".rpcPort", cfg.RPCPort)...)
	diags = append(diags, validateOptionalPort(path+".apiPort", cfg.APIPort)...)
	diags = append(diags, validateOptionalPort(path+".grpcPort", cfg.GRPCPort)...)

	if cfg.Pruning != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Pruning)) {
		case "default", "nothing", "everything", "custom":
		default:
			diags = append(diags, domain.Error(path+".pruning", "unsupported cosmos pruning mode", "use one of: default, nothing, everything, custom"))
		}
	}

	for i, seed := range cfg.Seeds {
		if strings.TrimSpace(seed) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.seeds[%d]", path, i), "seed entry cannot be empty", "remove empty entries or provide nodeID@host:port"))
		}
	}
	for i, peer := range cfg.PersistentPeers {
		if strings.TrimSpace(peer) == "" {
			diags = append(diags, domain.Error(fmt.Sprintf("%s.persistentPeers[%d]", path, i), "persistent peer entry cannot be empty", "remove empty entries or provide nodeID@host:port"))
		}
	}

	return diags
}

func validZMQAddress(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "tcp://") || strings.HasPrefix(value, "ipc://")
}

func validateOptionalPort(path string, port int) []domain.Diagnostic {
	if port == 0 {
		return nil
	}
	if port < 1 || port > 65535 {
		return []domain.Diagnostic{
			domain.Error(path, "invalid port", "valid range is 1-65535"),
		}
	}
	return nil
}

func isSupportedRestartPolicy(policy v1alpha1.RestartPolicy) bool {
	switch strings.ToLower(strings.TrimSpace(string(policy))) {
	case string(v1alpha1.RestartAlways), string(v1alpha1.RestartUnlessStopped), string(v1alpha1.RestartOnFailure):
		return true
	default:
		return false
	}
}
