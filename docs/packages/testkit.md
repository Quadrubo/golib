# testkit

The `testkit` package boots a whole service in the test process and hands specs
a real gRPC connection to it. It is public API for a consumer's suite rather
than something golib uses internally.

## Usage

`testkit.Boot` starts the dependencies, runs the service, and waits for its
health service to answer. A suite calls it once and stops it on cleanup.

```go
suite, err := testkit.Boot(ctx, testkit.Options{
    Modules:      wiring.Modules,
    Dependencies: []testkit.Dependency{ /* a container the service needs */ },
    Settings: map[string]any{
        "modules": map[string]any{"grpcerr": map[string]any{"domain": "svc.test"}},
    },
})
```

`Modules` has the shape of the service's own module list constructor, so the
suite runs the real wiring. `Settings` has the shape of the config file and
overrides whatever `Boot` and the dependencies set.

`Suite.Conn` is a connection to the running service, `Suite.Injector` holds
what the dependencies provided, and `Suite.Rebooted` runs a second service on
the same dependencies under overridden settings.

`testkit.ResourceID` returns an id under a prefix that no other spec carries.

## Mechanics

A `Dependency` starts before the service. One that also implements
`SettingsProvider` hands the service its settings, such as a container's url.

Settings layer in one order. The address comes first, then each dependency's
`Settings`, then `Options.Settings`, then the overrides a reboot passes.

The service listens on port 0 and `Boot` reads the assigned address back off
the injector, so parallel suites never pick the same port.

Specs reach the service over a real socket, so serialization, interceptors and
error mapping are all in the path. `main` and config file discovery are not,
which is the gap this trades away for the build it does not have to run.

## Decisions

The suite's injector holds what the dependencies provided and never the
service's own. Seeding a row through the database connection a dependency
provided keeps a spec independent of whichever RPC would otherwise have to
create it, while the service stays a black box.

`Settings` is nested maps shaped like the config file rather than a typed
struct or dotted keys. A typed struct would expose the service's internal
config objects, and the suite should read like a real client plus a config
file.

## Failure modes

The specs of a suite share its database and nothing resets it between them.
A spec creates its own resources under `testkit.ResourceID` and scopes its
lists to them rather than cleaning up.
