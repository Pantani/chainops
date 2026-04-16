# ADR 0005: Optional Compose Runtime Execution and Observation

- Status: Accepted (implemented with current CLI shape)
- Date: 2026-04-16

## Context

BGorch keeps local-state-first behavior as default:

- `apply` renders artifacts and updates snapshot under lock,
- `status`/`doctor` evaluate desired vs local snapshot.

Operational teams still need optional runtime coupling to Docker Compose for some workflows.

## Decision

Keep runtime actions explicit and opt-in for compose backend:

- `apply --runtime-exec`
- `status --observe-runtime`
- `doctor --observe-runtime`

No implicit runtime mutations/observations happen without flags.

## Current Implementation

Implemented behavior in code:

- compose backend implements both runtime interfaces:
  - `RuntimeExecutor` -> `docker compose ... up -d`
  - `RuntimeObserver` -> `docker compose ... ps --all`
- apply flow persists snapshot only after successful runtime execution when `--runtime-exec` is enabled.
- status/doctor runtime observation failures are reported as warnings/non-fatal observations.

## Rationale

- preserves deterministic local-state workflows by default,
- avoids hidden runtime side effects,
- allows progressive operational adoption per environment.

## Consequences

### Positive

- predictable command semantics,
- safe fallback to local-state-first mode,
- backend capability checks are explicit at runtime.

### Negative

- runtime behavior is backend-dependent,
- users must manage output-dir and compose file lifecycle explicitly,
- no `--require-runtime` strict mode yet.

## Out of Scope

- remote runtime execution for `ssh-systemd`,
- distributed locking,
- protocol-aware health probes.

## Follow-ups

Potential extensions (not implemented):

- strict runtime requirement flag (`--require-runtime`),
- richer runtime status normalization across backends,
- structured runtime observation payloads beyond plain lines.
