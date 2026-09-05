# migrate

The `migrate` package applies the goose migrations a consumer embeds, before
anything queries.

## Usage

`migrate.Module` takes the `fs.FS` holding the migrations, and goes between
`postgres` and anything that reads.

```go
migrate.Module(migrations.FS)
```

`modules.migrate` takes `lock_retry_interval` and `lock_retries`, which bound
how long a boot waits for a peer that is still migrating.

## Mechanics

`Provide` applies the migrations rather than deferring to `Run`, so a boot
either reaches a current schema or fails before anything queries it.

Migrations run under a goose session lock, so several replicas starting at once
apply them once. A boot that cannot take the lock retries
`lock_retries` times at `lock_retry_interval`.

The wait those two multiply out to is logged before the lock is taken, so a
timeout afterwards can be read against it.

## Decisions

Nothing closes the goose provider. `goose.Provider.Close` would close the pool
underneath `postgres`, which owns it. The module has no `Stop` at all, and a
spec asserts it does not implement `app.Stopper`.

Migrating at boot rather than from a separate command means a replica cannot
serve against a schema it has not applied. The cost is that every replica waits
on the lock during a rollout.

## Failure modes

goose runs each migration in a transaction unless the migration opts out. One
that opts out and then fails halfway leaves whatever it had already committed,
and the next boot retries from there.

`lock_retry_interval` is passed to goose in whole seconds, so a fractional
value is truncated.
