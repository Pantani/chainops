# Internal Package Map

This directory contains the BGorch implementation (not public API stability contracts).

## Package Responsibilities

- `api/v1alpha1`: spec types for `bgorch.io/v1alpha1`.
- `app`: command orchestration service used by CLI.
- `backend`: backend interfaces and runtime capability extensions.
- `backend/compose`: compose desired-state renderer + optional runtime exec/observe.
- `backend/sshsystemd`: host-mode systemd/env/layout artifact renderer.
- `chain`: plugin interfaces and capability model.
- `chain/genericprocess`: generic fallback plugin.
- `chain/cometbft`: cometbft-oriented plugin with generated config/app/genesis assets.
- `cli`: CLI parsing and command handlers.
- `doctor`: doctor report model and status checks abstraction.
- `domain`: shared domain DTOs (diagnostics, desired state, plans).
- `planner`: desired vs snapshot diff engine.
- `registry`: plugin/backend registries and default registration.
- `renderer`: safe artifact writing to output directory.
- `spec`: YAML loading, defaults, and node expansion.
- `state`: snapshot persistence and lock primitives.
- `validate`: core semantic validation rules.

## Command Pipeline Ownership

- CLI parses arguments and prints output.
- `app` orchestrates load/validate/build/plan/apply flows.
- plugins + backends build `domain.DesiredState`.
- planner + state compute convergence and persist local state.

## Extension Points

1. New plugin: implement `internal/chain.Plugin` and register in `internal/registry`.
2. New backend: implement `internal/backend.Backend`; optionally implement runtime interfaces.
3. New command behavior: extend `internal/app` first, then wire flags/output in `internal/cli`.

## Design Constraints

- keep chain semantics inside plugins,
- keep runtime semantics inside backends,
- keep planner/state deterministic and backend-agnostic,
- avoid direct filesystem/process side effects outside `renderer`, `state`, and runtime-capable backends.
