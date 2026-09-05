package pagination_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/pagination"
	"github.com/quadrubo/golib/queryfield"
)

func TestPagination(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pagination Suite")
}

type row struct {
	ID          string
	DisplayName string
	CreateTime  time.Time
	Colour      *string
}

var pages = pagination.MustCompile(pagination.Spec[row]{
	Fields: map[string]queryfield.Field[row]{
		"name":         queryfield.Text("id", func(r row) string { return r.ID }),
		"display_name": queryfield.Text("display_name", func(r row) string { return r.DisplayName }),
		"create_time":  queryfield.Time("create_time", func(r row) time.Time { return r.CreateTime }),
		"colour":       queryfield.Text("colour", func(r row) *string { return r.Colour }),
	},
	Tiebreak:        queryfield.Text("id", func(r row) string { return r.ID }),
	DefaultPageSize: 20,
	MaxPageSize:     100,
})

var createTime = time.Date(2026, time.August, 3, 12, 0, 0, 123456000, time.UTC)

// rows leaves colour NULL on every second row, so a cursor taken from one lands
// in the NULL group and a cursor taken from the next does not.
func rows(n int) []row {
	out := make([]row, 0, n)

	for i := range n {
		letter := string(rune('a' + i))

		held := row{
			ID:          letter,
			DisplayName: "room " + letter,
			CreateTime:  createTime.Add(time.Duration(i) * time.Minute),
		}

		if i%2 == 0 {
			colour := "colour " + letter
			held.Colour = &colour
		}

		out = append(out, held)
	}

	return out
}

// page parses the order and the token in one step, which is what a caller with
// nothing to report between the two does.
func page(pageSize int, pageToken, orderBy string) (*pagination.Page[row], error) {
	order, err := pages.ParseOrderBy(orderBy)
	if err != nil {
		return nil, err
	}

	return order.Page(pageSize, pageToken, nil)
}

// nextPage returns the token the given order_by and page size resume from.
func nextPage(orderBy string, size int) string {
	GinkgoHelper()

	first, err := page(size, "", orderBy)
	Expect(err).ToNot(HaveOccurred())

	_, token := first.Result(rows(size + 1))
	Expect(token).ToNot(BeEmpty())

	return token
}

var _ = Describe("ParseOrderBy", func() {
	It("orders by the tiebreak column alone for an omitted order_by", func() {
		sorted, err := page(0, "", "")

		Expect(err).ToNot(HaveOccurred())
		Expect(sorted.Order()).To(Equal([]string{"id ASC"}))
	})

	It("closes the order the client asked for on the tiebreak column", func() {
		sorted, err := page(0, "", "display_name, create_time")

		Expect(err).ToNot(HaveOccurred())
		Expect(sorted.Order()).To(Equal([]string{"display_name ASC", "create_time ASC", "id ASC"}))
	})

	It("appends no tiebreak to an order already closing on that column", func() {
		sorted, err := page(0, "", "name desc")

		Expect(err).ToNot(HaveOccurred())
		Expect(sorted.Order()).To(Equal([]string{"id DESC"}))
	})

	It("reads the direction suffix whatever the client cased it", func() {
		ascending, err := page(0, "", "display_name ASC, create_time asc")

		Expect(err).ToNot(HaveOccurred())
		Expect(ascending.Order()).To(Equal([]string{"display_name ASC", "create_time ASC", "id ASC"}))

		descending, err := page(0, "", "display_name DESC, create_time desc")

		Expect(err).ToNot(HaveOccurred())
		Expect(descending.Order()).To(Equal([]string{"display_name DESC", "create_time DESC", "id DESC"}))
	})

	It("rejects an order running more than one direction", func() {
		_, err := pages.ParseOrderBy("display_name, create_time desc")

		Expect(err).To(MatchError(
			ContainSubstring("orders some fields ascending and some descending")))
	})

	It("expands a nullable field into the flag and the value the order compares", func() {
		sorted, err := page(0, "", "colour desc")

		Expect(err).ToNot(HaveOccurred())
		Expect(sorted.Order()).To(Equal([]string{
			"(colour IS NULL) DESC", "COALESCE(colour, '') DESC", "id DESC",
		}))
	})

	It("rejects a field the spec does not declare", func() {
		_, err := pages.ParseOrderBy("capacity")

		Expect(err).To(MatchError(ContainSubstring(`"capacity" is not an orderable field`)))
	})

	It("rejects the column a field maps to", func() {
		_, err := pages.ParseOrderBy("id")

		Expect(err).To(MatchError(ContainSubstring(`"id" is not an orderable field`)))
	})

	It("rejects a field named twice", func() {
		_, err := pages.ParseOrderBy("display_name, display_name desc")

		Expect(err).To(MatchError(
			ContainSubstring(`"display_name" orders a column the order_by already orders`)))
	})

	It("rejects a direction that is neither asc nor desc", func() {
		_, err := pages.ParseOrderBy("display_name down")

		Expect(err).To(MatchError(ContainSubstring("followed by asc or desc")))
	})
})

