package recovery_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/grpcinterceptor/recovery"
	"github.com/quadrubo/golib/logging"
)

const domain grpcerr.Domain = "example.test"

func TestRecovery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Recovery Suite")
}

var _ = Describe("Unary", func() {
	var (
		logs *bytes.Buffer
		it   grpc.UnaryServerInterceptor
	)

	BeforeEach(func() {
		logs = &bytes.Buffer{}

		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{"modules.grpcerr.domain": string(domain)}),
			logging.Module(logging.WithWriter(logs), logging.WithoutDefault()),
			grpcerr.Module(),
		})
		Expect(err).ToNot(HaveOccurred())

		it, err = recovery.Unary(a.Injector())
		Expect(err).ToNot(HaveOccurred())
	})

	call := func(handler grpc.UnaryHandler) (any, error) {
		return it(context.Background(), "req",
			&grpc.UnaryServerInfo{FullMethod: "/spec.v1.Books/GetBook"}, handler)
	}

	It("returns the response when the handler returned", func() {
		resp, err := call(func(context.Context, any) (any, error) { return "resp", nil })

		Expect(err).ToNot(HaveOccurred())
		Expect(resp).To(Equal("resp"))
	})

	It("passes a handler error through unchanged", func() {
		boom := errors.New("boom")

		_, err := call(func(context.Context, any) (any, error) { return nil, boom })

		Expect(err).To(MatchError(boom))
	})

	It("turns a panic into an Internal error", func() {
		resp, err := call(func(context.Context, any) (any, error) {
			panic("nil map write")
		})

		s, ok := status.FromError(err)
		Expect(ok).To(BeTrue())
		Expect(s.Code()).To(Equal(codes.Internal))
		Expect(resp).To(BeNil())
	})

	It("builds the error it returns under its own domain", func() {
		_, err := call(func(context.Context, any) (any, error) { panic("nil map write") })

		s, _ := status.FromError(err)
		Expect(s.Details()).To(ContainElement(HaveField("Domain", string(domain))))
	})

	It("keeps the panic value out of the message", func() {
		_, err := call(func(context.Context, any) (any, error) {
			panic("connection string postgres://admin:hunter2@db")
		})

		s, _ := status.FromError(err)
		Expect(s.Message()).To(Equal("internal error"))
		Expect(s.Message()).ToNot(ContainSubstring("hunter2"))
	})

	It("logs the panic with its stack and method", func() {
		_, err := call(func(context.Context, any) (any, error) { panic("nil map write") })
		Expect(err).To(HaveOccurred())

		Expect(logs.String()).To(ContainSubstring("call panicked"))
		Expect(logs.String()).To(ContainSubstring("nil map write"))
		Expect(logs.String()).To(ContainSubstring("/spec.v1.Books/GetBook"))
		Expect(logs.String()).To(ContainSubstring("recovery_test.go"))
	})

	It("reports a chain that left out the logger", func() {
		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{}),
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = recovery.Unary(a.Injector())

		Expect(err).To(MatchError(ContainSubstring("recovery: failed to invoke the logger")))
	})

	It("reports a chain that left out the domain", func() {
		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{}),
			logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = recovery.Unary(a.Injector())

		Expect(err).To(MatchError(ContainSubstring("recovery: failed to invoke the domain")))
	})
})
