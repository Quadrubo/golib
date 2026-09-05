package testkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/grpcserver"
)

const (
	readyTimeout = 30 * time.Second
	stopTimeout  = 15 * time.Second

	// listenAddr leaves the port to the operating system, so parallel suites
	// never pick the same one.
	listenAddr = "127.0.0.1:0"
)

type Options struct {
	// Modules has the shape of a service's own module list constructor.
	Modules func(conf app.Module) []app.Module

	// Dependencies come up before the service and merge their settings into
	// its config.
	Dependencies []Dependency

	// Settings has the shape of the service's config file. It overrides what
	// Boot and the dependencies set.
	Settings map[string]any
}

type Suite struct {
	app      *app.App
	opts     Options
	deps     []Dependency
	injector do.Injector
	conn     *grpc.ClientConn
	addr     string

	cancel context.CancelFunc
	done   chan struct{}
	runErr error
}

// Boot starts the dependencies, then runs a service in this process and waits
// for its health service to answer. Specs reach it over a real socket.
func Boot(ctx context.Context, opts Options) (*Suite, error) {
	injector := do.New()

	if err := startAll(ctx, opts.Dependencies, injector); err != nil {
		return nil, err
	}

	s, err := boot(ctx, opts, injector, opts.Dependencies, nil)
	if err != nil {
		return nil, errors.Join(err, stopAll(context.WithoutCancel(ctx), opts.Dependencies))
	}

	return s, nil
}

// Rebooted runs a second service on the dependencies of the suite, with the
// overrides on the settings the suite booted with.
func (s *Suite) Rebooted(ctx context.Context, overrides map[string]any) (*Suite, error) {
	return boot(ctx, s.opts, s.injector, nil, overrides)
}

// boot runs a service and waits for its health service, and Stop takes down
// the dependencies in owned.
func boot(
	ctx context.Context,
	opts Options,
	injector do.Injector,
	owned []Dependency,
	overrides map[string]any,
) (*Suite, error) {
	a, err := app.New(ctx, opts.Modules(config.StaticModule(settingsFor(opts, overrides))))
	if err != nil {
		return nil, err
	}

	addr, err := do.Invoke[grpcserver.Addr](a.Injector())
	if err != nil {
		return nil, fmt.Errorf("testkit: failed to read the server address: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &Suite{
		app:      a,
		opts:     opts,
		deps:     owned,
		injector: injector,
		addr:     string(addr),
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	go func() {
		s.runErr = a.Run(runCtx)
		close(s.done)
	}()

	s.conn, err = grpc.NewClient(s.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("testkit: failed to dial %s: %w", s.addr, err), s.stopService())
	}

	if err := s.waitReady(ctx); err != nil {
		return nil, errors.Join(err, s.stopService())
	}

	return s, nil
}

// settingsFor layers the address, the dependencies, the options and the
// overrides, in that order.
func settingsFor(opts Options, overrides map[string]any) map[string]any {
	settings := map[string]any{
		"modules": map[string]any{
			"grpcserver": map[string]any{"addr": listenAddr},
		},
	}

	for _, dep := range opts.Dependencies {
		provider, ok := dep.(SettingsProvider)
		if !ok {
			continue
		}
		settings = merge(settings, provider.Settings())
	}

	settings = merge(settings, opts.Settings)

	return merge(settings, overrides)
}

func (s *Suite) Addr() string { return s.addr }

func (s *Suite) Conn() *grpc.ClientConn { return s.conn }

// Injector holds what the dependencies provided for arranging state. The
// service's own injector stays out of reach.
func (s *Suite) Injector() do.Injector { return s.injector }

func (s *Suite) Stop() error {
	return errors.Join(s.stopService(), stopAll(context.Background(), s.deps))
}

// stopService stops the service and leaves the dependencies up.
func (s *Suite) stopService() error {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.cancel()

	select {
	case <-s.done:
		return s.runErr
	case <-time.After(stopTimeout):
		return fmt.Errorf("testkit: the service did not stop within %s", stopTimeout)
	}
}

func (s *Suite) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	client := healthpb.NewHealthClient(s.conn)

	for {
		select {
		case <-s.done:
			return fmt.Errorf("testkit: the service stopped before it was ready: %w", s.runErr)
		case <-ctx.Done():
			return fmt.Errorf("testkit: the service was not ready after %s", readyTimeout)
		default:
		}

		checkCtx, checkCancel := context.WithTimeout(ctx, time.Second)
		_, err := client.Check(checkCtx, &healthpb.HealthCheckRequest{})
		checkCancel()

		if err == nil {
			return nil
		}

		time.Sleep(20 * time.Millisecond)
	}
}