var _ = Describe("Page", func() {
	It("returns the default page size for an omitted page_size", func() {
		sized, err := page(0, "", "")

		Expect(err).ToNot(HaveOccurred())
		Expect(sized.Limit()).To(Equal(21))
	})

	It("coerces a page_size above the maximum down", func() {
		sized, err := page(500, "", "")

		Expect(err).ToNot(HaveOccurred())
		Expect(sized.Limit()).To(Equal(101))
	})

	It("returns the page_size the client asked for below the maximum", func() {
		sized, err := page(5, "", "")

		Expect(err).ToNot(HaveOccurred())
		Expect(sized.Limit()).To(Equal(6))
	})
})

var _ = Describe("Keyset", func() {
	It("returns no predicate for a first page", func() {
		first, err := page(0, "", "")
		Expect(err).ToNot(HaveOccurred())

		_, _, ok := first.Keyset()

		Expect(ok).To(BeFalse())
	})

	It("binds the cursor values the last row of the page carries", func() {
		orderBy := "display_name, create_time"
		second, err := page(2, nextPage(orderBy, 2), orderBy)
		Expect(err).ToNot(HaveOccurred())

		sql, args, ok := second.Keyset()
		last := rows(3)[1]

		Expect(ok).To(BeTrue())
		Expect(sql).To(Equal("((display_name, create_time, id) > (?, ?, ?))"))
		Expect(args[0]).To(Equal(last.DisplayName))
		Expect(args[1]).To(BeTemporally("==", last.CreateTime))
		Expect(args[2]).To(Equal(last.ID))
	})

	It("compares the columns as a tuple descending", func() {
		second, err := page(2, nextPage("display_name desc", 2), "display_name desc")
		Expect(err).ToNot(HaveOccurred())

		sql, args, _ := second.Keyset()

		Expect(sql).To(Equal("((display_name, id) < (?, ?))"))
		Expect(args).To(HaveLen(2))
	})

	It("binds a nullable column the cursor holds a value for below the flag", func() {
		second, err := page(1, nextPage("colour", 1), "colour")
		Expect(err).ToNot(HaveOccurred())

		sql, args, _ := second.Keyset()

		Expect(sql).To(Equal(
			"(((colour IS NULL), COALESCE(colour, ''), id) > (FALSE, ?, ?))"))
		Expect(args[0]).To(Equal("colour a"))
	})

	It("compares a cursor in the NULL group against the zero the column substitutes", func() {
		second, err := page(2, nextPage("colour", 2), "colour")
		Expect(err).ToNot(HaveOccurred())

		sql, args, _ := second.Keyset()

		Expect(sql).To(Equal(
			"(((colour IS NULL), COALESCE(colour, ''), id) > (TRUE, '', ?))"))
		Expect(args).To(Equal([]any{"b"}))
	})

	It("compares a default order as a tuple of the tiebreak column alone", func() {
		second, err := page(2, nextPage("", 2), "")
		Expect(err).ToNot(HaveOccurred())

		sql, _, _ := second.Keyset()

		Expect(sql).To(Equal("((id) > (?))"))
	})
})

