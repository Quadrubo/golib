package filter_test

import (
	"strings"
	"testing"

	"github.com/quadrubo/golib/filter"
)

// FuzzParse walks arbitrary input through the grammar and the compiler, which a
// client reaches both of with a string it chooses. A filter either fails or
// binds every value it carries; no input reaches the SQL as text.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"prod",
		"New York Giants OR Yankees",
		"a b AND c AND d",
		"NOT (a OR b)",
		`-file:".java"`,
		"expr.type_map.1.type",
		`regex(m.key, '^.*prod.*$')`,
		`display_name = "Alto"`,
		`display_name = "St*io"`,
		`display_name = "*dio*"`,
		`display_name = "50%_\\"`,
		`name = "books/abc"`,
		"capacity > -5",
		`create_time >= "2026-01-01T00:00:00Z"`,
		`a = "unterminated`,
		"((((((((((a",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		expression, err := filter.ParseExpression(input)
		if err != nil && expression != nil {
			t.Fatalf("ParseExpression(%q) returned both an expression and %v", input, err)
		}

		for _, filtering := range []filter.Filtering[book]{books, searchable} {
			predicate, err := filtering.Parse(input)
			if err != nil {
				continue
			}

			if holders := strings.Count(predicate.SQL, "?"); holders != len(predicate.Args) {
				t.Fatalf("Parse(%q) bound %d arguments into %d placeholders: %s",
					input, len(predicate.Args), holders, predicate.SQL)
			}
		}
	})
}
