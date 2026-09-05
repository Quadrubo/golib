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
	log, err := logging.Component(i, component)
	if err != nil {
		return nil, err
	}

	domain, err := do.Invoke[grpcerr.Domain](i)
	if err != nil {
		return nil, fmt.Errorf("recovery: failed to invoke the domain: %w", err)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}

			log.ErrorContext(ctx, "call panicked",
				slog.String("method", info.FullMethod),
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)

			err = grpcerr.Internal("PANIC", "internal error").WithDomain(domain)
		}()

		return next(ctx, req)
	}, nil
}