var _ = Describe("Result", func() {
	It("returns an empty token at the end of the collection", func() {
		last, err := page(3, "", "")
		Expect(err).ToNot(HaveOccurred())

		out, token := last.Result(rows(3))

		Expect(out).To(HaveLen(3))
		Expect(token).To(BeEmpty())
	})

	It("drops the row Limit over-fetched", func() {
		first, err := page(3, "", "")
		Expect(err).ToNot(HaveOccurred())

		out, token := first.Result(rows(4))

		Expect(out).To(Equal(rows(3)))
		Expect(token).ToNot(BeEmpty())
	})

	It("returns a token of base64url characters", func() {
		Expect(nextPage("", 2)).To(MatchRegexp(`^[A-Za-z0-9_-]+$`))
	})
})

var _ = Describe("Page token", func() {
	It("resumes the order the token was issued for", func() {
		orderBy := "create_time desc"
		second, err := page(2, nextPage(orderBy, 2), orderBy)

		Expect(err).ToNot(HaveOccurred())
		Expect(second.Order()).To(Equal([]string{"create_time DESC", "id DESC"}))
	})

	It("carries a time cursor at the microsecond resolution the column stores", func() {
		second, err := page(1, nextPage("create_time", 1), "create_time")
		Expect(err).ToNot(HaveOccurred())

		_, args, _ := second.Keyset()

		Expect(args[0].(time.Time).UnixMicro()).To(Equal(createTime.UnixMicro()))
	})

	It("reads the order_by the client wrote in canonical form", func() {
		second, err := page(2, nextPage("display_name desc", 2), "  display_name   DESC  ")

		Expect(err).ToNot(HaveOccurred())
		Expect(second.Order()).To(Equal([]string{"display_name DESC", "id DESC"}))
	})

	It("rejects a token that is not base64url", func() {
		_, err := page(2, "not a token", "")

		Expect(err).To(MatchError(ContainSubstring("not base64url")))
	})

	It("rejects a token that does not decode", func() {
		_, err := page(2, base64.RawURLEncoding.EncodeToString([]byte("{")), "")

		Expect(err).To(MatchError(ContainSubstring("does not decode")))
	})

	It("rejects a token stamped with another version", func() {
		_, err := page(2, forge(map[string]any{"v": 9, "by": "", "c": []string{"a"}}), "")

		Expect(err).To(MatchError(ContainSubstring("not issued by this build")))
	})

	It("rejects a token carrying more cursor values than the order has columns", func() {
		forged := forge(map[string]any{"v": 1, "by": "display_name", "c": []string{"a", "b", "c"}})

		_, err := page(2, forged, "display_name")

		Expect(err).To(MatchError(ContainSubstring("wrong number of cursor values")))
	})

	It("rejects a token holding no value for a column that is never NULL", func() {
		forged := forge(map[string]any{"v": 1, "by": "display_name", "c": []any{nil, "a"}})

		_, err := page(2, forged, "display_name")

		Expect(err).To(MatchError(ContainSubstring("holds no value for display_name")))
	})

	It("rejects a token issued for another order_by", func() {
		_, err := page(2, nextPage("display_name", 2), "display_name desc")

		Expect(err).To(MatchError(ContainSubstring("issued for another order_by")))
	})

	It("rejects a token issued under another scope, which AIP-158 requires", func() {
		order, err := pages.ParseOrderBy("display_name")
		Expect(err).ToNot(HaveOccurred())

		issued, err := order.Page(2, "", []string{`display_name = "One"`})
		Expect(err).ToNot(HaveOccurred())

		_, token := issued.Result(rows(3))
		Expect(token).ToNot(BeEmpty())

		_, err = order.Page(2, token, []string{`display_name = "Two"`})

		Expect(err).To(MatchError(ContainSubstring("issued under different arguments")))
	})

	It("resumes a token issued under the same scope", func() {
		order, err := pages.ParseOrderBy("display_name")
		Expect(err).ToNot(HaveOccurred())

		issued, err := order.Page(2, "", []string{`display_name = "One"`})
		Expect(err).ToNot(HaveOccurred())

		_, token := issued.Result(rows(3))

		Expect(order.Page(2, token, []string{`display_name = "One"`})).Error().ToNot(HaveOccurred())
	})

	It("honours a page_size the call for a later page changes, which AIP-158 requires", func() {
		resized, err := page(3, nextPage("display_name", 2), "display_name")

		Expect(err).ToNot(HaveOccurred())
		Expect(resized.Limit()).To(Equal(4))
	})

	It("rejects a token carrying a cursor value the column cannot hold", func() {
		forged := forge(map[string]any{"v": 1, "by": "create_time", "c": []string{"now", "a"}})

		_, err := page(2, forged, "create_time")

		Expect(err).To(MatchError(ContainSubstring("unreadable cursor value")))
	})
})

