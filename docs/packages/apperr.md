# apperr

The `apperr` package supplies the errors an AIP API returns for the standard
request fields and the common resource states, built on `grpcerr`.

## Usage

A handler returns one of these rather than assembling a `grpcerr.Error` for a
case every resource has.

```go
return apperr.NotFound("BOOK_MISSING", names.BookType, name.String())
```

The field errors take the parse error another package produced.
`InvalidFilter`, `InvalidOrderBy`, `InvalidPageToken`, `InvalidUpdateMask`,
`InvalidParent` and `InvalidResourceName` each name their request field and
echo the input that failed.

The state errors take a reason, a resource type and a name. `NotFound`,
`AlreadyExists`, `Deleted`, `AlreadyDeleted`, `NotDeleted`, `EtagMismatch`,
`ConcurrentUpdate` and `ConcurrentDelete` cover the states a resource moves
through.

A field the AIP catalog does not know goes through `apperr.InvalidField` with
its own reason.

## Mechanics

Each field error carries a `BadRequest` naming the request field and the parse
error's own message as the description, so a client reads the violation rather
than the prose.

Each state error records the name in the metadata and on a `ResourceInfo`
carrying the resource type.

## Decisions

The service passes the reason and the resource type, so the wording stays
generic while the reason and the `ResourceInfo` name the exact case. The
messages therefore name no type, since the resource name already identifies its
collection.

`InvalidResourceName` takes the field rather than assuming `name`, because an
update names the resource under a path such as `book.name`.
