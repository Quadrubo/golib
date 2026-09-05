package testkit

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/do/v2"
	"golang.org/x/sync/errgroup"
)

// Dependency is infrastructure the service under test needs. Start brings it
// up and puts what specs arrange with, such as a database connection, into
// the injector.
type Dependency interface {
	Start(ctx context.Context, injector do.Injector) error
	Stop(ctx context.Context) error
}

// SettingsProvider yields the settings a started dependency hands the service.
type SettingsProvider interface {
	Settings() map[string]any
}

func startAll(ctx context.Context, deps []Dependency, injector do.Injector) error {
	g, gCtx := errgroup.WithContext(ctx)
	for _, dep := range deps {
		g.Go(func() error {
			if err := dep.Start(gCtx, injector); err != nil {
				return fmt.Errorf("testkit: dependency %T failed to start: %w", dep, err)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return errors.Join(err, stopAll(context.WithoutCancel(ctx), deps))
	}

	return nil
}

func stopAll(ctx context.Context, deps []Dependency) error {
	var errs []error
	for _, dep := range deps {
		if err := dep.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("testkit: dependency %T failed to stop: %w", dep, err))
		}
	}

	return errors.Join(errs...)
}

// merge overlays override onto base and descends into maps both hold.
func merge(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for key, value := range base {
		out[key] = value
	}

	for key, value := range override {
		if inner, ok := value.(map[string]any); ok {
			if outer, ok := out[key].(map[string]any); ok {
				out[key] = merge(outer, inner)
				continue
			}
		}
		out[key] = value
	}

	return out
}
