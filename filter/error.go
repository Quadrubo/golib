package filter

import "fmt"

// Error carries the offset the filter failed at, which a caller reports back to
// whoever wrote the filter.
type Error struct {
	// Position is a 1-based byte offset into the filter.
	Position int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("filter: %s (at position %d)", e.Message, e.Position)
}

func errorAt(position int, format string, args ...any) error {
	return &Error{Position: position, Message: fmt.Sprintf(format, args...)}
}
