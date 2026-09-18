# resourcename

The `resourcename` package parses and composes the resource names of
[AIP-122](https://google.aip.dev/122), where a name alternates a collection and
the id of one resource in it.

## Usage

A resource compiles its pattern once, into a package var.

```go
var BookPattern = resourcename.MustCompile("shelves/*/books/*")
```

`Parse` returns the ids in the order the pattern names their collections, and
`Format` composes a name from them.

`Singleton` compiles the pattern of a resource with one instance, which
[AIP-156](https://google.aip.dev/156) defines and the alternating shape has no
wildcard for. `serverConfig` and `users/*/settings` are both singletons, and
the first parses to no ids at all.

`FillUUIDv7` gives a resource its id on create. It keeps the id a client chose
and sets a UUID v7 on an empty one, which sorts by creation time.

```go
if err := resourcename.FillUUIDv7(&book.ID); err != nil {
	return nil, err
}
```

## Mechanics

An id segment matches `[a-z0-9-]{1,63}`. That is the format a service states in
the rule its protos validate names with, so the two have to agree.

`Error` carries the name it was given and the pattern it wanted, and reports a
name it was given none of as required rather than as malformed.

## Decisions

`MustCompile` and `Format` panic rather than returning an error. Both fail only
on a pattern or an id count spelled wrong in the source, which every request
would repeat, so a spec finds it rather than a client.

`Parse` returns the ids rather than a bool, because the id is what a store keys
on while the name is what the API carries. A handler parses once and holds ids
from there.

## Failure modes

A rule admitting an id the pattern rejects lets a request past validation and
fails it on the parse, which answers with the pattern rather than the field
violation the rule would have named.
