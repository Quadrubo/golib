# testkit/contract

The `testkit/contract` package registers the specs every AIP resource passes,
so a service states how to reach one resource and gets the standard method,
pagination, filter, etag, revision and view coverage from that.

## Usage

A spec file declares one `contract.Resource` per resource and hands it to
`contract.Describe`.

```go
var _ = contract.Describe(contract.Resource[*booksv1.Book]{
    ErrorDomain: "books.example.com",
    Methods: contract.Methods[*booksv1.Book]{
        Create: createBook,
        Get:    getBook,
        List:   listBooks,
    },
    Fixtures: contract.Fixtures[*booksv1.Book]{
        Full:    fullBook,
        Minimal: minimalBook,
    },
    Collection: contract.Collection{Filters: []string{"name", "title"}},
})
```

Every field is a closure over the service's own client, so the package knows
nothing about the transport. A nil closure skips the specs that need it, which
is how a read-only resource declares `Get` and `List` alone. `Parent` stays nil
for a top-level collection. `SoftDelete`, `Revisions`, `Batch` and `Views` stay
nil for a resource without them.

## Mechanics

`Describe` builds a harness from the resource and from the message descriptor
of `R`. The descriptor supplies the collection segment of the resource name,
the `name`, `etag`, `create_time`, `update_time` and `delete_time` fields, and
the AIP-203 field behaviors that sort the remaining fields into creatable,
writable, required, optional, immutable and output only.

The specs walk those sets rather than naming any field, so a resource that
gains a field gains the coverage of it in the same run. A field the specs
cannot build a value for, being a list, a map or a message other than a
timestamp or a duration, is left out of the walk. A message of another type
carries its own fields and rules, which a built value breaks.

`Fixtures` runs first and asserts what the later specs rely on, that `Full`
sets every creatable field, that `Minimal` sets the required ones alone, and
that the two differ in every field a create takes.

## Decisions

Everything but the resource declaration and `Describe` is unexported. The specs
are the contract, and a consumer that could register them one at a time could
register a subset and still report a passing contract.

The package is flat. Splitting the harness away from the specs keeps twenty
files in two named groups, but it forces the two to talk through exported
names, and around forty identifiers would then carry an exported name for no
reason beyond the split.

## Failure modes

A message without a `google.api.resource` annotation carries no pattern, so the
collection segment comes out empty and every name the specs build is malformed.
