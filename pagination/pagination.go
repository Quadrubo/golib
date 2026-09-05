package pagination

import (
	"fmt"
	"strings"

	"github.com/quadrubo/golib/queryfield"
)

type Spec[T any] struct {
	// Fields maps the name a client writes in order_by to the column it orders.
	Fields map[string]queryfield.Field[T]
	// Tiebreak closes every order on a unique column, which cannot be nullable.
	Tiebreak        queryfield.Field[T]
	DefaultPageSize int
	MaxPageSize     int
}

// MustCompile returns what an order is parsed through, and panics on a spec
// declared wrong, which no request can recover from and every request repeats.
func MustCompile[T any](spec Spec[T]) Paging[T] {
	spec.validate()

	return Paging[T]{spec: &spec}
}

// Paging holds the spec MustCompile validated. ParseOrderBy panics on a value
// declared without one.
type Paging[T any] struct {
	spec *Spec[T]
}

func (s Spec[T]) validate() {
	switch {
	case !s.Tiebreak.Declared():
		panic("pagination: the spec declares no tiebreak column")
	case s.Tiebreak.Nullable():
		panic("pagination: the spec declares a nullable tiebreak column")
	case s.DefaultPageSize < 1:
		panic("pagination: the spec declares a default page size below one")
	case s.MaxPageSize < s.DefaultPageSize:
		panic("pagination: the spec declares a maximum page size below its default")
	}

	for name, field := range s.Fields {
		if !field.Declared() {
			panic(fmt.Sprintf("pagination: the spec maps %q to a field no constructor returned", name))
		}
	}
}

type Order[T any] struct {
	spec      *Spec[T]
	columns   []column[T]
	canonical string
}

type column[T any] struct {
	queryfield.Field[T]

	desc bool
}

// terms returns the SQL a column contributes to the order and to the keyset
// tuple. A nullable column leads with the flag, which orders its NULL rows
// last ascending and first descending, and follows with COALESCE, which holds
// those rows at one value the tuple compares decisively against.
func (c column[T]) terms() []string {
	if !c.Nullable() {
		return []string{c.Column}
	}

	return []string{
		fmt.Sprintf("(%s IS NULL)", c.Column),
		fmt.Sprintf("COALESCE(%s, %s)", c.Column, c.Zero()),
	}
}

// desc reports the direction the order runs, which parseOrderBy requires every
// column to share.
func (o Order[T]) desc() bool { return o.columns[0].desc }

func (p Paging[T]) ParseOrderBy(orderBy string) (Order[T], error) {
	if p.spec == nil {
		panic("pagination: the paging did not come from MustCompile")
	}

	columns, canonical, err := parseOrderBy(orderBy, *p.spec)
	if err != nil {
		return Order[T]{}, err
	}

	return Order[T]{spec: p.spec, columns: columns, canonical: canonical}, nil
}

// Page rejects a token issued under another order_by or another scope, which
// AIP-158 requires the call for the next page to repeat.
//
// scope lists whatever else narrows the collection, such as the parent and the
// filter. Each part is compared as text, so a caller respelling the same filter
// walks no further.
func (o Order[T]) Page(pageSize int, pageToken string, scope []string) (*Page[T], error) {
	joined := strings.Join(scope, "\n")
	page := &Page[T]{order: o, size: o.spec.clamp(pageSize), scope: joined}

	if pageToken == "" {
		return page, nil
	}

	cursor, err := decodeToken(pageToken, o, joined)
	if err != nil {
		return nil, err
	}

	page.cursor = cursor

	return page, nil
}

type Page[T any] struct {
	order  Order[T]
	size   int
	scope  string
	cursor []any
}

// Limit over-fetches one row, which is how Result tells a further page from the
// end of the collection.
func (p *Page[T]) Limit() int { return p.size + 1 }

func (p *Page[T]) Order() []string {
	order := make([]string, 0, len(p.order.columns))

	for _, col := range p.order.columns {
		direction := "ASC"
		if col.desc {
			direction = "DESC"
		}

		for _, term := range col.terms() {
			order = append(order, fmt.Sprintf("%s %s", term, direction))
		}
	}

	return order
}

// Result drops the row Limit over-fetched and returns the token the call for the
// next page resumes from, empty at the end of the collection.
func (p *Page[T]) Result(rows []T) ([]T, string) {
	if len(rows) <= p.size {
		return rows, ""
	}

	rows = rows[:p.size]

	return rows, encodeToken(p.order, p.scope, rows[len(rows)-1])
}

func (s Spec[T]) clamp(requested int) int {
	size := requested
	if size <= 0 {
		size = s.DefaultPageSize
	}

	if size > s.MaxPageSize {
		size = s.MaxPageSize
	}

	return size
}
