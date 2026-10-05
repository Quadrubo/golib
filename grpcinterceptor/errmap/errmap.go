package errmap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/logging"
)

const component = "errmap"

// Unary returns the interceptor that stamps the domain onto a grpcerr.Error a
// handler returned, and replaces anything it cannot classify with Internal. A
// bare status of health or reflection passes unchanged.
func Unary(i do.Injector) (grpc.UnaryServerInterceptor, error) {
	m, err := newMapper(i)
	if err != nil {
		return nil, err
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

		return nil, m.mapped(ctx, info.FullMethod, err)
	}, nil
}

// Stream returns the interceptor that stamps the domain onto a grpcerr.Error
// a stream handler returned, and replaces anything it cannot classify with
// Internal. A bare status of health or reflection passes unchanged.
func Stream(i do.Injector) (grpc.StreamServerInterceptor, error) {
	m, err := newMapper(i)
	if err != nil {
		return nil, err
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		next grpc.StreamHandler,
	) error {
		err := next(srv, ss)
		if err == nil {
			return nil
		}

		return m.mapped(ss.Context(), info.FullMethod, err)
	}, nil
}

type mapper struct {
	log    *slog.Logger
	domain grpcerr.Domain
}

func newMapper(i do.Injector) (*mapper, error) {
	log, err := logging.Component(i, component)
	if err != nil {
		return nil, err
	}

	domain, err := do.Invoke[grpcerr.Domain](i)
	if err != nil {
		return nil, fmt.Errorf("errmap: failed to invoke the domain: %w", err)
	}

	return &mapper{log: log, domain: domain}, nil
}

func (m *mapper) mapped(ctx context.Context, method string, err error) error {
	// Returning the wrapper would let status.FromError rebuild the message
	// from the whole chain.
	var gerr *grpcerr.Error
	if errors.As(err, &gerr) {
		// The log keeps the context that a handler wrapped around gerr.
		//nolint:errorlint // compares identity, a match means nothing wrapped gerr
		if err != error(gerr) {
			m.log.DebugContext(ctx, "handled error",
				slog.String("method", method),
				slog.Any("error", err),
			)
		}

		return gerr.WithDomain(m.domain)
	}

	// The stream handlers of grpc answer a closed stream with a bare Canceled or DeadlineExceeded status.
	code := status.Code(err)

	// database/sql answers a transaction that a cancelled context rolled back with ErrTxDone.
	ended := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, sql.ErrTxDone) || code == codes.Canceled || code == codes.DeadlineExceeded

	// The context of the call decides the code, and not the error of an outbound call or of a library.
	switch {
	case ended && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return grpcerr.DeadlineExceeded("CALL_EXPIRED", "the call ran out of time").WithDomain(m.domain)
	case ended && ctx.Err() != nil:
		return grpcerr.Canceled("CALL_CANCELED", "the call was canceled").WithDomain(m.domain)
	}

	// Health and reflection define their own bare status codes, such as NOT_FOUND for an unknown service.
	if _, bare := status.FromError(err); bare && grpcService(method) {
		return err
	}

	// The original error reaches the log before a generic one reaches the client.
	m.log.ErrorContext(ctx, "unhandled error",
		slog.String("method", method),
		slog.Any("error", err),
	)

	return grpcerr.Internal("INTERNAL", "internal error").WithDomain(m.domain)
}

var grpcServices = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.v1.ServerReflection/",
	"/grpc.reflection.v1alpha.ServerReflection/",
}

func grpcService(method string) bool {
	return slices.ContainsFunc(grpcServices, func(service string) bool { return strings.HasPrefix(method, service) })
}
