package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"github.com/uptrace/bun"

	"github.com/quadrubo/golib/testkit/postgres"
)

func TestPostgres(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testkit Postgres Suite")
}

var (
	pg       *postgres.Postgres
	injector do.Injector
)

var _ = BeforeSuite(func() {
	pg = postgres.New(postgres.WithDatabase("kit"))
	injector = do.New()

	Expect(pg.Start(context.Background(), injector)).To(Succeed())
	DeferCleanup(func() { Expect(pg.Stop(context.Background())).To(Succeed()) })
})

var _ = Describe("Postgres", func() {
	ctx := context.Background()

	It("provides a connection to the database it named", func() {
		db := do.MustInvoke[*bun.DB](injector)

		var name string
		Expect(db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name)).To(Succeed())
		Expect(name).To(Equal("kit"))
	})

	It("points the service at the same database", func() {
		modules := pg.Settings()["modules"].(map[string]any)
		conf := modules["postgres"].(map[string]any)

		Expect(conf["url"]).To(Equal(pg.URL()))
		Expect(pg.URL()).To(ContainSubstring("/kit"))
	})

	It("quotes the name it is given", func() {
		url, err := pg.CreateDatabase(ctx, "needs quoting")
		Expect(err).ToNot(HaveOccurred())

		db, err := sql.Open("pgx", url)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		var name string
		Expect(db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name)).To(Succeed())
		Expect(name).To(Equal("needs quoting"))
	})

	It("creates a further database on the running container", func() {
		url, err := pg.CreateDatabase(ctx, "extra")
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(ContainSubstring("/extra"))

		db, err := sql.Open("pgx", url)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		var name string
		Expect(db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name)).To(Succeed())
		Expect(name).To(Equal("extra"))
	})
})
