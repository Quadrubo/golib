# grpcerr

The `grpcerr` package builds the errors [AIP-193](https://google.aip.dev/193)
describes. Every one carries a code, a message and an `ErrorInfo`.

Which error a given call returns is the consumer's to decide and document. This
page records what the package guarantees.

## Usage

A constructor takes a reason, a message template and the args the template
names.

```go
grpcerr.NotFound("BOOK_MISSING", "the book {name} does not exist",
    grpcerr.Arg("name", name))
```

`Arg` fills `{name}` with the quoted value and records the pair in the
`ErrorInfo` metadata, so a client reads the value without parsing the message.
`Metadata` adds a pair the message does not name.

The `With` methods attach the details AIP-193 defines. `ForResource` names the
resource, `WithFieldViolations` carries a `BadRequest`, and
`WithPreconditionViolations` carries a `PreconditionFailure`. Each copies rather
than writes, so an error declared at package scope is safe to derive from.

`grpcerr.Module` reads `modules.grpcerr.domain` and provides the `Domain` that
whatever renders an error stamps onto it.

## Mechanics

The message parameter is an unexported string type, so only an untyped string
constant converts to it. A runtime value therefore reaches a message only
through `Arg`, which is what keeps the metadata complete.

`build` panics on an arg the message does not name, on a `{key}` no arg fills,
and on a reason outside the UPPER_SNAKE format. These are declaration errors
that every request would repeat, so a spec finds them rather than a client.

`GRPCStatus` attaches one detail at a time, because `WithDetails` marshals
everything it is given as a unit and drops the lot when one payload fails.

`InvalidArgumentFrom` is the one entry for a message another parser produced,
since a constructor takes only a constant. Its signature demands the metadata
pair for the input the message quotes.

## Decisions

`codes.OK` has no constructor, since an error reporting success is a
contradiction. `codes.Unknown` has none either, because an error nothing
classified is reported as `Internal`.

The domain comes from config alone. A self-hosted deployment answers on a host
no build can know, so an unset domain fails the boot rather than producing
errors that carry an empty one.

An error declared at package scope runs before any injector exists and cannot
resolve the domain itself, which is why `WithDomain` derives a copy and the
`With` methods never write in place.

Each detail type may appear at most once, which is why an `Error` holds one
field per type rather than a list.

## Failure modes

The message is developer-facing English for a technical caller. It is not a UI
string, clients must not parse it, and nothing internal belongs in it.

A reason is permanent API surface. Renaming one breaks any client branching on
it, so name it with care the first time.
