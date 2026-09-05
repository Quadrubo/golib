package errmap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"

	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/logging"
)

const component = "errmap"

// Unary returns the interceptor that stamps the domain onto a grpcerr.Error a
// handler returned, and replaces anything it cannot classify with Internal.
func Unary(i do.Injector) (grpc.UnaryServerInterceptor, error) {
	log, err := logging.Component(i, component)
	if err != nil {
		return nil, err
	}

	domain, err := do.Invoke[grpcerr.Domain](i)
	if err != nil {
		return nil, fmt.Errorf("errmap: failed to invoke the domain: %w", err)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (any, error) {
		resp, err := next(ctx, req)
		if err == nil {
			return resp, nil
		}

		// Returning the wrapper would let status.FromError rebuild the message
		// from the whole chain.
		var gerr *grpcerr.Error
		if errors.As(err, &gerr) {
			// The wrapper never reaches the caller, so the log is the only place
			// left for the context a handler added.
			//nolint:errorlint // compares identity, a match means nothing wrapped gerr
			if err != error(gerr) {
				log.DebugContext(ctx, "handled error",
					slog.String("method", info.FullMethod),
					slog.Any("error", err),
				)
			}

			return nil, gerr.WithDomain(domain)
		}

		if errors.Is(err, context.Canceled) {
			return nil, grpcerr.Canceled("CALL_CANCELED", "the call was canceled").WithDomain(domain)
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return nil, grpcerr.DeadlineExceeded("CALL_EXPIRED", "the call ran out of time").
				WithDomain(domain)
		}

		// The original error reaches the log before a generic one reaches the client.
		log.ErrorContext(ctx, "unhandled error",
			slog.String("method", info.FullMethod),
			slog.Any("error", err),
		)

		return nil, grpcerr.Internal("INTERNAL", "internal error").WithDomain(domain)
	}, nil
}
