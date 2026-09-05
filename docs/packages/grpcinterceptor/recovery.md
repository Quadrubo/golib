# grpcinterceptor/recovery

The `recovery` package turns a panic in a handler into a failed call instead of
a dead process.

## Usage

`recovery.Unary` builds the interceptor from the injector, so it resolves the
logger and the error domain the same way a module does. It leads the chain, so
a panic in any interceptor behind it still becomes a failed call.

## Mechanics

The interceptor defers a `recover`. On a panic it logs the method, the panic
value and the stack at error level, then returns
`grpcerr.Internal("PANIC", "internal error")` under the configured domain.

A handler that returns an error normally is left alone, so this only ever acts
on a panic.

## Decisions

The panic value never reaches the client. It goes to the log and the response
carries the fixed message `internal error`. A panic value is arbitrary and
routinely carries connection strings, tokens or row contents, and a spec pins
that none of it escapes.

## Failure modes

`recover` only catches a panic on the goroutine that deferred it. A handler
that starts a goroutine and panics inside it still takes the process down, so a
handler owns any goroutine it starts.
