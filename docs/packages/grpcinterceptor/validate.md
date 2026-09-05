# grpcinterceptor/validate

The `validate` package runs the protovalidate rules a request message declares,
so a handler only ever sees a request that already holds.

## Usage

`validate.Module` provides the `protovalidate.Validator`, and `validate.Unary`
builds the interceptor from it. The module goes in the module list and the
interceptor goes last in the chain, behind `errmap`.

`validate.Masked` is for an Update handler, which the interceptor cannot serve.
It takes the validator, the resource, the parsed mask and the request field the
resource nests under.

```go
err := validate.Masked(validator, req.GetBook(), mask, "book")
```

## Mechanics

The interceptor validates anything that is a `proto.Message` and passes
anything else straight through. A violation becomes an `INVALID_ARGUMENT`
carrying one `BadRequest` field violation per broken rule, under the configured
domain.

`Masked` filters the validator by the mask, then rewrites each violation path
to `<field>.<path>` so it matches what the client sent.

## Decisions

One validator is shared by the interceptor and every handler, because
protovalidate compiles the rules of a message once and reuses them.

An Update carries the resource under `IGNORE_ALWAYS`, which stops the
interceptor from validating it at all. That is deliberate, since a partial
update legitimately omits required fields, and it is why `Masked` exists for
the handler to call once it knows the mask.

A rule that fails to compile is the server's fault rather than the caller's, so
the raw error goes back rather than an `INVALID_ARGUMENT`. `errmap` logs it and
replaces it with `INTERNAL`.

## Failure modes

The message of an `INVALID_ARGUMENT` is fixed. Every field detail is in the
`BadRequest`, so a client reads the violations rather than parsing prose.
