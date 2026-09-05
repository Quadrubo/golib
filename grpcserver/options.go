package grpcserver

import (
	"github.com/samber/do/v2"
	"google.golang.org/grpc"
)

type UnaryInterceptor func(do.Injector) (grpc.UnaryServerInterceptor, error)

type options struct {
	unary []UnaryInterceptor
}

type Option func(*options)

// WithUnaryInterceptors chains the interceptors in the order given, outermost
// first.
func WithUnaryInterceptors(interceptors ...UnaryInterceptor) Option {
	return func(o *options) { o.unary = append(o.unary, interceptors...) }
}
