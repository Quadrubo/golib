package queryfield

import (
	"errors"
	"strconv"
	"time"

	"github.com/quadrubo/golib/resourcename"
)

type Kind int

const (
	KindText Kind = iota
	KindTime
	KindInt
	KindResourceName
	KindBool
	KindFloat
	KindEnum
)

// A value func returns either the column type or a pointer to it, and the
// pointer form is what declares the column nullable.
type (
	textValue  interface{ string | *string }
	timeValue  interface{ time.Time | *time.Time }
	intValue   interface{ int64 | *int64 }
	boolValue  interface{ bool | *bool }
	floatValue interface{ float64 | *float64 }
)

// Field is the column a query field name resolves to. Column is repo-controlled
// SQL, never client input.
type Field[T any] struct {
	Column string
	Kind   Kind

	nullable bool
	zero     string

	cursor       func(T) *string
	parseCursor  func(string) (any, error)
	parseLiteral func(string) (any, error)
}

// Declared reports whether the field came from a constructor.
func (f Field[T]) Declared() bool {
	return f.Column != "" && f.cursor != nil && f.parseLiteral != nil
}

func (f Field[T]) Nullable() bool { return f.nullable }

// Zero returns the SQL literal COALESCE puts in place of NULL, which collapses
// the NULL rows onto one value a row comparison still decides against.
func (f Field[T]) Zero() string { return f.zero }

// Cursor returns the column encoded for a page token, or nil where it is NULL.
func (f Field[T]) Cursor(row T) *string { return f.cursor(row) }

func (f Field[T]) ParseCursor(raw string) (any, error) { return f.parseCursor(raw) }

// ParseLiteral returns the bind argument a filter value stands for, which a
// cursor value of the same column does not always spell the same way.
func (f Field[T]) ParseLiteral(raw string) (any, error) { return f.parseLiteral(raw) }

func Text[T any, V textValue](column string, value func(T) V) Field[T] {
	field := declare[T, V, string](column, KindText, "''", value, encodeText, parseText)
	field.parseLiteral = parseText

	return field
}

// Time encodes the cursor as epoch microseconds, the resolution timestamptz
// stores, and reads a filter value as the RFC3339 AIP-160 states.
func Time[T any, V timeValue](column string, value func(T) V) Field[T] {
	field := declare[T, V, time.Time](
		column, KindTime, "'-infinity'::timestamptz", value, encodeTime, parseTime)
	field.parseLiteral = parseLiteralTime

	return field
}

func Int[T any, V intValue](column string, value func(T) V) Field[T] {
	field := declare[T, V, int64](column, KindInt, "0", value, encodeInt, parseInt)
	field.parseLiteral = parseInt

	return field
}

// Float encodes the cursor as the shortest decimal that parses back to the
// same bits, so a page boundary never skips or repeats a row.
func Float[T any, V floatValue](column string, value func(T) V) Field[T] {
	field := declare[T, V, float64](
		column, KindFloat, "'-Infinity'::float8", value, encodeFloat, parseFloat)
	field.parseLiteral = parseFloat

	return field
}

// Bool reads a filter value as the word, so a filter writes hidden = true with
// no quotes around it.
func Bool[T any, V boolValue](column string, value func(T) V) Field[T] {
	field := declare[T, V, bool](column, KindBool, "false", value, encodeBool, parseBool)
	field.parseLiteral = parseBool

	return field
}

// Enum maps the name a filter writes to the word the column holds, so a filter
// writes phase = PHASE_OPEN against a column holding open. A name outside the
// map is rejected, which keeps the stored words out of the API.
func Enum[T any, V textValue](
	column string,
	values map[string]string,
	value func(T) V,
) Field[T] {
	field := declare[T, V, string](column, KindEnum, "''", value, encodeText, parseText)
	field.parseLiteral = func(raw string) (any, error) {
		held, ok := values[raw]
		if !ok {
			return nil, errors.New("it is not a value of the enum")
		}

		return held, nil
	}

	return field
}

// ResourceName declares a column holding the id a resource name ends in, so a
// filter writes books/abc against a column holding abc. Its kind separates it
// from text, since a fragment of a name reaches no pattern and unpacks to no
// id.
func ResourceName[T any, V textValue](
	column string,
	pattern resourcename.Pattern,
	value func(T) V,
) Field[T] {
	field := declare[T, V, string](column, KindResourceName, "''", value, encodeText, parseText)
	field.parseLiteral = func(raw string) (any, error) {
		ids, err := pattern.Parse(raw)
		if err != nil {
			return nil, err
		}

		return ids[len(ids)-1], nil
	}

	return field
}

// declare reads the nullability off V, which is a pointer exactly where the
// column holds NULL.
func declare[T, V, U any](
	column string,
	kind Kind,
	zero string,
	value func(T) V,
	encode func(U) string,
	parse func(string) (any, error),
) Field[T] {
	field := Field[T]{Column: column, Kind: kind, zero: zero, parseCursor: parse}

	var held V
	if _, field.nullable = any(held).(*U); field.nullable {
		field.cursor = func(row T) *string {
			held, _ := any(value(row)).(*U)
			if held == nil {
				return nil
			}

			raw := encode(*held)

			return &raw
		}

		return field
	}

	field.cursor = func(row T) *string {
		held, _ := any(value(row)).(U)
		raw := encode(held)

		return &raw
	}

	return field
}

func encodeText(value string) string { return value }

func encodeTime(value time.Time) string { return strconv.FormatInt(value.UnixMicro(), 10) }

func encodeInt(value int64) string { return strconv.FormatInt(value, 10) }

func encodeFloat(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }

func encodeBool(value bool) string { return strconv.FormatBool(value) }

func parseText(raw string) (any, error) { return raw, nil }

func parseTime(raw string) (any, error) {
	micros, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, errors.New("it is not epoch microseconds")
	}

	return time.UnixMicro(micros), nil
}

func parseLiteralTime(raw string) (any, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, errors.New("it is not an RFC3339 timestamp")
	}

	return value, nil
}

func parseInt(raw string) (any, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, errors.New("it is not a number")
	}

	return value, nil
}

func parseFloat(raw string) (any, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, errors.New("it is not a number")
	}

	return value, nil
}

// parseBool takes the two words FormatBool writes and no other spelling, which
// strconv.ParseBool would widen to 1, t and TRUE.
func parseBool(raw string) (any, error) {
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}

	return nil, errors.New("it is not true or false")
}
