package migrate_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pressly/goose/v3/lock"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
	"github.com/quadrubo/golib/migrate"
	"github.com/quadrubo/golib/postgres"
	tkpostgres "github.com/quadrubo/golib/testkit/postgres"
)

func TestMigrate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migrate Suite")
}

var pg *tkpostgres.Postgres

var _ = BeforeSuite(func() {
	pg = tkpostgres.New()

	Expect(pg.Start(context.Background(), do.New())).To(Succeed())
	DeferCleanup(func() { Expect(pg.Stop(context.Background())).To(Succeed()) })
})

// attempt carries a boot a spec started in the background, so it can assert on
// whether the boot came back before the lock was released.
type attempt struct {
	app *app.App
	err error
}

var _ = Describe("Module", func() {
	rooms := fstest.MapFS{"00001_rooms.sql": &fstest.MapFile{Data: []byte(`
-- +goose Up
CREATE TABLE rooms (id text PRIMARY KEY);

-- +goose Down
DROP TABLE rooms;
`)}}

	// The second statement fails, so a migration that is not one transaction
	// leaves the rooms table behind.
	halfway := fstest.MapFS{"00001_rooms.sql": &fstest.MapFile{Data: []byte(`
-- +goose Up
CREATE TABLE rooms (id text PRIMARY KEY);
CREATE TABLE;
`)}}

	databases := 0

	// Every spec migrates, so each one gets a database the others cannot see.
	freshSettings := func() map[string]any {
		GinkgoHelper()

		databases++
		dsn, err := pg.CreateDatabase(context.Background(), fmt.Sprintf("fresh_%d", databases))
		Expect(err).ToNot(HaveOccurred())

		return map[string]any{"modules.postgres.url": dsn}
	}

	start := func(settings map[string]any, fsys fs.FS) (*app.App, error) {
		return app.New(context.Background(), []app.Module{
			config.StaticModule(settings),
			logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
			postgres.Module(),
			migrate.Module(fsys),
		})
	}

	boot := func(settings map[string]any, fsys fs.FS) *app.App {
		GinkgoHelper()

		a, err := start(settings, fsys)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(a.Shutdown(context.Background())).To(Succeed()) })

		return a
	}

	// connect reaches the database without booting, for the specs that assert on
	// a schema no app is holding open.
	connect := func(settings map[string]any) *sql.DB {
		GinkgoHelper()

		db, err := sql.Open("pgx", settings["modules.postgres.url"].(string))
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })

		return db
	}

	countIn := func(a *app.App, table string) int {
		GinkgoHelper()

		var count int
		Expect(do.MustInvoke[*sql.DB](a.Injector()).
			QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).
			Scan(&count)).To(Succeed())

		return count
	}

	// holdLock takes the lock goose migrates under, the way a peer that is still
	// migrating holds it, and hands back the release.
	holdLock := func(settings map[string]any) func() {
		GinkgoHelper()

		conn, err := connect(settings).Conn(context.Background())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(conn.Close()).To(Succeed()) })

		_, err = conn.ExecContext(context.Background(),
			"SELECT pg_advisory_lock($1)", lock.DefaultLockID)
		Expect(err).ToNot(HaveOccurred())

		return func() {
			GinkgoHelper()

			_, err := conn.ExecContext(context.Background(),
				"SELECT pg_advisory_unlock($1)", lock.DefaultLockID)
			Expect(err).ToNot(HaveOccurred())
		}
	}

	It("applies the migrations it was given", func() {
		a := boot(freshSettings(), rooms)

		Expect(countIn(a, "rooms")).To(BeZero())
	})

	It("leaves a database that is already current alone", func() {
		settings := freshSettings()
		Expect(boot(settings, rooms).Shutdown(context.Background())).To(Succeed())

		// The migration creates rooms outright, so applying it twice would fail.
		Expect(countIn(boot(settings, rooms), "rooms")).To(BeZero())
	})

	It("rolls back a migration that failed halfway", func() {
		settings := freshSettings()

		_, err := start(settings, halfway)
		Expect(err).To(HaveOccurred())

		var table *string
		Expect(connect(settings).
			QueryRowContext(context.Background(), "SELECT to_regclass('rooms')::text").
			Scan(&table)).To(Succeed())

		Expect(table).To(BeNil())
	})

	It("retries a migration that failed instead of recording it", func() {
		settings := freshSettings()

		_, err := start(settings, halfway)
		Expect(err).To(HaveOccurred())

		Expect(countIn(boot(settings, rooms), "rooms")).To(BeZero())
	})

	It("refuses to boot when a migration fails", func() {
		_, err := start(freshSettings(), halfway)

		Expect(err).To(MatchError(ContainSubstring("migrate: failed to apply the migrations")))
	})

	It("waits for a lock another run is still holding", func() {
		settings := freshSettings()
		release := holdLock(settings)

		attempts := make(chan attempt, 1)
		go func() {
			defer GinkgoRecover()
			a, err := start(settings, rooms)
			attempts <- attempt{app: a, err: err}
		}()

		Consistently(attempts, 500*time.Millisecond).ShouldNot(Receive())
		release()

		var waited attempt
		Eventually(attempts, 10*time.Second).Should(Receive(&waited))
		Expect(waited.err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(waited.app.Shutdown(context.Background())).To(Succeed()) })
	})

	It("gives up on a lock nothing releases", func() {
		settings := freshSettings()
		settings["modules.migrate.lock_retries"] = 1
		holdLock(settings)

		attempts := make(chan attempt, 1)
		go func() {
			defer GinkgoRecover()
			a, err := start(settings, rooms)
			attempts <- attempt{app: a, err: err}
		}()

		// A boot that ignored lock_retries would retry out the 300 retry default.
		var gave attempt
		Eventually(attempts, 5*time.Second).Should(Receive(&gave))
		Expect(gave.err).To(MatchError(ContainSubstring("failed to acquire lock")))
	})

	It("refuses a filesystem that holds no migrations", func() {
		_, err := start(freshSettings(), fstest.MapFS{})

		Expect(err).To(MatchError(ContainSubstring("migrate: failed to read the migrations")))
	})

	It("does not shut down, since the pool belongs to postgres", func() {
		_, ok := migrate.Module(rooms).(app.Stopper)

		Expect(ok).To(BeFalse())
	})

	It("reports a module list that left out the database", func() {
		_, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{}),
			logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
			migrate.Module(rooms),
		})

		Expect(err).To(MatchError(ContainSubstring("migrate: failed to invoke the database")))
	})
})
