# `internal/app`

`app.App` is the application service used by CLI commands.

## Responsibility

- orchestrate command pipelines (`validate`, `render`, `plan`, `apply`, `status`, `doctor`),
- centralize plugin/backend resolution,
- coordinate planner/state/renderer interactions,
- enforce command semantics (dry-run, runtime flags, lock handling).

## Key Entrypoints

- `New(opts Options) *App`
- `ValidateSpec(path)`
- `Render(ctx, specPath, outputDir, writeState)`
- `Plan(ctx, specPath)`
- `Apply(ctx, specPath, opts)`
- `Status(ctx, specPath, opts)`
- `Doctor(ctx, specPath, opts)`

## Data Flow

1. load + default spec
2. resolve plugin/backend
3. validate + normalize
4. plugin build -> backend build desired
5. planner/state/render/runtime operations depending on command

## Important Operational Semantics

- diagnostics are returned separately from hard errors,
- `apply` always acquires lock before plan/write,
- snapshot persists only after successful apply pipeline,
- runtime observation errors in `status`/`doctor` are non-fatal.
