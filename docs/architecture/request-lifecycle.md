# Request Lifecycle and Runtime Flows

This document describes command execution flow as implemented in `internal/app/app.go` and related packages.

## 1. End-to-end command lifecycle

### `validate`

1. Load YAML via `spec.LoadFromFile`.
2. Apply defaults (`spec.ApplyDefaults`).
3. Resolve plugin/backend from registries.
4. Execute validations:
   - plugin-level (`plugin.Validate`),
   - backend-level (`backend.ValidateTarget`),
   - core schema/domain (`validate.Cluster`).
5. Return diagnostics only (no artifact/state mutation).

### `render`

1. Run the same validation + resolution pipeline.
2. Build desired state:
   - `plugin.Normalize`,
   - `plugin.Build`,
   - `backend.BuildDesired`.
3. Write artifacts to output directory (`renderer.WriteArtifacts`).
4. Optionally persist state snapshot when `--write-state` is enabled.

### `plan`

1. Build desired state.
2. Load snapshot from `.bgorch/state/<cluster>--<backend>.json`.
3. Compare desired vs snapshot hashes using `planner.Build`.
4. Return ordered change list (`create|update|delete|noop`).

### `apply`

1. Build desired state.
2. Acquire exclusive lock `.bgorch/state/<cluster>--<backend>.lock`.
3. Load current snapshot.
4. Build plan.
5. If `--dry-run`, return plan without writing artifacts/snapshot.
6. Write artifacts.
7. If `--runtime-exec`, execute backend runtime action (compose backend only today).
8. Save new snapshot.
9. Release lock.

Failure semantics:

- Lock acquisition failure aborts apply.
- Runtime execution failure aborts apply and does not persist snapshot.
- Lock release runs in `defer`; release errors are surfaced when no prior error exists.

### `status`

1. Validate spec first.
2. Build desired state (if validation has no errors).
3. Load snapshot and compute plan diff.
4. Produce human/machine-readable observations.
5. If `--observe-runtime`, call runtime observer when backend supports it.

Observation failure semantics:

- Runtime observe errors do not fail the command.
- Errors are surfaced in `runtimeObservationError` and observations list.

### `doctor`

`doctor` aggregates checks across:

- spec loading/validation,
- plugin/backend resolution,
- state directory accessibility,
- snapshot readability,
- desired vs snapshot drift,
- optional runtime observation checks.

`doctor` reports:

- `pass` for healthy checks,
- `warn` for degraded/non-blocking conditions,
- `fail` for actionable blocking issues.

## 2. State model and transitions

State backend is filesystem-based under `.bgorch/state`.

Artifacts:

- snapshot file: `<cluster>--<backend>.json`
- lock file: `<cluster>--<backend>.lock`

Snapshot data model:

- `services`: map of service name -> hash(JSON(service))
- `artifacts`: map of artifact path -> hash(content)
- metadata: `version`, `clusterName`, `backend`, `updatedAt`

Transition model:

1. `nil snapshot` + desired -> all resources become `create`.
2. matching hashes -> `noop`.
3. hash mismatch -> `update`.
4. missing in desired/current -> `delete`/`create`.

## 3. Concurrency and locking

Lock scope is `(clusterName, backend)`.

Implementation:

- lock acquisition uses `os.O_CREATE|os.O_EXCL` for atomicity;
- lock file stores `pid` and timestamp metadata;
- lock release is idempotent (`sync.Once`).

Guarantees:

- prevents concurrent local `apply` for the same cluster/backend pair;
- does not provide distributed locking across multiple machines.

## 4. Plugin/backend interaction flow

```text
ChainCluster
  -> Plugin.Validate / Normalize / Build
  -> plugin Output (artifacts + metadata)
  -> Backend.ValidateTarget / BuildDesired
  -> DesiredState (services, volumes, networks, artifacts, metadata)
```

Key constraints:

- plugin owns chain-family semantics;
- backend owns runtime translation/execution semantics;
- core owns orchestration, planning, diagnostics, and state persistence.

## 5. Runtime integrations

### Compose backend (`docker-compose`)

- Always renders compose artifacts.
- Optional runtime actions:
  - `ExecuteRuntime`: `docker compose ... up -d`
  - `ObserveRuntime`: `docker compose ... ps --all`
- Requires rendered compose file in output dir.

### SSH/systemd backend (`ssh-systemd`)

- Validates host-mode workloads.
- Renders systemd units, env files, and node directory layout files.
- Does not execute remote SSH/systemctl actions in current implementation.

## 6. Configuration loading and override rules

Applied defaults (`spec.ApplyDefaults`):

- `apiVersion` -> `bgorch.io/v1alpha1`
- `kind` -> `ChainCluster`
- `spec.plugin` -> `generic-process` when `family=generic`, else `family` value
- compose output file default -> `compose.yaml`
- node pool replicas default -> `1`
- workload mode default -> `container`
- restart policy default -> `unless-stopped`
- port protocol default -> `tcp`

CLI-level option defaults:

- state dir -> `.bgorch/state`
- render/apply output dir -> `.bgorch/render`

## 7. Error handling strategy

BGorch uses two channels:

- diagnostics (`[]domain.Diagnostic`) for user/spec issues,
- returned `error` for operational/internal failures.

Practical effect:

- validation issues are reportable and often non-panicking,
- filesystem/process/runtime failures short-circuit command execution,
- `status`/`doctor` degrade gracefully for runtime observation failures.

## 8. Background jobs / workers / eventing

Current implementation has no persistent background workers, event bus, queue, or scheduler.

- all command paths are synchronous per invocation,
- eventing is represented as immediate command output and doctor checks,
- no asynchronous reconciliation loop exists yet.

## 9. AuthN/AuthZ

There is no built-in authentication or authorization layer in the current codebase.

- CLI operates with local process permissions,
- runtime integrations inherit host/user permissions,
- secret modeling exists in API types, but secret materialization/integration flows are not implemented in the current backend logic.

## 10. Testing strategy (implemented)

- unit tests for validator, planner, locking, doctor model;
- golden tests for compose and ssh-systemd artifact rendering;
- plugin tests for cometbft artifact generation and deterministic output;
- app-layer tests for dry-run behavior, idempotence, runtime execution/observe fallback;
- CLI integration/regression tests for smoke flow and lock contention behavior.
