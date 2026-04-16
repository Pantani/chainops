# Operational Guide: Commands and Plugin/Backend Integration

This guide reflects the current implementation in the repository.

## Command Matrix

| Command | Implemented | Notes |
|---|---|---|
| `validate -f <spec>` | Yes | Runs plugin + backend + core validation. |
| `render -f <spec> -o <dir>` | Yes | Writes desired artifacts to disk. |
| `render --write-state` | Yes | Persists desired snapshot without runtime execution. |
| `plan -f <spec>` | Yes | Diff is snapshot-based (local filesystem). |
| `apply -f <spec>` | Yes | Lock + plan + artifact write + snapshot save. |
| `apply --dry-run` | Yes | Computes plan under lock, no writes. |
| `apply --runtime-exec` | Yes (backend-gated) | Executes backend runtime operation after successful render. |
| `status -f <spec>` | Yes | Desired vs snapshot summary. |
| `status --observe-runtime` | Yes (backend-gated) | Runs runtime observe when backend supports it and prerequisites are met. |
| `doctor -f <spec>` | Yes | Aggregated operational checks. |
| `doctor --observe-runtime` | Yes (backend-gated) | Adds runtime observe check when backend supports it and prerequisites are met. |

## Recommended Flow

```bash
go run ./cmd/bgorch validate -f examples/generic-single-compose.yaml
go run ./cmd/bgorch plan -f examples/generic-single-compose.yaml
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml
```

When you need compose runtime execution/observation:

```bash
go run ./cmd/bgorch apply  -f examples/generic-single-compose.yaml -o .bgorch/render --runtime-exec
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
```

When you need `ssh-systemd` runtime execution/observation (requires `spec.runtime.target` hosts and SSH connectivity):

```bash
go run ./cmd/bgorch apply  -f examples/generic-single-ssh-systemd.yaml -o .bgorch/render --runtime-exec
go run ./cmd/bgorch status -f examples/generic-single-ssh-systemd.yaml -o .bgorch/render --observe-runtime
go run ./cmd/bgorch doctor -f examples/generic-single-ssh-systemd.yaml -o .bgorch/render --observe-runtime
```

## Current Semantics

### `apply`

- always resolves plugin/backend and builds desired state first;
- acquires lock by `(cluster, backend)`;
- computes plan against snapshot;
- writes artifacts unless `--dry-run`;
- optionally executes backend runtime if `--runtime-exec` is set and backend supports it;
- persists snapshot only after successful artifact write and optional runtime execution.

### `status`

- validates spec and computes desired-vs-snapshot diff;
- reports observations regardless of runtime availability;
- runtime observation errors are surfaced as non-fatal fields/messages.

### `doctor`

- emits `pass/warn/fail` checks for spec, registry resolution, state access, snapshot health, drift;
- runtime observation is optional and warning-based on failure.

## Backend Capability Matrix

| Backend | BuildDesired | Runtime Exec | Runtime Observe | Notes |
|---|---|---|---|---|
| `docker-compose` | Yes | Yes | Yes | Requires Docker/Compose and rendered compose file. |
| `ssh-systemd` | Yes | Yes | Yes | Requires rendered artifacts, runtime targets, `ssh` binary and remote `systemctl`. |

## Plugin and Backend Contracts

### Plugin contract (`internal/chain.Plugin`)

- `Validate(spec)`
- `Normalize(spec)`
- `Build(ctx, spec) -> Output`
- `Capabilities()`

Responsibilities:

- chain-family specific semantics;
- plugin-specific artifact generation;
- defaults/inference inside chain scope.

### Backend contract (`internal/backend.Backend`)

- `ValidateTarget(spec)`
- `BuildDesired(ctx, spec, pluginOut)`

Optional runtime capabilities:

- `RuntimeExecutor` (`ExecuteRuntime`)
- `RuntimeObserver` (`ObserveRuntime`)

Responsibilities:

- runtime-specific validation and artifact translation;
- optional runtime command execution/observation.

## Compatibility Matrix

| Plugin | Backend | Status |
|---|---|---|
| `generic-process` | `docker-compose` | Supported |
| `generic-process` | `ssh-systemd` | Supported |
| `cometbft-family` | `docker-compose` | Supported |
| `cometbft-family` | `ssh-systemd` | Limited by workload mode constraints |

`ssh-systemd` only supports host workloads; compose backend only supports container workloads.

## Typed Extension Matrix

| Extension Block | Status | Notes |
|---|---|---|
| `spec.pluginConfig.genericProcess` | Implemented | Used by `generic-process` plugin (`extraFiles`). |
| `spec.pluginConfig.cometBFT` | Implemented | Typed extension consumed by `cometbft-family` at cluster/node/workload scope (ADR 0006). |
| `spec.runtime.backendConfig.compose` | Implemented | Compose project/network/output tuning. |
| `spec.runtime.backendConfig.sshSystemd` | Implemented | Connection metadata (`user`, `port`) for artifact generation. |

## Known Operational Limits

- snapshot/lock model is local filesystem based;
- no distributed lock or shared state store;
- runtime ops fail fast if backend preflight is not satisfied (for example: missing compose/ssh/systemctl, missing runtime targets, missing rendered artifacts);
- no asynchronous reconciliation loop/background workers.
