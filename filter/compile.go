package filter

import (
	"fmt"
	"strings"

	"github.com/quadrubo/golib/queryfield"
)

type Spec[T any] struct {
	// Fields maps the name a client writes in filter to the column it matches.
	Fields map[string]queryfield.Field[T]
	// Functions maps the name a client calls in filter to what the call
	// compiles to. A call not named here is refused.
	Functions map[string]FunctionCompiler
	// LeadingWildcard takes a * opening a value, which matches on a suffix and
	// reads every row of the table, since no btree orders a string by its end.
	LeadingWildcard bool
}

// FunctionCompiler compiles the call a filter writes into the boolean
// predicate it stands for. Each argument arrives as the text the client wrote,
// and the compiler validates count and form. The SQL is parenthesised by the
// caller, so it may carry an OR.
type FunctionCompiler func(args []string) (sql string, binds []any, err error)

// MustCompile returns what a filter is parsed through, and panics on a spec
// declared wrong, which no request can recover from and every request repeats.
func MustCompile[T any](spec Spec[T]) Filtering[T] {
	for name, field := range spec.Fields {
		if !field.Declared() {
			panic(fmt.Sprintf("filter: the spec maps %q to a field no constructor returned", name))
		}
	}

	for name, function := range spec.Functions {
		if function == nil {
			panic(fmt.Sprintf("filter: the spec maps %q to a nil function", name))
		}
	}

	return Filtering[T]{spec: &spec}
}

// Filtering holds the spec MustCompile checked. A value declared without it
// carries no spec, which Parse panics on rather than matching no column.
type Filtering[T any] struct {
	spec *Spec[T]
}

// Predicate is the SQL a filter compiles to and the arguments it binds. Bind
// arguments are placed with ?, which a caller on a driver numbering them
// rebinds. Every chain of more than one part is parenthesised, so a caller
// joining the predicate with AND binds all of it.
type Predicate struct {
	SQL  string
	Args []any
}

// Empty reports whether the filter matched every row, which the caller sending
// none asks for.
func (p Predicate) Empty() bool { return p.SQL == "" }

func (f Filtering[T]) Parse(filter string) (Predicate, error) {
	if f.spec == nil {
		panic("filter: the filtering did not come from MustCompile")
	}

	expression, err := ParseExpression(filter)
	if err != nil || expression == nil {
		return Predicate{}, err
	}

	c := &compiler[T]{spec: f.spec}

	sql, err := c.expression(expression)
	if err != nil {
		return Predicate{}, err
	}

	return Predicate{SQL: sql, Args: c.args}, nil
}

// compiler walks the tree ParseExpression read, refusing the productions this
// service does not serve and binding the values of those it does.
type compiler[T any] struct {
	spec *Spec[T]
	args []any
}

func (c *compiler[T]) expression(expression *Expression) (string, error) {
	// AIP-160 binds OR tighter than AND and SQL binds them the other way, so an
	// expression carrying both names the grouping it means. See docs/filter.md.
	if len(expression.Sequences) > 1 {
		for _, sequence := range expression.Sequences {
			for _, factor := range sequence.Factors {
				if len(factor.Terms) > 1 {
					return "", errorAt(factor.Terms[1].Position,
						`"OR" beside "AND" needs the parentheses naming which binds first`)
				}
			}
		}
	}

	parts := make([]string, 0, len(expression.Sequences))

	for _, sequence := range expression.Sequences {
		part, err := c.sequence(sequence)
		if err != nil {
			return "", err
		}

		parts = append(parts, part)
	}

	return join(parts, " AND "), nil
}

func (c *compiler[T]) sequence(sequence *Sequence) (string, error) {
	// Whitespace joins two factors into the AND the grammar reads it as, which
	// this service asks a filter to spell. See docs/filter.md.
	if len(sequence.Factors) > 1 {
		return "", errorAt(sequence.Factors[1].Position,
			`two restrictions need the "AND" or "OR" joining them`)
	}

	return c.factor(sequence.Factors[0])
}

func (c *compiler[T]) factor(factor *Factor) (string, error) {
	parts := make([]string, 0, len(factor.Terms))

	for _, term := range factor.Terms {
		part, err := c.term(term)
		if err != nil {
			return "", err
		}

		parts = append(parts, part)
	}

	return join(parts, " OR "), nil
}

func (c *compiler[T]) term(term *Term) (string, error) {
	sql, err := c.simple(term.Simple)
	if err != nil {
		return "", err
	}

	if term.Negated {
		return "NOT (" + sql + ")", nil
	}

	return sql, nil
}

func (c *compiler[T]) simple(simple *Simple) (string, error) {
	if simple.Composite != nil {
		return c.expression(simple.Composite)
	}

	return c.restriction(simple.Restriction)
}

