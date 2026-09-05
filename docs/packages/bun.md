# bun

The `bun` package wraps the pool `postgres` owns in a `*bun.DB`, so a store
writes queries through bun rather than raw SQL.

## Usage

`bun.Module` goes after `postgres` in the module list and needs nothing else.
A store resolves a `*bun.DB` from the injector.

## Mechanics

`Provide` resolves the `*sql.DB`, wraps it with the postgres dialect, and
provides the result. The two share one pool, so a store may use either.

## Decisions

The module has no `Stop`. `bun.DB.Close` would close the pool underneath
`postgres`, which owns it, and two owners closing one pool is how a close gets
lost. A spec asserts the module does not implement `app.Stopper`.

## Failure modes

The dialect is fixed to postgres. A service on another database needs its own
module rather than a setting here.
