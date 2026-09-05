# grpcinterceptor/errmap

The `errmap` package decides what a handler's error becomes on the wire, so a
handler returns its own error and never builds a status.

## Usage

`errmap.Unary` builds the interceptor from the injector. It sits behind
`recovery`, so a panic it cannot see is still caught, and ahead of anything
that returns a raw error of its own.

## Mechanics

The interceptor looks at the error a handler returned and does one of four
things.

A `grpcerr.Error`, wrapped or not, is stamped with the configured domain and
returned. A `context.Canceled` becomes `CALL_CANCELED` and a
`context.DeadlineExceeded` becomes `CALL_EXPIRED`. Anything else is logged at
error level and replaced with `INTERNAL`.

An error the handler chose is not logged, since it is not a surprise. A wrapper
around one is logged at debug level, because the wrapper is dropped and the log
is the only place its context survives.

## Decisions

The unwrapped `grpcerr.Error` is returned rather than the wrapper.
`status.FromError` rebuilds the message from the whole chain when it unwraps,
so returning the wrapper would put text like a pool address on the wire.

An unclassified error is replaced rather than passed on, which keeps a driver
message or a file path out of the response. The original still reaches the log.

`grpcerr.Internal` therefore stays out of service code. A handler returns its
own error and lets this interceptor decide.

## Failure modes

A bare status error built somewhere else is treated as unclassified and
replaced with `INTERNAL`, since it carries no `ErrorInfo` and AIP-193 requires
one. Anything wanting to reach the client intact is a `grpcerr.Error`.

A context error the handler wrapped in a `grpcerr.Error` keeps the handler's
code, because the `grpcerr.Error` case is checked first.