func (c *compiler[T]) restriction(restriction *Restriction) (string, error) {
	if restriction.Comparable.Function != nil {
		return c.function(restriction)
	}

	if restriction.Comparator == "" {
		return "", errorAt(restriction.Position, "a filter matches a field against a value")
	}

	if restriction.Comparator == ":" {
		return "", errorAt(restriction.Position, `":" is not implemented`)
	}

	name, err := path(restriction.Comparable, restriction.Position)
	if err != nil {
		return "", err
	}

	field, ok := c.spec.Fields[name]
	if !ok {
		return "", errorAt(restriction.Comparable.Position, "%q is not a filterable field", name)
	}

	if restriction.Arg.Composite != nil {
		return "", errorAt(restriction.Arg.Position, "a filter matches a field against a value")
	}

	literal, err := path(restriction.Arg.Comparable, restriction.Arg.Position)
	if err != nil {
		return "", err
	}

	if trimmed, leading, trailing := wildcard(restriction.Arg); leading || trailing {
		return c.like(restriction, field, name, trimmed, leading, trailing)
	}

	value, err := field.ParseLiteral(literal)
	if err != nil {
		return "", errorAt(restriction.Arg.Position, "%q does not match %q, %s", name, literal, err)
	}

	c.args = append(c.args, value)

	return field.Column + " " + sqlComparator(restriction.Comparator) + " ?", nil
}

// function compiles the call a restriction carries through the function the
// spec declares under its name.
func (c *compiler[T]) function(restriction *Restriction) (string, error) {
	call := restriction.Comparable.Function

	names := make([]string, 0, len(call.Name))
	for _, name := range call.Name {
		names = append(names, name.Text)
	}
	name := strings.Join(names, ".")

	declared, ok := c.spec.Functions[name]
	if !ok {
		return "", errorAt(restriction.Comparable.Position,
			"%q is not a function this filter calls", name)
	}

	if restriction.Comparator != "" {
		return "", errorAt(restriction.Position, "%q compares against no value", name)
	}

	args := make([]string, 0, len(call.Args))

	for _, arg := range call.Args {
		if arg.Composite != nil || arg.Comparable == nil || arg.Comparable.Function != nil {
			return "", errorAt(arg.Position, "%q takes values alone", name)
		}

		text, err := path(arg.Comparable, arg.Position)
		if err != nil {
			return "", err
		}

		args = append(args, text)
	}

	sql, binds, err := declared(args)
	if err != nil {
		return "", errorAt(restriction.Position, "%q does not take these arguments, %s", name, err)
	}

	c.args = append(c.args, binds...)

	return "(" + sql + ")", nil
}

// like emits the pattern match a wildcard asks for, over the value the * marks
// one end of.
func (c *compiler[T]) like(
	restriction *Restriction,
	field queryfield.Field[T],
	name, trimmed string,
	leading, trailing bool,
) (string, error) {
	// LIKE compares text, and a name is text a fragment of which names no
	// resource, so both the other kinds and a name match no wildcard.
	if field.Kind != queryfield.KindText {
		return "", errorAt(restriction.Arg.Position, "%q matches no wildcard", name)
	}

	if restriction.Comparator != "=" && restriction.Comparator != "!=" {
		return "", errorAt(restriction.Position,
			"%q orders no wildcard", restriction.Comparator)
	}

	if leading && !c.spec.LeadingWildcard {
		return "", errorAt(restriction.Arg.Position,
			"a * opening %q reads every row, so this collection takes one closing it alone", name)
	}

	// The value reaches SQL as it was written, with no literal parser between.
	// A wildcard marks a fragment where a parser reads whole values, so one
	// would take books/abc out of *books/abc and match every id ending in abc.
	var pattern strings.Builder

	if leading {
		pattern.WriteString("%")
	}

	pattern.WriteString(likeEscape.Replace(trimmed))

	if trailing {
		pattern.WriteString("%")
	}

	c.args = append(c.args, pattern.String())

	operator := "LIKE"
	if restriction.Comparator == "!=" {
		operator = "NOT LIKE"
	}

	return field.Column + " " + operator + " ?", nil
}

// wildcard strips the * AIP-160 gives a quoted value at either end, which
// matches a prefix or a suffix in place of the whole value. An unquoted value
// carries no wildcard, so a * in one is the character.
func wildcard(arg *Arg) (trimmed string, leading, trailing bool) {
	member := arg.Comparable.Member
	if len(member.Fields) > 0 || !member.Value.Quoted {
		return "", false, false
	}

	trimmed = member.Value.Text

	if leading = strings.HasPrefix(trimmed, "*"); leading {
		trimmed = trimmed[1:]
	}

	if trailing = strings.HasSuffix(trimmed, "*"); trailing {
		trimmed = trimmed[:len(trimmed)-1]
	}

	return trimmed, leading, trailing
}

// likeEscape escapes what LIKE reads as a pattern, so a value carrying one
// matches the character instead. Postgres takes \ as the escape character LIKE
// reads by default.
var likeEscape = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// path returns the dot-joined text a comparable names, which is the field name
// on the left of a comparator and the value on its right.
func path(comparable *Comparable, position int) (string, error) {
	if comparable.Function == nil {
		parts := make([]string, 0, 1+len(comparable.Member.Fields))
		parts = append(parts, comparable.Member.Value.Text)

		for _, field := range comparable.Member.Fields {
			parts = append(parts, field.Text)
		}

		return strings.Join(parts, "."), nil
	}

	names := make([]string, 0, len(comparable.Function.Name))
	for _, name := range comparable.Function.Name {
		names = append(names, name.Text)
	}

	return "", errorAt(position, "%q is not a function this filter calls", strings.Join(names, "."))
}

func sqlComparator(comparator string) string {
	if comparator == "!=" {
		return "<>"
	}

	return comparator
}

// join parenthesises a chain of more than one part, which holds the grouping a
// filter wrote under the precedence SQL gives AND and OR.
func join(parts []string, separator string) string {
	if len(parts) == 1 {
		return parts[0]
	}

	return "(" + strings.Join(parts, separator) + ")"
}
