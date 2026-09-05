# etag

The `etag` package derives the resource etag of
[AIP-154](https://google.aip.dev/154) from the time a row was last written.

## Usage

`etag.Encode` takes the row's update time and returns the etag to put on the
response. A guarded write compares the etag a client sent against the same
call on the row it read.

```go
if tag != etag.Encode(current.UpdateTime) {
    return grpcerr.Aborted("ETAG_MISMATCH", "the book {name} carries a newer etag",
        grpcerr.Arg("name", name))
}
```

## Mechanics

The etag is base64url of the eight big-endian bytes of the update time in epoch
microseconds, quoted. It is strong rather than weak, since it changes with
every write.

Microseconds are the resolution `timestamptz` stores, so an etag never claims
precision the column cannot return.

## Decisions

The etag is derived rather than stored. A column would cost a second write on
every update, and the guard a concurrent write already runs,
`WHERE update_time = ?`, is the same comparison.

## Failure modes

A model that does not read `update_time` back returns the zero time, which
encodes to one etag for every row. Every guarded write then matches, and the
response looks well formed with the concurrency check silently off.

bun has no tag meaning write never and read always. `scanonly` drops the field
from the one column list bun builds off the model, and that list is the SELECT
list as much as the INSERT one, so a read needs `ColumnExpr("*")` or a mutation
needs `Returning("*")` to fill the field.
