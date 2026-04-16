# ADR 0003: Backend Model (Runtime Execution Adapters)

- Status: Accepted
- Date: 2026-04-16

## Context

`bgorch` must operate in heterogeneous environments: local containers, VMs/bare metal, Kubernetes, and future Terraform/Ansible integrations.

## Decision

Define backend as a runtime adapter with a minimum contract:

- `ValidateTarget(spec)`
- `BuildDesired(ctx, spec, pluginOut)`

Optional runtime contracts:

- `RuntimeExecutor` (`ExecuteRuntime`)
- `RuntimeObserver` (`ObserveRuntime`)

This keeps runtime-specific operations optional and backend-owned.

## Implementation Status (current repository)

Implemented backends:

- `docker-compose`
- `ssh-systemd`

Compose backend additionally implements optional runtime execution/observation.

Planned backends/adapters:

- `kubernetes`
- `terraform` (infra adapter)
- `ansible` (bootstrap/config adapter)

## Rationale

- core computes desired state and plan;
- backend translates desired state to runtime artifacts/actions;
- runtime details stay isolated from chain-domain logic.

## Consequences

### Positive

- add new runtime with localized code changes,
- explicit capability gaps per backend,
- easier backend-focused testing (golden + integration).

### Negative

- feature parity differs across backends,
- requires clear compatibility matrix and docs.

## Boundaries

- Terraform adapter: infra provisioning, not process control-plane.
- Ansible adapter: host bootstrap/configuration, not orchestration core.

## Rejected Alternatives

1. Backend monoculture (Kubernetes-only or Docker-only)
   - rejected: conflicts with multi-runtime goals.
2. Embedding backend logic into core/planner
   - rejected: violates separation of concerns and reduces maintainability.
