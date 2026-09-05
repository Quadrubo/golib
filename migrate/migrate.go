package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
)

const configKey = "modules.migrate"

type Config struct {
	LockRetryInterval time.Duration `config:"lock_retry_interval" validate:"min=1s"`
	LockRetries       uint64        `config:"lock_retries" validate:"min=1"`
}

// Module applies the goose migrations in fsys, before anything queries.
func Module(fsys fs.FS) app.Module {
	return &module{fsys: fsys}
}

type module struct {
	fsys fs.FS
	log  *slog.Logger
}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "migrate" }

func (m *module) Provide(ctx context.Context, i do.Injector) error {
	cfg, err := config.Load(i, m.Name(), configKey,
		Config{LockRetryInterval: time.Second, LockRetries: 300})
	if err != nil {
		return err
	}

	db, err := do.Invoke[*sql.DB](i)
	if err != nil {
		return fmt.Errorf("migrate: failed to invoke the database: %w", err)
	}

	m.log, err = logging.Component(i, m.Name())
	if err != nil {
		return err
	}

	locker, err := lock.NewPostgresSessionLocker(
		lock.WithLockTimeout(uint64(cfg.LockRetryInterval.Seconds()), cfg.LockRetries))
	if err != nil {
		return fmt.Errorf("migrate: failed to build the migration lock: %w", err)
	}

	// Nothing closes the provider, since goose.Provider.Close would close the
	// pool underneath postgres.
	provider, err := goose.NewProvider(goose.DialectPostgres, db, m.fsys,
		goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("migrate: failed to read the migrations: %w", err)
	}

	// Logged before the lock is taken, so a timeout later has the wait it was
	// given in the log above it.
	m.log.Info("migrating",
		slog.Duration("lock_wait", cfg.LockRetryInterval*time.Duration(cfg.LockRetries)))

	return m.apply(ctx, provider)
}

func (m *module) apply(ctx context.Context, provider *goose.Provider) error {
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: failed to apply the migrations: %w", err)
	}

	if len(results) == 0 {
		m.log.Info("schema is current")
		return nil
	}

	for _, result := range results {
		m.log.Info("migrated",
			slog.Int64("version", result.Source.Version),
			slog.String("migration", result.Source.Path),
			slog.Duration("took", result.Duration),
		)
	}

	return nil
}
