package grpcerr

import (
	"context"

	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
)

const configKey = "modules.grpcerr"

// Domain names the service in the ErrorInfo AIP-193 requires on every error.
type Domain string

type Config struct {
	Domain string `config:"domain" validate:"required"`
}

// Module provides the Domain every error carries on its ErrorInfo, read from
// modules.grpcerr.
func Module() app.Module {
	return &module{}
}

type module struct{}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "grpcerr" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	cfg, err := config.Load(i, m.Name(), configKey, Config{})
	if err != nil {
		return err
	}

	do.ProvideValue(i, Domain(cfg.Domain))

	return nil
}
