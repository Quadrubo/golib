# testkit/postgres

The `testkit/postgres` package runs a throwaway postgres container for a suite
and points the service at it.

## Usage

`postgres.New` returns a `testkit.Dependency`, which a suite declares in one
line.

```go
Dependencies: []testkit.Dependency{tkpostgres.New()},
```

It defaults to `postgres:17-alpine` with a database named `test`. `WithImage`
names another image, which is how a service needing an extension gets one.
`WithDatabase` names the database.

`Start` puts the suite's own `*bun.DB` into the suite injector, so a spec seeds
rows without going through an RPC. `URL` reaches the same database for a spec
that opens its own connection, and `CreateDatabase` adds a further one.

## Mechanics

`Start` runs the container, reads its connection string, opens the suite
connection and provides it. `Settings` hands the service the same url
under `modules.postgres.url`, so both talk to one database.

`Stop` closes the suite connection and terminates the container.

## Decisions

The container creates one database at startup. `CreateDatabase` creates further
ones on the container already running, which is what lets a spec that mutates
schema start from nothing without paying for a second container.

`CREATE DATABASE` takes no bind parameter, so the name is quoted as an
identifier rather than concatenated into the statement.

## Failure modes

The suite connection and the service's own pool are two connections to one
database. A spec that seeds through the first and reads through the second sees
its own writes, but nothing isolates them, so a spec scopes its rows rather
than assuming an empty table.
