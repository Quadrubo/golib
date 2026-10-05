package grpcserver

import (
	"github.com/samber/do/v2"
	"google.golang.org/grpc"

	"github.com/quadrubo/golib/config"
)

type UnaryInterceptor func(do.Injector) (grpc.UnaryServerInterceptor, error)

type StreamInterceptor func(do.Injector) (grpc.StreamServerInterceptor, error)

type options struct {
	unary           []UnaryInterceptor
	stream          []StreamInterceptor
	maxReceiveBytes config.Bytes
	addr            string
}

type Option func(*options)

// WithUnaryInterceptors chains the interceptors in the order given, outermost
// first.
func WithUnaryInterceptors(interceptors ...UnaryInterceptor) Option {
	return func(o *options) { o.unary = append(o.unary, interceptors...) }
}

// WithMaxReceiveBytes replaces the default of max_receive_bytes. A configured
// max_receive_bytes still replaces this value. Zero keeps the default.
func WithMaxReceiveBytes(n config.Bytes) Option {
	return func(o *options) { o.maxReceiveBytes = n }
}

// WithAddr replaces the default of addr. A configured addr still replaces this
// value. An empty addr keeps the default.
func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

// WithStreamInterceptors chains the interceptors in the order given,
// outermost first.
func WithStreamInterceptors(interceptors ...StreamInterceptor) Option {
	return func(o *options) { o.stream = append(o.stream, interceptors...) }
}
