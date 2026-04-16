# ADR 0002: Plugin Model (Chain Family Extensions)

- Status: Accepted
- Date: 2026-04-16

## Context

The core must support multiple blockchain families, including unknown ones, without leaking family-specific details into the common schema.

## Decision

Define plugin interface with explicit responsibilities:

- `Validate(spec)`
- `Normalize(spec)`
- `Build(ctx, spec)` for family-specific artifacts/metadata
- `Capabilities()` for operational capability signaling

Schema separation:

1. Portable common layer: runtime, nodePools, workloads, volumes, files, health checks, generic policies.
2. Typed extension layer: `pluginConfig.<pluginName>`.

## Implementation Status (current repository)

Implemented plugins:

- `generic-process`
- `cometbft-family`

## Rationale

- minimizes core coupling to chain-family behavior,
- enables incremental plugin evolution with localized changes,
- reuses planner/backends across heterogeneous chain families.

## Consequences

### Positive

- extensibility with clear ownership,
- specialized validation per family,
- deterministic family-level artifact generation.

### Negative

- plugin contracts must remain versioned and stable,
- plugin-core compatibility requires dedicated tests.

## Rejected Alternatives

1. Family-specific fields in root schema
   - rejected: harms portability and multiplies global churn.
2. Untyped `map[string]any` plugin extensions
   - rejected: weak validation guarantees and poor tooling ergonomics.
