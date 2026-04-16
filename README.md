# BGorch - The Blockchain Gorchestrator

BGorch is a Go-first declarative orchestrator for blockchain node topologies.

It gives you one control-plane workflow (`validate -> render -> plan -> apply -> status -> doctor`) and decouples:

- chain-family logic (`internal/chain/*` plugins),
- runtime materialization (`internal/backend/*` backends),
- deterministic desired-state diffing (`internal/planner`, `internal/state`).

## Purpose

BGorch exists to manage multi-process blockchain nodes across different runtimes without hard-coding chain-specific semantics into the core.

Current focus:

- deterministic artifact generation,
- local-state-first reconciliation,
- explicit plugin/backend contracts,
- safe apply semantics with per-cluster/backend locking.

## Key Features

- Versioned API: `bgorch.io/v1alpha1`.
- Plugins:
  - `generic-process`
  - `cometbft-family`
- Backends:
  - `docker-compose` (plus alias `compose`)
  - `ssh-systemd` (plus alias `sshsystemd`)
- Commands:
  - `validate`, `render`, `plan`, `apply`, `status`, `doctor`
- Optional runtime operations for compose backend:
  - `apply --runtime-exec`
  - `status --observe-runtime`
  - `doctor --observe-runtime`
- Optional runtime operations for `ssh-systemd`:
  - `apply --runtime-exec`
  - `status --observe-runtime`
  - `doctor --observe-runtime`
- Snapshot-based plan/apply flow with deterministic hashing of services and artifacts.
- Locking on `apply` by `(cluster, backend)` under `.bgorch/state`.
- Typed plugin extension blocks:
  - `pluginConfig.genericProcess` implemented
  - `pluginConfig.cometBFT` implemented (see ADR 0006)

## High-Level Architecture

```text
Spec YAML (v1alpha1)
  -> spec.LoadFromFile + defaults
  -> validate.Cluster + plugin.Validate + backend.ValidateTarget
  -> plugin.Normalize + plugin.Build
  -> backend.BuildDesired
  -> DesiredState
     -> render (artifacts)
     -> plan (desired vs snapshot hashes)
     -> apply (lock -> render -> optional runtime exec -> snapshot save)
     -> status/doctor (desired vs snapshot + optional runtime observe)
```

Boundaries enforced in code:

- Core (`internal/app`) does orchestration and command semantics.
- Plugins (`internal/chain`) own chain-family validation/normalization/artifacts.
- Backends (`internal/backend`) own runtime translation/execution/observation.
- Planner/state are runtime-agnostic and only compare desired vs snapshot.

## Tech Stack

- Go `1.22`
- YAML parsing: `gopkg.in/yaml.v3`
- Standard library for CLI, filesystem, process execution, and hashing

## Quickstart

```bash
# Validate
go run ./cmd/bgorch validate -f examples/generic-single-compose.yaml

# Render desired artifacts
go run ./cmd/bgorch render -f examples/generic-single-compose.yaml -o .bgorch/render

# Plan against local snapshot
go run ./cmd/bgorch plan -f examples/generic-single-compose.yaml

# Apply (write artifacts + snapshot)
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render

# Apply with compose runtime execution (requires docker/compose)
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render --runtime-exec

# Observe status/doctor (local snapshot mode)
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml

# Observe runtime for compose backend
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
```

## CLI Commands

- `validate -f <spec> [--output text|json]`
- `render -f <spec> [-o <dir>] [--write-state]`
- `plan -f <spec> [--output text|json]`
- `apply -f <spec> [-o <dir>] [--dry-run] [--runtime-exec] [--output text|json]`
- `status -f <spec> [-o <dir>] [--observe-runtime] [--output text|json]`
- `doctor -f <spec> [-o <dir>] [--observe-runtime] [--output text|json]`

## Local Development

### Prerequisites

- Go `1.22+`
- `gofmt` (bundled with Go)
- Optional for compose runtime flags: Docker Engine + Compose plugin

### Setup

```bash
go mod download
```

### Build

```bash
go build ./cmd/bgorch
# or
make build
```

### Test

```bash
go test ./...
# or
make test
```

### Lint / Format / Verify

```bash
make format     # gofmt check via scripts/verify.sh
make vet
make verify     # format + build + test + vet + race (when supported)
```

## Environment Variables

BGorch does not require application-level environment variables to run.

Variables currently used by tooling only:

- `GO_BIN`: Go binary override in `scripts/verify.sh`.
- `PKGS`: package selector for verify targets.
- `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`: test isolation in CLI integration tests.

## Build and Deployment Notes

Current repository scope is local binary build + artifact rendering.

- Build artifact: `bgorch` CLI binary.
- Deployment pipelines (release automation, package distribution, remote rollout orchestration) are not implemented yet.
- `apply` mutates local rendered artifacts and local snapshot state; remote execution is optional and backend-gated via flag.

## Important Directories

- `cmd/bgorch/`: CLI entrypoint.
- `internal/api/v1alpha1/`: versioned spec types.
- `internal/app/`: application service orchestrating command flows.
- `internal/chain/`: plugin contracts and chain-family plugins.
- `internal/backend/`: backend contracts and runtime backends.
- `internal/spec/`: YAML loading, defaults, node expansion.
- `internal/validate/`: core semantic validation.
- `internal/planner/`: desired vs snapshot diff logic.
- `internal/state/`: snapshot store and lock primitives.
- `internal/renderer/`: artifact writer.
- `internal/doctor/`: doctor report model.
- `docs/`: ADRs, architecture, operations, schema, research.
- `examples/`: runnable sample specs.
- `test/`: integration/regression fixtures and golden outputs.

## Current Constraints and Caveats

- `status` and `doctor` are local-state-first by default; runtime observation is optional and backend-dependent.
- Runtime operations are preflight-gated per backend and require backend prerequisites (for example, compose binary, SSH reachability, rendered artifacts, runtime targets when required).
- `ssh-systemd` runtime operations run remote preflight/observation commands (`ssh` + `systemctl`) and are explicit opt-in flags.
- `cometbft-family` plugin supports typed `pluginConfig.cometBFT` on cluster/node/workload scopes.
- Locking is local filesystem based (single-machine safety, not distributed coordination).

## Documentation Map

- [Architecture maturity snapshot](docs/architecture/phase-2-3.md)
- [Request lifecycle and flows](docs/architecture/request-lifecycle.md)
- [Operations and command semantics](docs/operations/commands-and-integration.md)
- [Developer workflows](docs/development/developer-workflows.md)
- [API schema overview](docs/schema/v1alpha1.md)
- [ADRs](docs/adr)
