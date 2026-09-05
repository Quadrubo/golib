package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
)

const (
	configKey = "modules.postgres"

	driver = "pgx"
)

type Config struct {
	URL string `config:"url" validate:"required"`

	MaxOpenConns    int           `config:"max_open_conns" validate:"min=0"`
	MaxIdleConns    int           `config:"max_idle_conns" validate:"min=0"`
	ConnMaxLifetime time.Duration `config:"conn_max_lifetime" validate:"min=0"`
	ConnMaxIdleTime time.Duration `config:"conn_max_idle_time" validate:"min=0"`

	// ConnectTimeout bounds the ping at boot, which turns an unreachable
	// database into a failed boot instead of a hang.
	ConnectTimeout time.Duration `config:"connect_timeout" validate:"min=1s"`
}

// Module provides the *sql.DB pool a service queries through.
func Module() app.Module {
	return &module{}
}

type module struct {
	log *slog.Logger
	db  *sql.DB
}

var (
	_ app.Provider = (*module)(nil)
	_ app.Stopper  = (*module)(nil)
)

func (m *module) Name() string { return "postgres" }

func (m *module) Provide(ctx context.Context, i do.Injector) error {
	cfg, err := config.Load(i, m.Name(), configKey, Config{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnectTimeout:  10 * time.Second,
	})
	if err != nil {
		return err
	}

	m.log, err = logging.Component(i, m.Name())
	if err != nil {
		return err
	}

	db, err := sql.Open(driver, cfg.URL)
	if err != nil {
		return fmt.Errorf("postgres: failed to open the pool: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("postgres: failed to reach %s: %w", redact(cfg.URL), err)
	}

	m.db = db
	do.ProvideValue(i, db)

	m.log.Info("connected",
		slog.String("url", redact(cfg.URL)),
		slog.Int("max_open_conns", cfg.MaxOpenConns),
	)

	return nil
}

func (m *module) Stop(_ context.Context) error {
	if err := m.db.Close(); err != nil {
		return fmt.Errorf("postgres: failed to close the pool: %w", err)
	}

	m.log.Info("closed")

	return nil
}

// redact keeps the password out of logs and errors.
func redact(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return ""
	}

	return u.Redacted()
}
