package pagination

import (
	"fmt"
	"strings"
)

// Keyset returns the predicate skipping every row up to and including the one
// the cursor names, parenthesised so a caller joining a filter with AND binds
// all of it. Bind arguments are placed with ?, which a caller on a driver
// numbering them rebinds.
func (p *Page[T]) Keyset() (sql string, args []any, ok bool) {
	if p.cursor == nil {
		return "", nil, false
	}

	terms := make([]string, 0, len(p.order.columns))
	placeholders := make([]string, 0, len(p.order.columns))
	args = make([]any, 0, len(p.order.columns))

	for i, col := range p.order.columns {
		terms = append(terms, col.terms()...)

		switch {
		case !col.Nullable():
			placeholders = append(placeholders, "?")
			args = append(args, p.cursor[i])
		// A cursor carrying no value sits in the NULL group, which the flag enters
		// and the zero the column substitutes compares within.
		case p.cursor[i] == nil:
			placeholders = append(placeholders, "TRUE", col.Zero())
		default:
			placeholders = append(placeholders, "FALSE", "?")
			args = append(args, p.cursor[i])
		}
	}

	sql = fmt.Sprintf("((%s) %s (%s))",
		strings.Join(terms, ", "), step(p.order.desc()), strings.Join(placeholders, ", "))

	return sql, args, true
}

func step(desc bool) string {
	if desc {
		return "<"
	}

	return ">"
}