// forge encodes a token the package would not issue, which is the only way to
// reach a decode path a round trip cannot produce.
func forge(fields map[string]any) string {
	GinkgoHelper()

	raw, err := json.Marshal(fields)
	Expect(err).ToNot(HaveOccurred())

	return base64.RawURLEncoding.EncodeToString(raw)
}

var _ = Describe("Spec", func() {
	tiebreak := queryfield.Text("id", func(r row) string { return r.ID })

	It("panics on a paging declared without MustCompile", func() {
		var bypass pagination.Paging[row]

		Expect(func() { _, _ = bypass.ParseOrderBy("") }).
			To(PanicWith(ContainSubstring("did not come from MustCompile")))
	})

	It("panics on a spec mapping a name to a field no constructor returned", func() {
		bad := pagination.Spec[row]{
			Fields:          map[string]queryfield.Field[row]{"colour": {Column: "colour"}},
			Tiebreak:        tiebreak,
			DefaultPageSize: 20,
			MaxPageSize:     100,
		}

		Expect(func() { _ = pagination.MustCompile(bad) }).
			To(PanicWith(ContainSubstring(`maps "colour" to a field no constructor returned`)))
	})

	It("rejects a second name resolving to a column the order_by already orders", func() {
		aliased := pagination.MustCompile(pagination.Spec[row]{
			Fields: map[string]queryfield.Field[row]{
				"name":    queryfield.Text("id", func(r row) string { return r.ID }),
				"room_id": queryfield.Text("id", func(r row) string { return r.ID }),
			},
			Tiebreak:        tiebreak,
			DefaultPageSize: 20,
			MaxPageSize:     100,
		})

		_, err := aliased.ParseOrderBy("name, room_id")

		Expect(err).To(MatchError(
			ContainSubstring(`"room_id" orders a column the order_by already orders`)))
	})

	It("panics on a spec declaring a nullable tiebreak column", func() {
		nullable := queryfield.Text("id", func(r row) *string { return &r.ID })
		bad := pagination.Spec[row]{Tiebreak: nullable, DefaultPageSize: 20, MaxPageSize: 100}

		Expect(func() { _ = pagination.MustCompile(bad) }).
			To(PanicWith(ContainSubstring("nullable tiebreak column")))
	})

	It("panics on a spec declaring no tiebreak column", func() {
		bad := pagination.Spec[row]{DefaultPageSize: 20, MaxPageSize: 100}

		Expect(func() { _ = pagination.MustCompile(bad) }).
			To(PanicWith(ContainSubstring("declares no tiebreak column")))
	})

	It("panics on a spec declaring a default page size below one", func() {
		bad := pagination.Spec[row]{Tiebreak: tiebreak, MaxPageSize: 100}

		Expect(func() { _ = pagination.MustCompile(bad) }).
			To(PanicWith(ContainSubstring("default page size below one")))
	})

	It("panics on a spec declaring a maximum below its default", func() {
		bad := pagination.Spec[row]{Tiebreak: tiebreak, DefaultPageSize: 50, MaxPageSize: 10}

		Expect(func() { _ = pagination.MustCompile(bad) }).
			To(PanicWith(ContainSubstring("maximum page size below its default")))
	})
})
