package authentication

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/quadrubo/golib/grpcerr"
)

// Authenticator checks the bearer credential of a call and returns the context
// the handler runs under, or an error that fails the call.
type Authenticator interface {
	Authenticate(ctx context.Context, credential string) (context.Context, error)
}

// Anonymous lists the services and the full methods that run without a
// credential.
type Anonymous struct {
	Services []string
	Methods  []string
}

var (
	errMissing   = grpcerr.Unauthenticated("CREDENTIAL_MISSING", "the call carries no bearer credential")
	errMalformed = grpcerr.Unauthenticated("CREDENTIAL_MALFORMED",
		"the authorization metadata is not a single bearer credential")
)

// Unary returns the interceptor that runs a unary call under the context the
// Authenticator returns, unless Anonymous lists the call.
func Unary(i do.Injector) (grpc.UnaryServerInterceptor, error) {
	g, err := newGate(i)
	if err != nil {
		return nil, err
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (any, error) {
		ctx, err := g.admit(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}

		return next(ctx, req)
	}, nil
}

// Stream returns the interceptor that runs a stream call under the context the
// Authenticator returns, unless Anonymous lists the call.
func Stream(i do.Injector) (grpc.StreamServerInterceptor, error) {
	g, err := newGate(i)
	if err != nil {
		return nil, err
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		next grpc.StreamHandler,
	) error {
		ctx, err := g.admit(ss.Context(), info.FullMethod)
		if err != nil {
			return err
		}

		return next(srv, &stream{ServerStream: ss, ctx: ctx})
	}, nil
}

type stream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stream) Context() context.Context { return s.ctx }

type gate struct {
	authenticator Authenticator
	services      map[string]bool
	methods       map[string]bool
}

func newGate(i do.Injector) (*gate, error) {
	authenticator, err := do.Invoke[Authenticator](i)
	if err != nil {
		return nil, fmt.Errorf("authentication: failed to invoke the authenticator: %w", err)
	}

	anonymous, err := do.Invoke[Anonymous](i)
	if err != nil {
		return nil, fmt.Errorf("authentication: failed to invoke the anonymous calls: %w", err)
	}

	g := &gate{
		authenticator: authenticator,
		services:      make(map[string]bool, len(anonymous.Services)),
		methods:       make(map[string]bool, len(anonymous.Methods)),
	}
	for _, service := range anonymous.Services {
		g.services[service] = true
	}
	for _, method := range anonymous.Methods {
		g.methods[method] = true
	}

	return g, nil
}

func (g *gate) admit(ctx context.Context, method string) (context.Context, error) {
	if g.methods[method] || g.services[serviceOf(method)] {
		return ctx, nil
	}

	credential, err := bearer(ctx)
	if err != nil {
		return nil, err
	}

	authenticated, err := g.authenticator.Authenticate(ctx, credential)
	if err != nil {
		return nil, err
	}

	if authenticated == nil {
		return nil, errors.New("authentication: the authenticator returned no context")
	}

	return authenticated, nil
}

func serviceOf(method string) string {
	trimmed := strings.TrimPrefix(method, "/")
	if last := strings.LastIndex(trimmed, "/"); last >= 0 {
		return trimmed[:last]
	}

	return trimmed
}

func bearer(ctx context.Context) (string, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) == 0 {
		return "", errMissing
	}

	if len(values) > 1 {
		return "", errMalformed
	}

	scheme, credential, ok := strings.Cut(values[0], " ")
	// RFC 6750 section 2.1 allows one or more spaces after the scheme.
	credential = strings.TrimLeft(credential, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || credential == "" || strings.TrimSpace(credential) != credential {
		return "", errMalformed
	}

	return credential, nil
}
