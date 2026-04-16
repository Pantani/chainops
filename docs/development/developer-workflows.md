# Developer Workflows

This document is implementation-focused and aligned with current repository behavior.

## Local Setup

### Prerequisites

- Go `1.22+`
- Optional: Docker + Compose plugin for runtime flags with compose backend

### Bootstrap

```bash
go mod download
```

## Common Development Loops

### Validate + render + plan loop

```bash
go run ./cmd/bgorch validate -f examples/generic-single-compose.yaml
go run ./cmd/bgorch render   -f examples/generic-single-compose.yaml -o .bgorch/render
go run ./cmd/bgorch plan     -f examples/generic-single-compose.yaml
```

### Apply loop

```bash
# Dry-run first
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render --dry-run

# Real apply
go run ./cmd/bgorch apply -f examples/generic-single-compose.yaml -o .bgorch/render
```

### Runtime compose loop (optional)

```bash
go run ./cmd/bgorch apply  -f examples/generic-single-compose.yaml -o .bgorch/render --runtime-exec
go run ./cmd/bgorch status -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
go run ./cmd/bgorch doctor -f examples/generic-single-compose.yaml -o .bgorch/render --observe-runtime
```

## Running Specific Subsystems

### Plugin behavior only

Use plugin tests:

```bash
go test ./internal/chain/genericprocess ./internal/chain/cometbft -v
```

### Backend rendering only

```bash
go test ./internal/backend/compose ./internal/backend/sshsystemd -v
```

### Planner/state semantics only

```bash
go test ./internal/planner ./internal/state -v
```

### CLI end-to-end smoke/regression

```bash
go test ./test/integration ./test/regression -v
```

## Debugging Guide

### Snapshot and lock inspection

- state directory: `.bgorch/state`
- snapshot files: `<cluster>--<backend>.json`
- lock files: `<cluster>--<backend>.lock`

Inspect with:

```bash
ls -la .bgorch/state
cat .bgorch/state/<cluster>--<backend>.json
```

### Rendered artifact inspection

```bash
find .bgorch/render -type f | sort
```

### Runtime compose failures

When `--runtime-exec` or `--observe-runtime` fails on compose backend:

- ensure Docker daemon is running,
- ensure compose file exists in `--output-dir`,
- rerun without runtime flags to isolate local-state path issues.

## Testing Workflow

### Fast local checks

```bash
go test ./...
```

### Full verification pipeline

```bash
make verify
```

`make verify` runs:

1. gofmt check
2. build
3. tests
4. vet
5. race tests when platform/CGO support is available

## CI Expectations

Repository scripts expect deterministic outputs:

- sorted artifacts and service lists in backends/plugins,
- stable golden outputs for compose and ssh-systemd renderers,
- no unformatted Go files.

## Migrations / Seed Data

There are no database migrations or seed workflows in the current implementation.

## Release Process (Current)

No automated release pipeline is implemented in this repository.

Current release practice is manual:

1. run `make verify`,
2. build binary from `cmd/bgorch`,
3. publish artifacts/changelog through external workflow.
