package pagination_test

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/pagination"
	"github.com/quadrubo/golib/queryfield"
	tkpostgres "github.com/quadrubo/golib/testkit/postgres"
)

type nullRow struct {
	ID     string
	Colour *string
}

var nullPages = pagination.MustCompile(pagination.Spec[nullRow]{
	Fields: map[string]queryfield.Field[nullRow]{
		"colour": queryfield.Text("colour", func(r nullRow) *string { return r.Colour }),
	},
	Tiebreak:        queryfield.Text("id", func(r nullRow) string { return r.ID }),
	DefaultPageSize: 20,
	MaxPageSize:     100,
})

var _ = Describe("A walk over a nullable column", Ordered, func() {
	var db *sql.DB

	BeforeAll(func() {
		ctx := context.Background()

		pg := tkpostgres.New()

		Expect(pg.Start(ctx, do.New())).To(Succeed())
		DeferCleanup(func() { Expect(pg.Stop(context.Background())).To(Succeed()) })

		var err error
		db, err = sql.Open("pgx", pg.URL())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		_, err = db.ExecContext(ctx, `
			CREATE TABLE swatches (id text PRIMARY KEY, colour text);
			INSERT INTO swatches (id, colour) VALUES
				('a', 'green'), ('b', NULL), ('c', 'amber'),
				('d', NULL), ('e', 'green'), ('f', 'blue');`)
		Expect(err).ToNot(HaveOccurred())
	})

	// walk reads the table one row per page and returns the ids in the order the
	// pages returned them, which is where a dropped or repeated row shows up.
	walk := func(orderBy string, size int) []string {
		GinkgoHelper()

		var (
			out   []string
			token string
		)

		for range 20 {
			order, err := nullPages.ParseOrderBy(orderBy)
			Expect(err).ToNot(HaveOccurred())

			p, err := order.Page(size, token, nil)
			Expect(err).ToNot(HaveOccurred())

			query := "SELECT id, colour FROM swatches"
			args := []any{}

			if keyset, keysetArgs, ok := p.Keyset(); ok {
				query += " WHERE " + rebind(keyset)
				args = keysetArgs
			}

			query += " ORDER BY " + strings.Join(p.Order(), ", ") +
				" LIMIT " + strconv.Itoa(p.Limit())

			rows, err := db.QueryContext(context.Background(), query, args...)
			Expect(err).ToNot(HaveOccurred())

			var found []nullRow

			for rows.Next() {
				var r nullRow
				Expect(rows.Scan(&r.ID, &r.Colour)).To(Succeed())
				found = append(found, r)
			}

			Expect(rows.Err()).ToNot(HaveOccurred())
			Expect(rows.Close()).To(Succeed())

			kept, next := p.Result(found)
			for _, r := range kept {
				out = append(out, r.ID)
			}

			if next == "" {
				return out
			}

			token = next
		}

		Fail("the walk did not reach the end of the table")

		return nil
	}

	It("returns every row once, ascending with the NULLs last", func() {
		Expect(walk("colour", 1)).To(Equal([]string{"c", "f", "a", "e", "b", "d"}))
	})

	It("returns every row once, descending with the NULLs first", func() {
		Expect(walk("colour desc", 1)).To(Equal([]string{"d", "b", "e", "a", "f", "c"}))
	})

	It("returns the same order however many rows a page holds", func() {
		for _, orderBy := range []string{"colour", "colour desc"} {
			Expect(walk(orderBy, 1)).To(Equal(walk(orderBy, 6)))
			Expect(walk(orderBy, 2)).To(Equal(walk(orderBy, 6)))
		}
	})
})

// rebind turns the bun placeholder into the numbered one database/sql wants.
func rebind(query string) string {
	var out []byte

	n := 0

	for i := range len(query) {
		if query[i] != '?' {
			out = append(out, query[i])

			continue
		}

		n++
		out = append(out, '$')
		out = append(out, strconv.Itoa(n)...)
	}

	return string(out)
}
