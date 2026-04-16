# BGorch Architecture and Maturity Snapshot

Status date: **2026-04-16**.

## Architectural Axes

BGorch is split into three independent concerns:

1. Core orchestration (`internal/app`, `internal/planner`, `internal/state`, `internal/renderer`)
2. Chain-family plugins (`internal/chain/*`)
3. Runtime backends (`internal/backend/*`)

This keeps chain semantics out of the backend layer and runtime details out of the core planner/state model.

## Maturity by Capability

| Capability | Status | Notes |
|---|---|---|
| API `bgorch.io/v1alpha1` | Implemented | Go types + schema docs available. |
| CLI `validate`/`render`/`plan` | Implemented | Deterministic artifact pipeline and diffing. |
| CLI `apply` with lock/snapshot | Implemented | Local-state-first; lock per `(cluster, backend)`. |
| CLI `status`/`doctor` | Implemented | Local snapshot + diagnostics/checks. |
| Compose runtime execution (`--runtime-exec`) | Implemented | Backend-gated, explicit opt-in flag. |
| Compose runtime observation (`--observe-runtime`) | Implemented | Available in `status` and `doctor`. |
| Plugin `generic-process` | Implemented | Generic fallback plugin. |
| Plugin `cometbft-family` | Implemented | Validation + generated config/app/genesis artifacts. |
| Backend `docker-compose` | Implemented | Render + runtime exec/observe. |
| Backend `ssh-systemd` | Implemented | Host-mode validation + systemd/env/layout render + optional runtime preflight/observation. |
| `ssh-systemd` runtime exec/observe | Implemented (preflight-gated) | Uses `ssh` + `systemctl`, requires targets/artifacts and explicit runtime flags. |
| Typed `pluginConfig.cometBFT` | Implemented | CometBFT plugin consumes typed config with scope precedence (cluster -> node -> workload). |
| Backend `kubernetes` | Planned | Not yet implemented. |
| Terraform/Ansible adapters | Planned | Not yet implemented. |

## Core Flow (Implemented)

1. CLI loads and defaults the spec.
2. Core resolves plugin/backend from registries.
3. Validation combines core + plugin + backend diagnostics.
4. Plugin normalizes + builds plugin output.
5. Backend builds desired state.
6. Planner diffs desired vs snapshot hashes.
7. Commands render, apply, inspect, and diagnose from the same desired model.

## Non-negotiable Boundaries

- Core does not encode chain-specific protocol logic.
- Plugins do not own runtime process orchestration semantics.
- Backends do not own chain-family business rules.
- Planner/state remain backend-agnostic and deterministic.

## Known Gaps

- lock/state are local only (single-machine safety);
- no distributed reconciliation loop;
- no distributed lock/state backend yet;
- runtime operations are synchronous and command-driven (no controller loop);
- several schema fields are modeled in API types but not yet consumed end-to-end by backends.

## Decision References

- `docs/adr/0001-core-architecture.md`
- `docs/adr/0002-plugin-model.md`
- `docs/adr/0003-backend-model.md`
- `docs/adr/0004-mvp-locking-and-command-semantics.md`
- `docs/adr/0005-optional-runtime-execution-compose.md`
- `docs/adr/0006-typed-family-plugin-config.md`
