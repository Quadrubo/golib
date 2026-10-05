# grpcinterceptor/errmap

The `errmap` package decides what a handler's error becomes on the wire, so a
handler returns its own error and never builds a status.

## Usage

`errmap.Unary` and `errmap.Stream` build the interceptors from the injector.
Each sits behind `recovery`, so a panic it cannot see is still caught, and
ahead of anything that returns a raw error of its own.

## Mechanics

The interceptor looks at the error a handler returned and does one of five
things.

A `grpcerr.Error`, wrapped or not, is stamped with the configured domain and
returned. Once the context of the call has ended, a `context.Canceled`, a
`context.DeadlineExceeded`, a `sql.ErrTxDone` or a bare status of either
context code becomes `CALL_EXPIRED` when the deadline of the call passed and
`CALL_CANCELED` otherwise. A bare status of `grpc.health.v1.Health`,
`grpc.reflection.v1.ServerReflection` or
`grpc.reflection.v1alpha.ServerReflection`, such as the `NOT_FOUND` of a health
check for an unknown service, passes unchanged. Anything else is logged at
error level and replaced with `INTERNAL`.

An error the handler chose is not logged, since it is not a surprise. A wrapper
around one is logged at debug level, because the wrapper is dropped and the log
is the only place its context survives.

## Decisions

The unwrapped `grpcerr.Error` is returned rather than the wrapper.
`status.FromError` rebuilds the message from the whole chain when it unwraps,
so returning the wrapper would put text like a pool address on the wire.

The two bare statuses count as context errors because the stream handlers of
grpc, such as health `Watch`, answer a closed stream with them. Without that
rule every closed stream would log an unhandled error.

Health and reflection follow their own protocols, whose clients read bare
status codes. A health probe for an unknown service expects `NOT_FOUND`, so
those statuses pass without an `ErrorInfo`. The services are named exactly,
so a consumer service in a package such as `grpc.gateway` still gets its
`ErrorInfo`.

`database/sql` answers a statement on a transaction that a cancelled context
already rolled back with `sql.ErrTxDone`. After a client cancel that error is
no failure of the server, so it counts as a context error.

A context error counts only while the context of the call has ended. The
deadline of an outbound request, such as a database query or a call to
another service, can time out while the call itself goes on. Its error is a
failure of the server, so it is logged and becomes `INTERNAL` rather than
`CALL_EXPIRED`, which names the deadline of the caller as the cause.

The code comes from the context of the call rather than from the error. A
library can answer an expired context with `context.Canceled`, and the
caller then still reads that its deadline passed.

An unclassified error is replaced rather than passed on, which keeps a driver
message or a file path out of the response. The original still reaches the log.

`grpcerr.Internal` therefore stays out of service code. A handler returns its
own error and lets this interceptor decide.

## Failure modes

A bare status error built somewhere else is treated as unclassified and
replaced with `INTERNAL`, since it carries no `ErrorInfo` and AIP-193 requires
one. A bare `Canceled` or `DeadlineExceeded` status becomes `CALL_CANCELED` or
`CALL_EXPIRED` only once the context of the call has ended. Before that it is
unclassified too. A bare status of health or reflection passes unchanged.
Anything wanting to reach the client intact is a `grpcerr.Error`.

A context error the handler wrapped in a `grpcerr.Error` keeps the handler's
code, because the `grpcerr.Error` case is checked first.
