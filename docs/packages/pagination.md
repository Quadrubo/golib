# pagination

The `pagination` package pages a collection by keyset under a client
`order_by`, following [AIP-132](https://google.aip.dev/132) and
[AIP-158](https://google.aip.dev/158).

What `order_by` accepts and what a `page_token` promises is the consumer's to
document. This page records the mechanics.

## Usage

A resource compiles its spec into a package var, naming the fields `order_by`
may sort by and the column every order closes on.

```go
var booksPaging = pagination.MustCompile(pagination.Spec[*models.Book]{
    Fields:          map[string]queryfield.Field[*models.Book]{"title": titleField},
    Tiebreak:        idField,
    DefaultPageSize: 20,
    MaxPageSize:     100,
})
```

A list handler parses the `order_by`, builds a page from the size, the token
and whatever else narrows the collection, then feeds the query.

`Page.Order` returns the ORDER BY terms, `Page.Keyset` the predicate and its
binds, and `Page.Limit` the row count to ask for. `Page.Result` drops the
over-fetched row and returns the next token.

A bun consumer feeds `Page.Order` through `OrderExpr` rather than `Order`,
since a nullable term is an expression and `Order` would quote it as an
identifier.

## Mechanics

Every order closes on the tiebreak, so two rows sharing a sort value still have
an order and a page boundary falls between them. An omitted `order_by` sorts on
that column alone.

`Keyset` emits one shape, a row-value comparison over every sort column, such
as `(title, id) < (?, ?)`. It is the only shape postgres turns into an index
condition, so it is the only shape emitted.

`Limit` over-fetches one row, which is how `Result` tells a further page from
the end of the collection.

The token is base64url of a JSON object stamped with a version. It carries the
position, the `order_by` it was issued under, and the scope the caller passed.
A call repeating it under a different order or scope is `INVALID_ARGUMENT`.

## Decisions

Keyset over offset. Measured on a million rows sorted by an unindexed column at
page 25,000, offset scans the table and then sorts 500,020 rows through a
10.5MB external merge that spills to disk, while keyset scans the same table
and top-N sorts 20 rows in 26kB. Offset pays the identical scan and adds a sort
that grows with depth.

The tiebreak inherits the direction of the column before it rather than always
ascending. A btree scanned either way yields `ASC, ASC` or `DESC, DESC`, never
a mix, so an ascending tiebreak would make every descending sort mixed.

`ParseOrderBy` rejects an `order_by` running more than one direction. No index
produces `title ASC, create_time DESC`, and the shape that expresses it reads
as a filter rather than an index range. Measured on 10M rows at a deep page, it
scanned 3,320,001 rows through 71,542 buffers against 20 rows and 24 buffers
for the tuple.

`page_size` is deliberately not pinned by the token. It only ever reaches
`LIMIT`, so resizing mid-walk changes how much the next call returns and
nothing about where it resumes.

AIP-158 asks for more than base64, saying a page token must not be
user-parseable. Anyone can decode ours. The version stamp buys what obfuscation
would, since a server rejects a shape it does not know rather than decoding an
old token under new field meanings, and the cursor holds values the client just
received. Signing would close the gap the AIP names and costs a key to rotate,
which is the change to make once a token carries something the response does
not.

## Nullable columns

A NULL compares unknown against every value, so a tuple holding one decides
nothing. Such a column contributes two terms instead of one.

```sql
(colour IS NULL), COALESCE(colour, '')
```

The flag separates the NULL rows and places them, since `false` sorts before
`true`, so NULLs land last ascending and first descending. `COALESCE` holds
every NULL row at one value a cursor still compares decisively against.

The index such a column wants is
`((colour IS NULL), COALESCE(colour, <zero>), id)`. The zero is rendered into
the SQL rather than bound, so an expression index over it matches.

The tiebreak may not be nullable, and `MustCompile` panics on one.

## Indexes

An unindexed sort column reads the whole table on every page. Measured on a
million rows, the first page of twenty by an unindexed column is a parallel
sequential scan of all 1,000,000 rows and 6,484 buffers, against an index-only
scan of 20 rows and 23 buffers with a `(column, id)` index.

An `order_by` naming several columns needs an index on that exact sequence.
Every order closes on the tiebreak, so an order on `a` wants `(a, id)`, and an
index on `(a, b, id)` sorts the group it lands in rather than reading straight
down it.

One index per column costs roughly 6x on writes, measured at 20,000 inserts
going from 56ms to 371ms across six indexes, and several times the table in
disk. Narrowing `Spec.Fields` is the first lever, since `order_by` is an API
contract rather than a column list.

## Failure modes

A walk sees the collection as it is at each call, not as it was at the first.
Sorting on a column writes reach, such as `update_time`, means a row updated
mid-walk repeats if it moves behind the cursor and is skipped if it moves
ahead. This is inherent to keyset pagination on a mutable sort key. A client
that needs a stable set sorts on a column nothing rewrites.
