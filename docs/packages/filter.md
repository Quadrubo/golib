# filter

The `filter` package compiles an [AIP-160](https://google.aip.dev/160) filter
into a SQL predicate. The
[EBNF](https://google.aip.dev/assets/misc/ebnf-filtering.txt) that AIP links is
the grammar, and the prose around it adds what the grammar does not carry.

This page records where we depart from that and why. Anything the EBNF settles
is not restated here.

## Usage

A resource compiles its spec into a package var, naming the fields a filter may
match on.

```go
var booksFilter = filter.MustCompile(filter.Spec[*models.Book]{
    Fields: map[string]queryfield.Field[*models.Book]{"title": titleField},
})
```

`Parse` returns a `Predicate` carrying the SQL and its binds. `Predicate.Empty`
reports the filter that matched every row, which is what an absent filter
compiles to.

`Spec.Functions` maps a call name to a `FunctionCompiler`, which is how a
consumer serves a question no column comparison asks, such as a geometry
intersection.

`Spec.LeadingWildcard` admits a `*` opening a value. A collection large enough
to feel a full scan leaves it unset.

## Mechanics

`ParseExpression` reads the whole EBNF and returns a tree. The compiler then
refuses the productions this package does not serve, so an unsupported filter
is rejected by name rather than by a syntax error.

Bind arguments are placed with `?`, and every chain of more than one part is
parenthesised, so a caller joining the predicate with `AND` binds all of it.

A field name is never a column. `name` matches on `id`, and client input
reaches SQL only as a bind argument.

## Departures from the grammar

**Mixed AND and OR must be parenthesised.** The EBNF binds `OR` tighter than
`AND`, so it reads `a = 1 AND b = 2 OR c = 3` as `a = 1 AND (b = 2 OR c = 3)`.
SQL and the languages a caller writes the filter from bind the other way. Both
groupings parse and both return rows, so a caller holding the second reads a
wrong page rather than an error. Mixing them outside parentheses is
`INVALID_ARGUMENT`.

**Two restrictions must be joined by an operator.** The EBNF lets whitespace
alone join them. Its comment ties that to fuzzy match ranking, which nothing
here does, so what the production leaves is a second spelling of `AND`.

**A minus ahead of an argument signs a number.** The EBNF places `MINUS` on
`term` alone, so `capacity > -5` parses under no rule. A `-` leading the
argument of a restriction is read as the sign of the number after it.

**A value carrying an operator has to be quoted.** TEXT ends at
`( ) - . = : < > ! , ' "`. An RFC3339 timestamp carries both `-` and `:`, so
every timestamp is written quoted.

**The escape sequences a string carries.** The EBNF defines none, so this
package fixes the set to `\\`, `\'`, `\"`, `\n`, `\r` and `\t`. Any other
character after a backslash is an error rather than the character itself, so a
set that grows later breaks no filter that parses today.

**A null is the four characters.** AIP-160 has no null literal, so `null` is
TEXT and reads as a value like any other. Nothing reaches SQL as `IS NULL`.

## Wildcards

A `*` at either end of a quoted value matches a prefix or a suffix. Three
narrowings apply.

It matches text alone, since `LIKE` compares text. A resource name is text and
still takes no wildcard, because a fragment of one names no resource.

It runs under `=` and `!=` alone, since `LIKE` orders nothing.

A `*` opening the value runs only where the spec sets `LeadingWildcard`.
`LIKE 'Ac%'` reaches a btree declared `text_pattern_ops` and reads one range of
it, while `LIKE '%me'` reaches no btree at all. The two cost the same to write
and differ by the whole table to run.

A `*` between the ends is the character, and so is a `*` in an unquoted value.

## Failure modes

A NULL column is invisible to a filter in both directions. It matches no `=`,
which SQL requires, and no `!=` either, since `<>` and `NOT LIKE` compare NULL
to a value as unknown rather than as true.

One value is unwritable. A leading `*` is the wildcard in quotes, and the
unquoted spelling ends at the first operator character, so a value opening with
`*` and carrying one of them, such as `*a-b`, reaches no filter. Escaping the
`*` yields the same character the wildcard is read from.
