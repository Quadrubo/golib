package filter_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/filter"
)

func TestFilter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Suite")
}

// render writes the tree back as nested calls, so an entry below states the
// grouping the grammar puts on a filter rather than the text it came from.
func render(expression *filter.Expression) string {
	parts := make([]string, 0, len(expression.Sequences))
	for _, sequence := range expression.Sequences {
		parts = append(parts, renderSequence(sequence))
	}

	return renderJoin("AND", parts)
}

func renderJoin(name string, parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}

	return name + "(" + strings.Join(parts, ", ") + ")"
}

func renderSequence(sequence *filter.Sequence) string {
	parts := make([]string, 0, len(sequence.Factors))
	for _, factor := range sequence.Factors {
		parts = append(parts, renderFactor(factor))
	}

	return renderJoin("SEQ", parts)
}

func renderFactor(factor *filter.Factor) string {
	parts := make([]string, 0, len(factor.Terms))
	for _, term := range factor.Terms {
		parts = append(parts, renderTerm(term))
	}

	return renderJoin("OR", parts)
}

func renderTerm(term *filter.Term) string {
	if term.Negated {
		return "-(" + renderSimple(term.Simple) + ")"
	}

	return renderSimple(term.Simple)
}

func renderSimple(simple *filter.Simple) string {
	if simple.Composite != nil {
		return render(simple.Composite)
	}

	return renderRestriction(simple.Restriction)
}

func renderRestriction(restriction *filter.Restriction) string {
	if restriction.Comparator == "" {
		return renderComparable(restriction.Comparable)
	}

	return renderComparable(restriction.Comparable) +
		" " + restriction.Comparator + " " + renderArg(restriction.Arg)
}

func renderComparable(comparable *filter.Comparable) string {
	if comparable.Function != nil {
		return renderFunction(comparable.Function)
	}

	return renderMember(comparable.Member)
}

func renderMember(member *filter.Member) string {
	parts := []string{renderValue(member.Value)}
	for _, field := range member.Fields {
		parts = append(parts, renderValue(field))
	}

	return strings.Join(parts, ".")
}

func renderFunction(function *filter.Function) string {
	names := make([]string, 0, len(function.Name))
	for _, name := range function.Name {
		names = append(names, renderValue(name))
	}

	args := make([]string, 0, len(function.Args))
	for _, arg := range function.Args {
		args = append(args, renderArg(arg))
	}

	return strings.Join(names, ".") + "(" + strings.Join(args, ", ") + ")"
}

func renderArg(arg *filter.Arg) string {
	if arg.Composite != nil {
		return "(" + render(arg.Composite) + ")"
	}

	return renderComparable(arg.Comparable)
}

func renderValue(value filter.Value) string {
	if value.Quoted {
		return strconv.Quote(value.Text)
	}

	return value.Text
}

