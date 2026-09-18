package filter_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/filter"
	"github.com/quadrubo/golib/queryfield"
	"github.com/quadrubo/golib/resourcename"
)

type book struct {
	ID          string
	DisplayName string
	Capacity    int64
	CreateTime  time.Time
	Span        time.Duration
}

var bookPattern = resourcename.MustCompile("books/*")

var books = filter.MustCompile(filter.Spec[book]{
	Fields: map[string]queryfield.Field[book]{
		"name":         queryfield.ResourceName("id", bookPattern, func(r book) string { return r.ID }),
		"display_name": queryfield.Text("display_name", func(r book) string { return r.DisplayName }),
		"capacity":     queryfield.Int("capacity", func(r book) int64 { return r.Capacity }),
		"create_time":  queryfield.Time("create_time", func(r book) time.Time { return r.CreateTime }),
		"span":         queryfield.Duration("span_seconds", time.Second, func(r book) time.Duration { return r.Span }),
	},
})

var searchable = filter.MustCompile(filter.Spec[book]{
	Fields: map[string]queryfield.Field[book]{
		"display_name": queryfield.Text("display_name", func(r book) string { return r.DisplayName }),
	},
	LeadingWildcard: true,
})

var _ = Describe("Filtering", func() {
	DescribeTable("compiles a filter into a predicate",
		func(input, sql string, args []any) {
			predicate, err := books.Parse(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(predicate.SQL).To(Equal(sql))
			Expect(predicate.Args).To(Equal(args))
		},
		Entry("equality against a string",
			`display_name = "Alto"`, "display_name = ?", []any{"Alto"}),
		Entry("equality against an unquoted value",
			"display_name = Alto", "display_name = ?", []any{"Alto"}),
		Entry("inequality, which SQL spells the other way",
			`display_name != "Alto"`, "display_name <> ?", []any{"Alto"}),
		Entry("a whole number", "capacity > 20", "capacity > ?", []any{int64(20)}),
		Entry("a signed whole number", "capacity > -5", "capacity > ?", []any{int64(-5)}),
		Entry("an RFC3339 timestamp",
			`create_time >= "2026-01-01T00:00:00Z"`, "create_time >= ?",
			[]any{time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}),
		Entry("a duration in the seconds form",
			`span = "7200s"`, "span_seconds = ?", []any{int64(7200)}),
		Entry("a duration written unquoted",
			"span = 7200s", "span_seconds = ?", []any{int64(7200)}),
		Entry("a signed duration",
			"span > -3600s", "span_seconds > ?", []any{int64(-3600)}),
		Entry("a resource name against the column holding its id",
			`name = "books/abc"`, "id = ?", []any{"abc"}),
		Entry("a resource name under an ordering comparator, which order_by name walks too",
			`name > "books/abc"`, "id > ?", []any{"abc"}),
		Entry("AND",
			`display_name = "a" AND capacity > 2`,
			"(display_name = ? AND capacity > ?)", []any{"a", int64(2)}),
		Entry("OR",
			`display_name = "a" OR display_name = "b"`,
			"(display_name = ? OR display_name = ?)", []any{"a", "b"}),
		Entry("NOT", `NOT display_name = "a"`, "NOT (display_name = ?)", []any{"a"}),
		Entry("OR under AND, parenthesised as this service asks",
			`display_name = "a" AND (capacity > 2 OR capacity < 1)`,
			"(display_name = ? AND (capacity > ? OR capacity < ?))",
			[]any{"a", int64(2), int64(1)}),
		Entry("a * closing a value, which matches on a prefix",
			`display_name = "Stu*"`, "display_name LIKE ?", []any{"Stu%"}),
		Entry("a * closing a value under inequality",
			`display_name != "Stu*"`, "display_name NOT LIKE ?", []any{"Stu%"}),
		Entry("a value carrying what LIKE reads as a pattern",
			`display_name = "50%_\\*"`, "display_name LIKE ?", []any{`50\%\_\\%`}),
		Entry("a * in an unquoted value, which is the character",
			"display_name = Stu*", "display_name = ?", []any{"Stu*"}),
	)

	DescribeTable("compiles a wildcard a spec takes at either end",
		func(input, sql string, args []any) {
			predicate, err := searchable.Parse(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(predicate.SQL).To(Equal(sql))
			Expect(predicate.Args).To(Equal(args))
		},
		Entry("a * opening a value, which matches on a suffix",
			`display_name = "*dio"`, "display_name LIKE ?", []any{"%dio"}),
		Entry("a * at both ends, which matches anywhere in the value",
			`display_name = "*tud*"`, "display_name LIKE ?", []any{"%tud%"}),
		Entry("a * alone, which matches every value",
			`display_name = "*"`, "display_name LIKE ?", []any{"%"}),
	)

	It("matches every row for the filter a caller sends none of", func() {
		predicate, err := books.Parse("")
		Expect(err).ToNot(HaveOccurred())
		Expect(predicate.Empty()).To(BeTrue())
	})

	DescribeTable("refuses what it does not serve",
		func(input, want string) {
			_, err := books.Parse(input)
			Expect(err).To(HaveOccurred())

			var failure *filter.Error
			Expect(errors.As(err, &failure)).To(BeTrue())
			Expect(failure.Message).To(Equal(want))
		},
		Entry("OR beside AND",
			`display_name = "a" AND capacity > 2 OR capacity < 1`,
			`"OR" beside "AND" needs the parentheses naming which binds first`),
		Entry("two restrictions joined by whitespace",
			`display_name = "a" capacity > 2`,
			`two restrictions need the "AND" or "OR" joining them`),
		Entry("a field the spec does not declare",
			`colour = "red"`, `"colour" is not a filterable field`),
		Entry("a field of a field, which the spec declares none of",
			`owner.id = "x"`, `"owner.id" is not a filterable field`),
		Entry("has", `display_name : "a"`, `":" is not implemented`),
		Entry("a global restriction", "capacity", "a filter matches a field against a value"),
		Entry("a function", `lower(display_name) = "a"`,
			`"lower" is not a function this filter calls`),
		Entry("a value the column does not hold",
			`capacity > "twenty"`, `"capacity" does not match "twenty", it is not a number`),
		Entry("a timestamp written as anything else",
			`create_time > "yesterday"`,
			`"create_time" does not match "yesterday", it is not an RFC3339 timestamp`),
		Entry("a duration in the Go spelling",
			"span = 2h0m0s",
			`"span" does not match "2h0m0s", it is not a duration in seconds, such as "7200s"`),
		Entry("a * opening a value, which this spec does not take",
			`display_name = "*dio"`,
			`a * opening "display_name" reads every row, so this collection takes one closing it alone`),
		Entry("a wildcard against a column holding no text",
			`create_time = "2026*"`, `"create_time" matches no wildcard`),
		Entry("a wildcard closing a resource name, a fragment of which names no resource",
			`name = "books/ab*"`, `"name" matches no wildcard`),
		Entry("a wildcard opening a resource name, which matched every id ending in it",
			`name = "*books/abc"`, `"name" matches no wildcard`),
		Entry("a minus opening no number", `capacity > -"5"`, `"-" opens no number`),
		Entry("a wildcard under an ordering comparator",
			`display_name > "Stu*"`, `">" orders no wildcard`),
		Entry("a resource name of another collection",
			`name = "lists/abc"`,
			`"name" does not match "lists/abc", resource name "lists/abc" must match books/*`),
	)

	It("reports the offset the filter failed at", func() {
		_, err := books.Parse(`colour = "red"`)

		Expect(err).To(MatchError(`filter: "colour" is not a filterable field (at position 1)`))
	})

	It("panics on a spec mapping a field no constructor returned", func() {
		Expect(func() {
			filter.MustCompile(filter.Spec[book]{
				Fields: map[string]queryfield.Field[book]{"colour": {Column: "colour"}},
			})
		}).To(PanicWith(`filter: the spec maps "colour" to a field no constructor returned`))
	})

	It("panics on a filtering declared without MustCompile", func() {
		Expect(func() {
			_, _ = filter.Filtering[book]{}.Parse(`display_name = "a"`)
		}).To(PanicWith("filter: the filtering did not come from MustCompile"))
	})
})
