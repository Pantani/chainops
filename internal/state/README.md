# `internal/state`

State package provides local snapshot persistence and apply locking.

## Snapshot Model

`Snapshot` stores hashes for:

- desired services (`hash(json(service))`)
- desired artifacts (`hash(content)`)

Used by planner to compute create/update/delete/noop diff.

## Lock Model

- lock key is `(cluster, backend)`
- lock file: `.bgorch/state/<cluster>--<backend>.lock`
- acquisition is atomic via `O_CREATE|O_EXCL`
- lock release is idempotent

## Scope

This model protects local concurrent applies on a single machine.
It does not provide distributed lock/state coordination.
