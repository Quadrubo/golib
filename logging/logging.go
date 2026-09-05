package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
)

const configKey = "modules.logging"

// ComponentKey tags every record with the part of the system that wrote it.
const ComponentKey = "component"

const (
	FormatText = "text"
	FormatJSON = "json"
)

type Config struct {
	Level  slog.Level `config:"level"`
	Format string     `config:"format" validate:"required,oneof=text json"`
}

// Module provides a *slog.Logger and, unless WithoutDefault says otherwise,
// makes it the process default.
func Module(opts ...Option) app.Module {
	o := options{writer: os.Stdout, setDefault: true}
	for _, opt := range opts {
		opt(&o)
	}

	return &module{opts: o}
}

type module struct {
	opts options
}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "logging" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	cfg, err := config.Load(i, m.Name(), configKey,
		Config{Level: slog.LevelInfo, Format: FormatText})
	if err != nil {
		return err
	}

	logger := newLogger(cfg, m.opts.writer)
	do.ProvideValue(i, logger)

	if m.opts.setDefault {
		// Without SetDefault, anything reaching for the package-level slog API,
		// including dependencies, keeps writing to stderr at info and skips
		// this config.
		slog.SetDefault(logger)
	}

	return nil
}

// Component returns the logger tagged with the component writing through it.
// component also names the module in the error an absent logger produces.
func Component(i do.Injector, component string) (*slog.Logger, error) {
	log, err := do.Invoke[*slog.Logger](i)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to invoke the logger: %w", component, err)
	}

	return log.With(slog.String(ComponentKey, component)), nil
}

func newLogger(cfg Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.Level}

	var handler slog.Handler = slog.NewTextHandler(w, opts)
	if cfg.Format == FormatJSON {
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}
