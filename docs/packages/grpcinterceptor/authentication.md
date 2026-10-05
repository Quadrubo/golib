# grpcinterceptor/authentication

The `authentication` package hands the bearer credential of every call to the
consumer's `Authenticator` and refuses a call that carries none. It answers
who a caller is and never what a caller may do.

## Usage

A module placed before `grpcserver` provides an `authentication.Authenticator`
and an `authentication.Anonymous` into the injector. `authentication.Unary`
and `authentication.Stream` build the interceptors from them. Both sit behind
`errmap`, so a refusal carries the domain, and ahead of `validate`, so an
unknown caller learns nothing from the validation.

```go
do.ProvideValue[authentication.Authenticator](i, sessions)
do.ProvideValue(i, authentication.Anonymous{
    Services: []string{healthpb.Health_ServiceDesc.ServiceName},
    Methods:  []string{authenticationv1.AuthenticationService_SignIn_FullMethodName},
})
```

`Authenticate` takes the credential and returns the context the handler runs
under, which carries whatever the consumer calls a principal.

## Mechanics

A call whose full method is in `Methods`, or whose service is in `Services`,
runs unchanged, even with a credential. The service is the full method up to
its last slash, as grpc routes it. Every other call needs exactly one
`authorization` metadata value of the form `Bearer <credential>`, with the
scheme in any case (RFC 9110 section 11.1), one or more spaces and no
whitespace after the credential (RFC 6750 section 2.1). A call without one fails with `CREDENTIAL_MISSING`,
and a value of another shape fails with `CREDENTIAL_MALFORMED`, both
`UNAUTHENTICATED` and both before the `Authenticator` runs. The stream
interceptor hands the handler a stream whose `Context` returns the context of
the `Authenticator`. An `Authenticator` that returns neither a context nor an
error fails the call, which `errmap` turns into `INTERNAL`.

## Decisions

The default is closed. A new method needs a credential until a consumer lists
it, so a missing entry fails loudly rather than opening a method. Reflection
and health are services like any other, so they need a credential unless
`Services` lists them.

The anonymous calls are full names rather than a proto option. The generated
`_FullMethodName` constants and `ServiceDesc.ServiceName` give a consumer
names that fail to compile when they go stale, and the package needs to know
no option of a consumer's protos.

## Failure modes

A server that registers `Unary` without `Stream` checks no credential on its
streaming methods, which stay open to every caller.

An `Authenticator` that returns a context not derived from the one it got
drops the deadline, the cancellation and the metadata of the call. The
handler then runs on past the deadline of the caller.
