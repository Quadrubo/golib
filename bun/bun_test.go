package bun_test

import (
	"context"
	"database/sql"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"

	"github.com/quadrubo/golib/app"
	libbun "github.com/quadrubo/golib/bun"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
	"github.com/quadrubo/golib/postgres"
	tkpostgres "github.com/quadrubo/golib/testkit/postgres"
)

func TestBun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bun Suite")
}

var dsn string

var _ = BeforeSuite(func() {
	pg := tkpostgres.New()

	Expect(pg.Start(context.Background(), do.New())).To(Succeed())
	DeferCleanup(func() { Expect(pg.Stop(context.Background())).To(Succeed()) })

	dsn = pg.URL()
})

var _ = Describe("Module", func() {
	start := func(modules ...app.Module) *app.App {
		GinkgoHelper()

		a, err := app.New(context.Background(), append([]app.Module{
			config.StaticModule(map[string]any{"modules.postgres.url": dsn}),
			logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
			postgres.Module(),
		}, modules...))
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(a.Shutdown(context.Background())).To(Succeed()) })

		return a
	}

	It("wraps the pool it was given", func() {
		a := start(libbun.Module())

		Expect(do.MustInvoke[*bun.DB](a.Injector()).DB).
			To(BeIdenticalTo(do.MustInvoke[*sql.DB](a.Injector())))
	})

	It("queries through the pool it wrapped", func() {
		a := start(libbun.Module())

		var one int
		Expect(do.MustInvoke[*bun.DB](a.Injector()).
			NewSelect().ColumnExpr("1").Scan(context.Background(), &one)).To(Succeed())

		Expect(one).To(Equal(1))
	})

	It("speaks postgres", func() {
		a := start(libbun.Module())

		Expect(do.MustInvoke[*bun.DB](a.Injector()).Dialect().Name()).To(Equal(dialect.PG))
	})

	It("does not shut down, since the pool belongs to postgres", func() {
		_, ok := libbun.Module().(app.Stopper)

		Expect(ok).To(BeFalse())
	})

	It("reports a module list that left out the database", func() {
		_, err := app.New(context.Background(), []app.Module{libbun.Module()})

		Expect(err).To(MatchError(ContainSubstring("bun: failed to invoke the database")))
	})
})
