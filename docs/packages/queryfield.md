# queryfield

The `queryfield` package declares a queryable column once, so ordering and
filtering share one definition of what a field name means.

## Usage

A resource declares each queryable field with the constructor for its kind,
giving the column and an accessor for the value a row carries.

```go
var titleField = queryfield.Text("title", func(b *models.Book) string { return b.Title })
```

The accessor's return type declares nullability. `func(b *Book) string` names a
NOT NULL column and `func(b *Book) *string` names one that holds NULL. There is
one constructor per kind rather than one per kind and nullability.

`Text`, `Time`, `Int`, `Float`, `Bool`, `Enum` and `ResourceName` are the
seven. `Enum` maps the name a filter writes to the word the column holds.
`ResourceName` takes a `resourcename.Pattern` and unpacks a name to the id it
ends in.

## Mechanics

A field carries two decoders, because the same column is spelled differently by
its two callers. `ParseCursor` reads a page token, which the server wrote, and
`ParseLiteral` reads a filter value, which a client wrote. A time cursor is
epoch microseconds where a time filter is RFC3339, and a `ResourceName` cursor
is the bare id where its filter is the whole name.

`Kind` is what a caller switches on for the one decision neither decoder
settles, which is whether a wildcard may match the column.

`Zero` is the literal `COALESCE` puts in place of NULL, which collapses the
NULL rows onto one value a row comparison still decides against.

## Decisions

Presence in a consumer's map is the permission. Ordering and filtering each
name their own subset of the declared fields, so there is no `Sortable` flag to
forget to check.

`Column` is repo-controlled SQL and never client input. A field name is never a
column either, so `name` sorts on `id` and client input reaches SQL only as a
bind argument.

`Bool` reads a filter value as the word alone, and `parseBool` takes only what
`FormatBool` writes. `strconv.ParseBool` would widen that to `1`, `t` and
`TRUE`, which would put three spellings in the API.

`Float` encodes a cursor as the shortest decimal that parses back to the same
bits, so a page boundary never skips or repeats a row.

## Failure modes

A `Field` declared without a constructor carries no column and no codecs.
`Declared` reports that, and the specs that compile a set of fields panic on
one rather than matching no column at request time.
