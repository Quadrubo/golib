package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
)

const configKey = "modules.grpcserver"

type Config struct {
	Addr string `config:"addr" validate:"required"`

	// GracePeriod bounds GracefulStop, which waits on in-flight calls forever.
	// It has to be under the drain budget the app publishes.
	GracePeriod time.Duration `config:"grace_period" validate:"required"`

	Reflection bool `config:"reflection"`

	Health bool `config:"health"`

	// MaxReceiveBytes caps one received message, and takes a binary unit such
	// as "16MiB". The server answers a larger message with ResourceExhausted.
	MaxReceiveBytes config.Bytes `config:"max_receive_bytes" validate:"min=1"`
}

// Module provides the *grpc.Server a service registers on, and runs it.
func Module(opts ...Option) app.Module {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	return &module{opts: o}
}

type module struct {
	cfg    Config
	opts   options
	log    *slog.Logger
	server *grpc.Server
	health *health.Server
	lis    net.Listener
}

type Addr string

var installGRPCLog sync.Once

var (
	_ app.Provider = (*module)(nil)
	_ app.Runner   = (*module)(nil)
	_ app.Stopper  = (*module)(nil)
)

func (m *module) Name() string { return "grpcserver" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	var err error

	m.cfg, err = config.Load(i, m.Name(), configKey, Config{
		Addr:            ":50051",
		GracePeriod:     5 * time.Second,
		Reflection:      true,
		Health:          true,
		MaxReceiveBytes: 4 * 1024 * 1024,
	})
	if err != nil {
		return err
	}

	budget, err := do.Invoke[app.DrainBudget](i)
	if err != nil {
		return fmt.Errorf("grpcserver: failed to invoke the drain budget: %w", err)
	}

	// Draining for the whole budget means Run returns as the deadline lands,
	// which the app cannot tell from a module that never came back.
	if m.cfg.GracePeriod >= time.Duration(budget) {
		return fmt.Errorf("grpcserver: grace_period of %s has to be less than the %s drain budget",
			m.cfg.GracePeriod, budget)
	}

	m.log, err = logging.Component(i, m.Name())
	if err != nil {
		return err
	}

	log, err := do.Invoke[*slog.Logger](i)
	if err != nil {
		return fmt.Errorf("grpcserver: failed to invoke the logger: %w", err)
	}

	// grpclog writes package globals its own goroutines read, so a second app
	// in one process leaves the first one's logger installed.
	installGRPCLog.Do(func() { grpclog.SetLoggerV2(GRPCLogger(log)) })

	unary, err := m.chain(i)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer(
		grpc.ChainUnaryInterceptor(unary...),
		grpc.MaxRecvMsgSize(int(m.cfg.MaxReceiveBytes)),
	)
	do.ProvideValue(i, m.server)

	if m.cfg.Health {
		m.health = health.NewServer()
		healthpb.RegisterHealthServer(m.server, m.health)
		do.ProvideValue(i, m.health)
	}

	m.lis, err = net.Listen("tcp", m.cfg.Addr)
	if err != nil {
		return fmt.Errorf("grpcserver: failed to listen on %s: %w", m.cfg.Addr, err)
	}
	do.ProvideValue(i, Addr(m.lis.Addr().String()))

	return nil
}

func (m *module) Stop(context.Context) error {
	_ = m.lis.Close()

	return nil
}

func (m *module) chain(i do.Injector) ([]grpc.UnaryServerInterceptor, error) {
	chain := make([]grpc.UnaryServerInterceptor, 0, len(m.opts.unary))
	for _, build := range m.opts.unary {
		interceptor, err := build(i)
		if err != nil {
			return nil, fmt.Errorf("grpcserver: failed to build an interceptor: %w", err)
		}

		chain = append(chain, interceptor)
	}

	return chain, nil
}

func (m *module) Run(ctx context.Context) error {
	if m.cfg.Reflection {
		reflection.Register(m.server)
	}

	unregister := context.AfterFunc(ctx, m.stop)
	defer unregister()

	m.log.Info("serving",
		slog.String("addr", m.lis.Addr().String()), slog.Bool("reflection", m.cfg.Reflection))

	if err := m.server.Serve(m.lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("grpcserver: failed to serve: %w", err)
	}

	return nil
}

// stop lets in-flight calls finish within the grace period, then drops them.
func (m *module) stop() {
	m.log.Info("stopping", slog.Duration("grace_period", m.cfg.GracePeriod))

	if m.health != nil {
		m.health.Shutdown()
	}

	stopped := make(chan struct{})
	go func() {
		m.server.GracefulStop()
		close(stopped)
	}()

	timer := time.NewTimer(m.cfg.GracePeriod)
	defer timer.Stop()

	select {
	case <-stopped:
		m.log.Info("stopped")
	case <-timer.C:
		m.log.Warn("dropping calls that outlasted the grace period")
		m.server.Stop()
	}
}
