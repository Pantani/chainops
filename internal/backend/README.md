# `internal/backend`

Backends translate plugin output + spec into runtime-specific desired state and optional runtime actions.

## Contracts

### Required (`Backend`)

- `Name() string`
- `ValidateTarget(spec)`
- `BuildDesired(ctx, spec, pluginOut)`

### Optional runtime capabilities

- `RuntimeExecutor` (`ExecuteRuntime`)
- `RuntimeObserver` (`ObserveRuntime`)

## Implementations

- `compose`: container-mode backend; renders compose artifacts and supports runtime exec/observe.
- `sshsystemd`: host-mode backend; renders systemd/env/layout artifacts only.

## Extension Guidance

When adding a backend:

1. keep output deterministic and sorted,
2. return actionable diagnostics in `ValidateTarget`,
3. keep chain-specific logic out of backend code,
4. implement runtime interfaces only when execution/observation is truly supported.
