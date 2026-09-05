# fieldmask

The `fieldmask` package resolves the update mask of
[AIP-134](https://google.aip.dev/134) and limits validation to what it selects.

## Usage

A resource declares the paths an update takes, then parses the mask a request
carried.

```go
mask, err := fieldmask.Parse(req.GetUpdateMask(), fieldmask.Paths{
    Writable:  []string{"title", "isbn"},
    Immutable: []string{"author"},
})
```

`Mask.Has` reports whether a path was selected. `fieldmask.Filter` turns the
mask into a `protovalidate.Filter`, so the rules the resource declares run
against the fields an update actually carries.

## Mechanics

An omitted mask, or one carrying no path, selects every writable path. A path
in neither the writable nor the immutable set is an error.

`Filter` matches on the name of a top-level field. It passes anything that is
not a field descriptor, so message-level rules still run.

## Decisions

`Paths` separates writable from immutable because selecting an immutable path
is not itself a violation. AIP-203 permits it where the value is unchanged, so
`Parse` admits the path and leaves the service to compare the value and answer
for a change.

## Failure modes

Filtering by field name means two messages nested under one request cannot be
told apart, since only the leaf name is matched. A mask covering a nested
resource selects the field of that name wherever it appears.
