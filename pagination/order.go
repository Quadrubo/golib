package pagination

import (
	"fmt"
	"slices"
	"strings"
)

// parseOrderBy reads the AIP-132 syntax, a comma-separated list of field names
// each optionally suffixed with " desc", and returns it in canonical form so the
// page token does not carry the spacing the caller wrote.
func parseOrderBy[T any](orderBy string, spec Spec[T]) ([]column[T], string, error) {
	var (
		columns   []column[T]
		canonical []string
	)

	seen := map[string]bool{}

	for _, part := range strings.Split(orderBy, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		name, desc, err := parseOrderByField(part)
		if err != nil {
			return nil, "", err
		}

		field, ok := spec.Fields[name]
		if !ok {
			return nil, "", fmt.Errorf("pagination: %q is not an orderable field", name)
		}

		// Two field names may resolve to one column, which the order would then
		// compare twice, so the column is what closes the order against a repeat.
		if seen[field.Column] {
			return nil, "", fmt.Errorf("pagination: %q orders a column the order_by already orders", name)
		}

		seen[field.Column] = true

		columns = append(columns, column[T]{Field: field, desc: desc})

		if desc {
			name += " desc"
		}

		canonical = append(canonical, name)
	}

	// A btree yields one direction forwards and the other backwards, never a mix
	// of the two, so an order mixing them reaches no index and scans the table.
	for _, col := range columns {
		if col.desc != columns[0].desc {
			return nil, "", fmt.Errorf(
				"pagination: %q orders some fields ascending and some descending", orderBy)
		}
	}

	// The tiebreak takes the direction of the column before it, so a btree over
	// both serves the order by scanning one way.
	if !slices.ContainsFunc(columns, func(col column[T]) bool { return col.Column == spec.Tiebreak.Column }) {
		desc := len(columns) > 0 && columns[len(columns)-1].desc
		columns = append(columns, column[T]{Field: spec.Tiebreak, desc: desc})
	}

	return columns, strings.Join(canonical, ", "), nil
}

func parseOrderByField(part string) (name string, desc bool, err error) {
	switch words := strings.Fields(part); len(words) {
	case 1:
		return words[0], false, nil
	case 2:
		switch strings.ToLower(words[1]) {
		case "asc":
			return words[0], false, nil
		case "desc":
			return words[0], true, nil
		}
	}

	return "", false, fmt.Errorf(
		"pagination: %q is not a field name optionally followed by asc or desc", part)
}
