package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
	"github.com/quadrubo/golib/postgres"
	tkpostgres "github.com/quadrubo/golib/testkit/postgres"
)

func TestPostgres(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Suite")
}

// One container for the suite, since none of these specs write.
var dsn string

var _ = BeforeSuite(func() {
	pg := tkpostgres.New()

	Expect(pg.Start(context.Background(), do.New())).To(Succeed())
	DeferCleanup(func() { Expect(pg.Stop(context.Background())).To(Succeed()) })

	dsn = pg.URL()
})

var _ = Describe("Module", func() {
	start := func(settings map[string]any) (*app.App, error) {
		a, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(settings),
				logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
				postgres.Module(),
			})
		if a != nil {
			DeferCleanup(func() { _ = a.Shutdown(context.Background()) })
		}
		return a, err
	}

	It("provides a pool that reaches the database", func() {
		a, err := start(map[string]any{"modules.postgres.url": dsn})
		Expect(err).ToNot(HaveOccurred())

		db, err := do.Invoke[*sql.DB](a.Injector())
		Expect(err).ToNot(HaveOccurred())
		Expect(db.PingContext(context.Background())).To(Succeed())
	})

	It("applies the pool limits", func() {
		a, err := start(map[string]any{
			"modules.postgres.url":            dsn,
			"modules.postgres.max_open_conns": 3,
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(do.MustInvoke[*sql.DB](a.Injector()).Stats().MaxOpenConnections).To(Equal(3))
	})

	It("closes the pool on shutdown", func() {
		a, err := start(map[string]any{"modules.postgres.url": dsn})
		Expect(err).ToNot(HaveOccurred())

		db := do.MustInvoke[*sql.DB](a.Injector())
		Expect(a.Shutdown(context.Background())).To(Succeed())

		Expect(db.PingContext(context.Background())).To(MatchError(ContainSubstring("database is closed")))
	})

	It("refuses a database it cannot reach", func() {
		_, err := start(map[string]any{
			"modules.postgres.url": "postgres://test:hunter2@127.0.0.1:1/test?sslmode=disable",
		})

		Expect(err).To(MatchError(ContainSubstring("failed to reach")))
		Expect(err.Error()).ToNot(ContainSubstring("hunter2"))
	})

	It("gives up on a database that does not answer within the connect timeout", func() {
		// Nothing accepts on the listener, so the kernel backlog completes the TCP
		// handshake and the pgx startup message waits on a reply that never comes.
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { _ = lis.Close() })

		began := time.Now()
		_, err = start(map[string]any{
			"modules.postgres.url":             "postgres://test:test@" + lis.Addr().String() + "/test?sslmode=disable",
			"modules.postgres.connect_timeout": "1s",
		})

		Expect(err).To(MatchError(ContainSubstring("context deadline exceeded")))
		// Well under the 10s default, so the configured timeout is what bounded it.
		Expect(time.Since(began)).To(BeNumerically("<", 5*time.Second))
	})

	It("refuses a connect timeout below a second", func() {
		_, err := start(map[string]any{
			"modules.postgres.url":             dsn,
			"modules.postgres.connect_timeout": "0s",
		})

		Expect(err).To(MatchError(ContainSubstring("connect_timeout")))
	})

	It("refuses a config without a url", func() {
		_, err := start(map[string]any{})

		Expect(err).To(MatchError(ContainSubstring("url")))
	})

	It("reports a module list that left out the logger", func() {
		_, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(map[string]any{"modules.postgres.url": dsn}),
				postgres.Module(),
			})

		Expect(err).To(MatchError(ContainSubstring("postgres: failed to invoke the logger")))
	})
})

var _ = Describe("Errors", func() {
	// Provoke real violations, since these predicates are only worth anything if
	// they match what postgres returns.
	var db *sql.DB

	BeforeEach(func() {
		var err error
		db, err = sql.Open("pgx", dsn)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		_, err = db.ExecContext(context.Background(), `
			DROP TABLE IF EXISTS uses, books;
			CREATE TABLE books (id text PRIMARY KEY, label text UNIQUE);
			CREATE TABLE uses (book_id text REFERENCES books (id));
			INSERT INTO books (id) VALUES ('main');
			INSERT INTO uses (book_id) VALUES ('main');`)
		Expect(err).ToNot(HaveOccurred())
	})

	exec := func(query string) error {
		_, err := db.ExecContext(context.Background(), query)
		Expect(err).To(HaveOccurred())

		return err
	}

	It("reports a duplicate key as a unique violation", func() {
		err := exec(`INSERT INTO books (id) VALUES ('main')`)

		Expect(postgres.IsUniqueViolation(err)).To(BeTrue())
		Expect(postgres.IsForeignKeyViolation(err)).To(BeFalse())
	})

	It("reports the index a duplicate key violated", func() {
		err := exec(`INSERT INTO books (id) VALUES ('main')`)

		Expect(postgres.IsUniqueViolationOn(err, "books_pkey")).To(BeTrue())
		Expect(postgres.IsUniqueViolationOn(err, "books_label_key")).To(BeFalse())
	})

	It("tells one unique index of a table apart from another", func() {
		_, err := db.ExecContext(context.Background(),
			`INSERT INTO books (id, label) VALUES ('first', 'shared')`)
		Expect(err).ToNot(HaveOccurred())

		err = exec(`INSERT INTO books (id, label) VALUES ('second', 'shared')`)

		Expect(postgres.IsUniqueViolation(err)).To(BeTrue())
		Expect(postgres.IsUniqueViolationOn(err, "books_label_key")).To(BeTrue())
		Expect(postgres.IsUniqueViolationOn(err, "books_pkey")).To(BeFalse())
	})

	It("reports a referenced row as a foreign key violation", func() {
		err := exec(`DELETE FROM books WHERE id = 'main'`)

		Expect(postgres.IsForeignKeyViolation(err)).To(BeTrue())
		Expect(postgres.IsUniqueViolation(err)).To(BeFalse())
	})

	It("reports a query that selected no row", func() {
		err := db.QueryRowContext(context.Background(),
			`SELECT id FROM books WHERE id = 'gone'`).Scan(new(string))
		Expect(err).To(HaveOccurred())

		Expect(postgres.IsNoRows(err)).To(BeTrue())
		Expect(postgres.IsUniqueViolation(err)).To(BeFalse())
	})

	It("reports neither for an unrelated database error", func() {
		err := exec(`SELECT * FROM nothing_here`)

		Expect(postgres.IsUniqueViolation(err)).To(BeFalse())
		Expect(postgres.IsForeignKeyViolation(err)).To(BeFalse())
	})

	It("reports neither for an error that is not from the driver", func() {
		Expect(postgres.IsUniqueViolation(errors.New("boom"))).To(BeFalse())
	})
})
