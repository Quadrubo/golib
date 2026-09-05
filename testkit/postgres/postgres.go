package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/samber/do/v2"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/quadrubo/golib/testkit"
)

type Postgres struct {
	opts      options
	container *tcpostgres.PostgresContainer
	db        *bun.DB
	url       string
}

func New(opts ...Option) *Postgres {
	cfg := options{
		image:    "postgres:17-alpine",
		database: "test",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Postgres{opts: cfg}
}

var (
	_ testkit.Dependency       = (*Postgres)(nil)
	_ testkit.SettingsProvider = (*Postgres)(nil)
)

func (p *Postgres) Start(ctx context.Context, injector do.Injector) error {
	container, err := tcpostgres.Run(ctx, p.opts.image,
		tcpostgres.WithDatabase(p.opts.database),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return fmt.Errorf("testkit/postgres: failed to start the container: %w", err)
	}
	p.container = container

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("testkit/postgres: failed to read the connection string: %w", err)
	}
	p.url = url

	sqldb, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("testkit/postgres: failed to open the suite connection: %w", err)
	}
	p.db = bun.NewDB(sqldb, pgdialect.New())
	do.ProvideValue(injector, p.db)

	return nil
}

func (p *Postgres) Stop(ctx context.Context) error {
	var errs []error
	if p.db != nil {
		errs = append(errs, p.db.Close())
	}
	if p.container != nil {
		errs = append(errs, p.container.Terminate(ctx))
	}

	return errors.Join(errs...)
}

func (p *Postgres) Settings() map[string]any {
	return map[string]any{
		"modules": map[string]any{
			"postgres": map[string]any{"url": p.url},
		},
	}
}

func (p *Postgres) URL() string { return p.url }

// CreateDatabase adds a database beside the one the container created.
func (p *Postgres) CreateDatabase(ctx context.Context, name string) (string, error) {
	// CREATE DATABASE takes no bind parameter, so bun quotes the name instead.
	if _, err := p.db.ExecContext(ctx, "CREATE DATABASE ?", bun.Ident(name)); err != nil {
		return "", fmt.Errorf("testkit/postgres: failed to create the database %q: %w", name, err)
	}

	u, err := url.Parse(p.url)
	if err != nil {
		return "", fmt.Errorf("testkit/postgres: failed to parse the url: %w", err)
	}
	u.Path = "/" + name

	return u.String(), nil
}
