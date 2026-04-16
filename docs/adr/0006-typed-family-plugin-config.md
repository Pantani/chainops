# ADR 0006: Typed Family Plugin Config (`pluginConfig.<family>`)

- Status: Accepted
- Date: 2026-04-16

## Context

`cometbft-family` is implemented and needs family-specific knobs with typed validation.  
`v1alpha1` already has typed plugin extension blocks and must keep portable core schema clean.

Without typed family blocks, plugin-specific behavior tends to drift into ad-hoc defaults and weak validation.

## Decision

Adopt typed extension blocks under `spec.pluginConfig` for chain families, starting with:

- `spec.pluginConfig.cometBFT` (first family-specific reference implementation).

Rules:

1. Keep portable fields in common schema; put family-only knobs in typed extension blocks.
2. Avoid untyped `map[string]any` payloads for plugin extensions.
3. Family plugin validates its own typed block and owns defaults/normalization.
4. Unknown extension blocks remain invalid at schema/validation layer.

## Implemented Scope (CometBFT reference)

Implemented typed fields for `pluginConfig.cometBFT`:

- `chainID`
- `moniker`
- `p2pPort`
- `rpcPort`
- `proxyAppPort`
- `logLevel`
- `pruning`
- `minimumGasPrices`
- `persistentPeers[]`
- `prometheusEnabled`
- `prometheusListenAddr`
- `apiEnabled`
- `grpcEnabled`

## Current Status

| Item | Status |
|---|---|
| `pluginConfig.genericProcess` | Implemented |
| `pluginConfig.cometBFT` | Implemented |
| `cometbft-family` typed config consumption | Implemented (cluster/node/workload precedence) |

## Consequences

### Positive

- Stronger validation and clearer UX for family-specific parameters.
- Better forward compatibility for plugin evolution.
- Preserves clean boundary between portable core and family logic.

### Negative

- Requires schema and validator evolution for each family block.
- Increases migration surface for early adopters when fields evolve.
- Adds maintenance cost to keep docs/examples/tests aligned with typed blocks.

## Follow-ups

1. Keep `docs/schema/v1alpha1.schema.json` aligned with Go types and validator rules.
2. Add optional typed blocks for additional families (Ethereum/Bitcoin/others) without touching core portable fields.
3. Define migration/versioning strategy for future API versions (`v1beta1+`).
