package bun

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/samber/do/v2"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/quadrubo/golib/app"
)

// Module provides a *bun.DB over the pool postgres owns.
func Module() app.Module {
	return &module{}
}

// module has no Stop, since bun.DB.Close would close that pool underneath its
// owner.
type module struct{}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "bun" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	db, err := do.Invoke[*sql.DB](i)
	if err != nil {
		return fmt.Errorf("bun: failed to invoke the database: %w", err)
	}

	do.ProvideValue(i, bun.NewDB(db, pgdialect.New()))

	return nil
}
