# BGorch - The Blockchain Gorchestrator

BGorch is a Go-first declarative orchestrator for multi-blockchain operations.
CLI binary: `bgorch`.

It separates:
- a generic control-plane core (`validate`/`render`/`plan`, state, diagnostics),
- chain-family extensions (plugins),
- runtime adapters (backends).

## Status (2026-04-16)

### Implemented
- Versioned API: `bgorch.io/v1alpha1`
- CLI commands: `validate`, `render`, `plan`, `apply`, `status`, `doctor`
- Plugin registry + backend registry
- Plugin: `generic-process`
- Backend: `docker-compose` (deterministic compose render)
- Backend: `ssh-systemd` (host workloads with systemd artifacts)
- Local snapshot state for diff/plan
- Local file lock for `apply` by `(cluster, backend)` in `.bgorch/state`
- Unit + golden tests for validation/planner/compose rendering

### Planned (not implemented yet)
- Commands: `backup`, `restore`
- Real reference plugin (e.g. `cometbft-family`)
- Kubernetes backend + Terraform/Ansible adapters

## Architectural docs

- Research foundation: `docs/research/infra-foundations.md`
- Core architecture ADR: `docs/adr/0001-core-architecture.md`
- Plugin model ADR: `docs/adr/0002-plugin-model.md`
- Backend model ADR: `docs/adr/0003-backend-model.md`
- Command semantics + lock model (MVP): `docs/adr/0004-mvp-locking-and-command-semantics.md`
- Fase 2/3 architecture snapshot: `docs/architecture/phase-2-3.md`
- Operational command guide: `docs/operations/commands-and-integration.md`

## Quickstart

```bash
# 1) Validate spec
go run ./cmd/bgorch validate -f examples/generic-single-compose.yaml

# 2) Render desired artifacts (compose + plugin files)
go run ./cmd/bgorch render -f examples/generic-single-compose.yaml -o .bgorch/render

# 3) Compare desired state with local snapshot
go run ./cmd/bgorch plan -f examples/generic-single-compose.yaml

# 4) Apply (MVP: write rendered artifacts + snapshot under lock)
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render

# 5) Inspect local convergence and diagnostics
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml
```

## Current CLI surface

The current CLI commands are:
- `validate`
- `render`
- `plan`
- `apply`
- `status`
- `doctor`

## Principles enforced in the MVP

- Core is not Terraform/Ansible/Compose-first.
- Deterministic artifact rendering.
- Idempotent-oriented planning via desired vs snapshot diff.
- Explicit plugin/backend boundaries for multi-chain and multi-runtime evolution.
