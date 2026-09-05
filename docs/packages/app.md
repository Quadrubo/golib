# app

The `app` package runs a service as an ordered list of modules. It provides
them, runs them, and takes them down again inside a bounded shutdown.

## Usage

A component implements `app.Module`, which asks only for a name, and then
whichever of the three lifecycle interfaces it actually needs.

`Provider` registers what the module offers and resolves what it needs.
`Runner` blocks for as long as the component is live. `Stopper` releases
whatever `Provide` acquired.

The name has to be unique. It selects the settings the module reads under
`modules.<name>`, tags its log records, and identifies the module in provide,
run and stop errors. `New` refuses a list in which two modules share a name.

`New` takes the modules in the order they should provide. `Run` blocks until
the app comes down and returns whatever went wrong on the way.

When `Provide` fails partway through, the module releases whatever it already
acquired before it returns the error. The app tears down the modules that
provided before it, and never the module that failed, because that module is
only half built.

The two budgets that bound shutdown are set with `WithDrainTimeout` and
`WithStopTimeout`, and `WithShutdownTimeout` is the ceiling they have to fit
inside.

## Mechanics

The app provides modules in the order they are listed. When a module fails to
provide, the app stops the modules it has already provided and returns the
error.

The first `Run` to return brings the whole app down, whether it returned an
error or not. Shutting down then happens in two phases, each with a budget of
its own. `WithDrainTimeout` bounds how long the app waits for running modules
to return after their context is cancelled, and defaults to 7 seconds. Any
module that has not returned by then is abandoned. `WithStopTimeout` bounds
releasing what the modules hold, and defaults to 1 second.

Modules are stopped in reverse order, so a module is always released before
whatever it was built on.

A `Stopper` reads its budget from the context that `Stop` is given. A module
that drains inside `Run` has no such context and needs the number before it
provides, so the app also publishes the drain budget in the injector as an
`app.DrainBudget`.

## Decisions

The drain and stop budgets are separate rather than one budget shared between
them. The drain deadline only ever fires for a module that ignored its context,
and such a module is abandoned however long it was given. If its overrun were
charged against the stop budget, a single hung module could stop every other
module from releasing what it holds. Stop therefore always gets its full
second.

`WithShutdownTimeout` bounds nothing on its own and defaults to 8 seconds. The
number comes from outside the process, because it stands for the grace period
the supervisor allows before it kills the container, which for `docker stop`
defaults to 10 seconds. It exists to catch the mistake of raising the drain or
stop budget without raising the limit the platform actually enforces, so `New`
refuses to boot when the two do not fit inside it.

`do` is used for dependency injection only. Nothing relies on `do.Shutdowner`,
which leaves exactly one shutdown ordering to reason about.

## Failure modes

The lifecycle interfaces are optional and matched by type assertion, so a typo
in a method signature quietly opts the module out. A module asserts the
interfaces it means to implement:

```go
var _ app.Stopper = (*module)(nil)
```
