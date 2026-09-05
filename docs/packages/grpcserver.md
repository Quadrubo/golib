# grpcserver

The `grpcserver` package serves gRPC. It provides the `*grpc.Server` a service
registers on, runs it, and drains it inside the app's shutdown budget.

## Usage

`grpcserver.Module` reads `modules.grpcserver`, which takes `addr`,
`grace_period`, `reflection`, `health` and `max_receive_bytes`. The interceptor
chain is given at construction, outermost first.

```go
grpcserver.Module(grpcserver.WithUnaryInterceptors(
    recovery.Unary,
    errmap.Unary,
    validate.Unary,
))
```

A service resolves the `*grpc.Server` from the injector and registers on it.
Registration happens while the module provides, since `Run` hands the server to
`Serve` and it takes no registration after that.

Setting `addr` to a port of `0` leaves the port to the operating system.
`grpcserver.Addr` is provided with the address actually bound, so a caller that
did so reads back what it got.

## Mechanics

`Provide` builds the server, chains the interceptors, registers the standard
health service when `health` is set, and binds the listener. `Run` optionally
registers reflection and serves on that listener until its context is
cancelled.

On cancellation the health service is put into `NOT_SERVING` first, so a load
balancer stops sending new calls, then `GracefulStop` runs. If it has not
finished within `grace_period`, `Stop` drops what is left.

gRPC's own logging is routed into `slog` under the component `grpc`. Its
severities are fixed, so they map onto slog levels rather than the configured
one. `grpcserver.GRPCLogger` builds that adapter for a caller that wants to
install it itself.

## Decisions

`grace_period` has to be less than the drain budget the app publishes, and the
module refuses to provide when it is not. That fails the boot rather than every
shutdown after it.

Equal does not fit either. A module that drains for exactly the budget returns
just as the deadline lands, and the app cannot tell that apart from one that
never returned at all.

The listener is bound in `Provide` rather than `Run`. A taken port then fails
the boot instead of surfacing from a goroutine seconds later, and the bound
address is known before anything serves, so nothing has to pick a free port in
advance and hope it stays free.

`Stop` closes that listener, which matters only when a later module fails to
provide and `Run` never reaches `Serve`, since `Serve` closes it itself.

Reflection is registered in `Run` rather than `Provide`, so a service that
registers its own services during provide is reflected too.

`grpclog.SetLoggerV2` is installed once per process. It writes package globals
that gRPC's own goroutines read without synchronising, so calling it again once
gRPC is running is a data race. A second app in one process therefore keeps the
first one's gRPC logger, which only a test process ever does.

## Failure modes

`max_receive_bytes` caps one received message, and the server answers a larger
one with `RESOURCE_EXHAUSTED` rather than a validation error. A client sending
legitimately large payloads needs the limit raised on both ends, since the
client has its own.
