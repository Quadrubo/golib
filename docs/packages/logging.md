# logging

The `logging` package provides the `*slog.Logger` every other module writes
through, and makes it the process default.

## Usage

`logging.Module` goes after `config` in the module list and reads
`modules.logging`, which takes a `level` and a `format` of `text` or `json`.

A module resolves the logger through `logging.Component` with its own name,
which tags every record with the part of the system that wrote it.

```go
m.log, err = logging.Component(i, m.Name())
```

`WithWriter` sends records somewhere other than `os.Stdout` and
`WithoutDefault` leaves the process default alone. Nothing in production sets
either. They exist so a spec can boot the real module rather than a stand-in
for it.

## Mechanics

`Provide` builds a `slog.TextHandler` or a `slog.JSONHandler` at the configured
level, provides the logger, and calls `slog.SetDefault` with it.

Records carry the component under the `component` key, which
`logging.ComponentKey` names for anything asserting on them.

## Decisions

Setting the process default is what stops a second, unconfigured logger
running alongside the first. Anything reaching for the package-level `slog`
API, including dependencies that never see the injector, otherwise keeps
writing to stderr at info and ignores the configured level and format.

That default is also the floor for failures too early to have a logger. A
`main` reports through `slog.Error`, which is the standard library handler on
stderr until `logging` replaces it and the configured one afterwards. Fatal
errors therefore land on stdout with everything else, on the grounds that the
most important record should not be the one the log shipper cannot parse.

Writing to the process default is process-wide state, so it is opt-out rather
than opt-in. A spec that boots a second app would otherwise change the logger
underneath the first.

## Failure modes

A `format` outside `text` and `json` fails the boot, since the `validate` tag
lists both. A `level` the standard library cannot parse fails the same way.

gRPC's own `grpclog` is not covered by any of this and keeps writing to stderr
until something routes it.
