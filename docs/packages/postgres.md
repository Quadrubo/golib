# postgres

The `postgres` package owns the connection pool, and carries the predicates a
store answers a driver error with.

## Usage

`postgres.Module` reads `modules.postgres` and provides a `*sql.DB`. The only
required setting is `url`. The pool limits and the connect timeout all have
defaults.

A store classifies a driver error through the predicates rather than by
matching strings.

```go
if postgres.IsUniqueViolationOn(err, "books_pkey") {
    return grpcerr.AlreadyExists("BOOK_EXISTS", "the book {name} already exists",
        grpcerr.Arg("name", name))
}
```

`IsNoRows`, `IsUniqueViolation`, `IsUniqueViolationOn` and
`IsForeignKeyViolation` are the four. A constraint name never leaves the
package writing the SQL.

## Mechanics

`Provide` opens the pool, applies the limits, and pings once under
`connect_timeout`. A database it cannot reach fails the boot rather than the
first query. `Stop` closes the pool.

The url carries a password, so nothing reports it as given. `redact` runs it
through `url.Redacted` first, and a DSN in keyword form is reported as nothing
at all rather than risk printing it raw.

## Decisions

The ping happens at boot so an unreachable database is a boot failure. Without
it the process would start, report healthy, and fail on the first request
instead.

`IsUniqueViolationOn` takes the constraint name because a store that answers
for a duplicate id should not also answer for whatever index the table gains
later. `IsUniqueViolation` is the broad form for when that distinction does not
matter.

## Failure modes

The pool is opened lazily by `database/sql`, so the limits take effect but
nothing connects until the ping. A url that parses but points nowhere is caught
by the ping, and one that does not parse is reported without its password,
which means without any detail at all.
