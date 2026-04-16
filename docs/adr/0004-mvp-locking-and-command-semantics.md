# ADR 0004: Locking and `apply`/`status`/`doctor` Semantics

- Status: Accepted
- Date: 2026-04-16

## Context

`bgorch` executes mutable operations from CLI against a local snapshot state model.

Without explicit locking and command contracts, concurrent apply operations may corrupt local state and produce unreliable diagnostics.

## Decision

### 1. Lock scope

- Lock key: `(clusterName, backend)`.
- Lock path: `.bgorch/state/<cluster>--<backend>.lock`.
- `apply` acquires exclusive lock.
- `status` and `doctor` remain read-only and lock-free.

### 2. `apply` semantics

Implemented pipeline:

1. load + validate + normalize,
2. build desired,
3. acquire lock,
4. load snapshot + build plan,
5. optional dry-run early return,
6. write artifacts,
7. optional runtime execution (backend-dependent),
8. save snapshot,
9. release lock.

Failure semantics:

- if lock acquisition fails, apply aborts;
- if runtime execution fails, snapshot is not persisted;
- lock release is attempted in deferred cleanup.

### 3. `status` semantics

- validates and builds desired state;
- compares desired vs snapshot;
- emits observations;
- optional runtime observation can enrich output when backend supports it.

### 4. `doctor` semantics

- runs actionable checks for spec, registry resolution, state accessibility, snapshot, and drift;
- optional runtime observation contributes pass/warn checks;
- runtime observation failures are warnings, not hard failures.

## Rationale

- low-complexity operational safety for local workflows,
- deterministic plan/apply behavior,
- explicit command expectations for operators.

## Consequences

### Positive

- protects against local concurrent applies,
- avoids snapshot persistence on failed runtime execution,
- keeps diagnostics available during degraded runtime states.

### Negative

- no multi-host/distributed locking,
- lock model is filesystem dependent,
- runtime parity differs by backend capability.

## Out of Scope

- distributed lock backend,
- transactional cross-backend apply,
- asynchronous reconciliation worker.