var _ = Describe("ParseExpression", func() {
	DescribeTable("groups the examples AIP-160 publishes",
		func(input, want string) {
			expression, err := filter.ParseExpression(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(render(expression)).To(Equal(want))
		},
		Entry("a global restriction", "prod", "prod"),
		Entry("a sequence", "New York Giants", "SEQ(New, York, Giants)"),
		Entry("OR inside a sequence",
			"New York Giants OR Yankees", "SEQ(New, York, OR(Giants, Yankees))"),
		Entry("the parenthesised form of that sequence",
			"New York (Giants OR Yankees)", "SEQ(New, York, OR(Giants, Yankees))"),
		Entry("a sequence inside AND", "a b AND c AND d", "AND(SEQ(a, b), c, d)"),
		Entry("the parenthesised form of that expression",
			"(a b) AND c AND d", "AND(SEQ(a, b), c, d)"),
		Entry("OR over two restrictions",
			"a < 10 OR a >= 100", "OR(a < 10, a >= 100)"),
		Entry("OR over three terms", "a OR b OR c", "OR(a, b, c)"),
		Entry("OR binding tighter than AND", "a AND b OR c", "AND(a, OR(b, c))"),
		Entry("NOT over a composite", "NOT (a OR b)", "-(OR(a, b))"),
		Entry("a minus negating a restriction", `-file:".java"`, `-(file : ".java")`),
		Entry("a minus negating a global restriction", "-30", "-(30)"),
		Entry("equality without whitespace", "package=com.google", "package = com.google"),
		Entry("inequality against a single-quoted string", "msg != 'hello'", `msg != "hello"`),
		Entry("inequality against a double-quoted string", `msg != "hello"`, `msg != "hello"`),
		Entry("greater than", "1 > 0", "1 > 0"),
		Entry("greater or equal", "2.5 >= 2.4", "2.5 >= 2.4"),
		Entry("a dotted member as the argument",
			"yesterday < request.time", "yesterday < request.time"),
		Entry("a function as the argument",
			"experiment.rollout <= cohort(request.user)",
			"experiment.rollout <= cohort(request.user)"),
		Entry("has", "map:key", "map : key"),
		Entry("a member of four fields", "expr.type_map.1.type", "expr.type_map.1.type"),
		Entry("a function of two arguments",
			`regex(m.key, '^.*prod.*$')`, `regex(m.key, "^.*prod.*$")`),
		Entry("a function under a qualified name", "math.mem('30mb')", `math.mem("30mb")`),
		Entry("a composite holding a function and a restriction",
			`(msg.endsWith('world') AND retries < 10)`,
			`AND(msg.endsWith("world"), retries < 10)`),
		Entry("the timestamp function",
			`timestamp("2012-04-21T11:30:00-04:00")`, `timestamp("2012-04-21T11:30:00-04:00")`),
		Entry("the duration function", `duration("32s")`, `duration("32s")`),
		Entry("has against a wildcard", "r:*", "r : *"),
		Entry("has on a repeated field", "r.foo:42", "r.foo : 42"),
		Entry("a duration written as text", "20s", "20s"),
		Entry("a fractional duration written as text", "1.2s", "1.2s"),
		Entry("a minus signing an argument the EBNF reaches through none",
			"capacity > -5", "capacity > -5"),
	)

	It("returns no expression for the empty filter the grammar admits", func() {
		Expect(filter.ParseExpression("")).To(BeNil())
		Expect(filter.ParseExpression("   ")).To(BeNil())
	})

	It("reads NOT carrying no whitespace as a function name", func() {
		expression, err := filter.ParseExpression("NOT(a)")
		Expect(err).ToNot(HaveOccurred())
		Expect(render(expression)).To(Equal("NOT(a)"))
	})

	It("unescapes the sequences it defines", func() {
		expression, err := filter.ParseExpression(`a = "one\ttwo\"three\\"`)
		Expect(err).ToNot(HaveOccurred())
		Expect(render(expression)).To(Equal(`a = "one\ttwo\"three\\"`))
	})

	DescribeTable("returns the offset a filter fails at",
		func(input, want string, position int) {
			_, err := filter.ParseExpression(input)
			Expect(err).To(HaveOccurred())

			var failure *filter.Error
			Expect(errors.As(err, &failure)).To(BeTrue())
			Expect(failure.Message).To(Equal(want))
			Expect(failure.Position).To(Equal(position))
		},
		Entry("a string that never closes", `a = "hello`, "the string is not closed", 5),
		Entry("an escape it does not define", `a = "\q"`, `\q is not an escape sequence`, 6),
		Entry("a bang opening no comparator", "a ! b", `"!" opens no comparator other than "!="`, 3),
		Entry("a parenthesis that never closes", "(a OR b", `the "(" is not closed`, 1),
		Entry("a parenthesis closing nothing", "a b)", `")" joins no expression`, 4),
		Entry("AND running out of input", "a AND", `"AND" is not followed by whitespace`, 3),
		Entry("AND missing its right side", "a AND ", "the end of the filter is not a value", 7),
		Entry("a comparator missing its argument", "a = ", "the end of the filter is not a value", 5),
		Entry("a byte no rune starts, inside a string",
			"a = \"x\xffy\"", "the filter is not valid UTF-8", 7),
		Entry("a byte no rune starts, in an unquoted value",
			"a = x\xffy", "the filter is not valid UTF-8", 6),
		Entry("a byte no rune starts, opening the filter",
			"\xffa = x", "the filter is not valid UTF-8", 1),
	)
})
