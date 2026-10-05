package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"

	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/logging"
)

const component = "recovery"

// Unary returns the interceptor that turns a panic in a handler into an
// Internal error, logging the panic and its stack.
func Unary(i do.Injector) (grpc.UnaryServerInterceptor, error) {
	g, err := newGuard(i)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = g.failed(ctx, info.FullMethod, r)
			}
		}()

		return next(ctx, req)
	}, nil
}

// Stream returns the interceptor that turns a panic in a stream handler into
// an Internal error, logging the panic and its stack.
func Stream(i do.Injector) (grpc.StreamServerInterceptor, error) {
	g, err := newGuard(i)
	if err != nil {
		return nil, err
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		next grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = g.failed(ss.Context(), info.FullMethod, r)
			}
		}()

		return next(srv, ss)
	}, nil
}

type guard struct {
	log    *slog.Logger
	domain grpcerr.Domain
}

func newGuard(i do.Injector) (*guard, error) {
	log, err := logging.Component(i, component)
	if err != nil {
		return nil, err
	}

	domain, err := do.Invoke[grpcerr.Domain](i)
	if err != nil {
		return nil, fmt.Errorf("recovery: failed to invoke the domain: %w", err)
	}

	return &guard{log: log, domain: domain}, nil
}

func (g *guard) failed(ctx context.Context, method string, panicked any) error {
	g.log.ErrorContext(ctx, "call panicked",
		slog.String("method", method),
		slog.Any("panic", panicked),
		slog.String("stack", string(debug.Stack())),
	)

	return grpcerr.Internal("PANIC", "internal error").WithDomain(g.domain)
}
