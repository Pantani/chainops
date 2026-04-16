# BGorch API `v1alpha1`

`v1alpha1` models desired topology as a portable core plus typed extension blocks.

## Layering

1. Portable core (`spec.*`)
   - runtime backend selection
   - node pools and per-node workloads
   - volumes, files, health checks, restart policy
   - generic lifecycle/backup/observe placeholders
2. Typed extensions
   - `spec.pluginConfig.*`
   - `spec.runtime.backendConfig.*`

## Canonical Sources

- Go types: `internal/api/v1alpha1/types.go`
- JSON Schema reference: `docs/schema/v1alpha1.schema.json`

## Top-level contract

- `apiVersion`: `bgorch.io/v1alpha1`
- `kind`: `ChainCluster`
- `metadata.name`: DNS-1123 label (validated)
- `spec.family`: chain family identifier
- `spec.plugin`: plugin registry key
- `spec.runtime.backend`: backend registry key
- `spec.nodePools[]`: logical node groups

## Runtime extension blocks

- `spec.runtime.backendConfig.compose`
  - `projectName`
  - `networkName`
  - `outputFile`
- `spec.runtime.backendConfig.sshSystemd`
  - `user`
  - `port`

## Plugin extension blocks

- `spec.pluginConfig.genericProcess.extraFiles[]`
- `spec.pluginConfig.cometBFT`
  - `chainID`, `moniker`
  - `p2pPort`, `rpcPort`, `proxyAppPort`
  - `logLevel`, `pruning`, `minimumGasPrices`
  - `persistentPeers[]`
  - `prometheusEnabled`, `prometheusListenAddr`
  - `apiEnabled`, `grpcEnabled`

Plugins may additionally read node/workload scoped plugin config blocks.

## Defaults applied during load

From `spec.ApplyDefaults`:

- `apiVersion` defaulted to `bgorch.io/v1alpha1`
- `kind` defaulted to `ChainCluster`
- `plugin` defaulted from family (`generic -> generic-process`, else family name)
- compose `outputFile` defaulted to `compose.yaml`
- pool `replicas` defaulted to `1`
- workload `mode` defaulted to `container`
- workload `restartPolicy` defaulted to `unless-stopped`
- port `protocol` defaulted to `tcp`

## Validation model

Validation is layered:

1. core schema/domain checks (`internal/validate`),
2. plugin checks,
3. backend checks.

A valid YAML shape is not sufficient by itself; plugin/backend-specific checks still apply.

## Important Caveat

`docs/schema/v1alpha1.schema.json` is a reference schema for the current contract surface, but behavior is enforced by Go validators and plugin/backend logic. Always treat `internal/api/v1alpha1/types.go` and `internal/validate/validator.go` as implementation truth.
