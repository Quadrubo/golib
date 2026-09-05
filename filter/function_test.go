package filter_test

import (
	"errors"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/filter"
	"github.com/quadrubo/golib/queryfield"
)

// within compiles within(low, high) against the capacity column, which gives
// the specs a function taking numbers and returning two binds.
func within(args []string) (string, []any, error) {
	if len(args) != 2 {
		return "", nil, errors.New("takes a low and a high")
	}

	binds := make([]any, 0, len(args))
	for _, arg := range args {
		number, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return "", nil, errors.New("takes whole numbers")
		}
		binds = append(binds, number)
	}

	return "capacity BETWEEN ? AND ?", binds, nil
}

var called = filter.MustCompile(filter.Spec[book]{
	Fields: map[string]queryfield.Field[book]{
		"display_name": queryfield.Text("display_name", func(r book) string { return r.DisplayName }),
	},
	Functions: map[string]filter.FunctionCompiler{
		"within": within,
	},
})

var _ = Describe("A declared function", func() {
	DescribeTable("compiles the call into the predicate it stands for",
		func(input, sql string, args []any) {
			predicate, err := called.Parse(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(predicate.SQL).To(Equal(sql))
			Expect(predicate.Args).To(Equal(args))
		},
		Entry("a bare call",
			"within(2, 5)", "(capacity BETWEEN ? AND ?)", []any{int64(2), int64(5)}),
		Entry("a call joined with a restriction",
			`within(2, 5) AND display_name = "Alto"`,
			`((capacity BETWEEN ? AND ?) AND display_name = ?)`,
			[]any{int64(2), int64(5), "Alto"}),
		Entry("a negated call",
			"NOT within(2, 5)", "NOT ((capacity BETWEEN ? AND ?))", []any{int64(2), int64(5)}),
		Entry("two calls joined by OR",
			"within(2, 5) OR within(7, 9)",
			"((capacity BETWEEN ? AND ?) OR (capacity BETWEEN ? AND ?))",
			[]any{int64(2), int64(5), int64(7), int64(9)}),
	)

	DescribeTable("refuses a call outside its declaration",
		func(input, message string) {
			_, err := called.Parse(input)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("a name the spec does not declare",
			"regex(display_name)", `"regex" is not a function this filter calls`),
		Entry("a call compared against a value",
			"within(2, 5) = true", `"within" compares against no value`),
		Entry("an argument the function refuses",
			"within(2)", `"within" does not take these arguments, takes a low and a high`),
		Entry("an argument that is not a number",
			`within(2, "many")`, `"within" does not take these arguments, takes whole numbers`),
		Entry("an argument nesting a call",
			"within(within(1, 2), 5)", `"within" takes values alone`),
	)

	It("panics on a spec mapping a name to nil", func() {
		Expect(func() {
			filter.MustCompile(filter.Spec[book]{
				Functions: map[string]filter.FunctionCompiler{"broken": nil},
			})
		}).To(PanicWith(ContainSubstring(`maps "broken" to a nil function`)))
	})
})
