package errmap_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/grpcinterceptor/errmap"
	"github.com/quadrubo/golib/logging"
)

const domain grpcerr.Domain = "example.test"

// errBookGone is declared the way a service declares a sentinel, before any
// injector exists to supply a domain.
var errBookGone = grpcerr.NotFound("BOOK_MISSING", "the book no longer exists")

func TestErrmap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Errmap Suite")
}

var _ = Describe("Unary", func() {
	var (
		logs *bytes.Buffer
		it   grpc.UnaryServerInterceptor
	)

	// Boot the real logging module so the specs read the lines a service writes.
	BeforeEach(func() {
		logs = &bytes.Buffer{}

		a, err := app.New(context.Background(), []app.Module{
			// Debug level, so a spec asserting nothing was logged means it.
			config.StaticModule(map[string]any{
				"modules.logging.level":  "debug",
				"modules.grpcerr.domain": string(domain),
			}),
			logging.Module(logging.WithWriter(logs), logging.WithoutDefault()),
			grpcerr.Module(),
		})
		Expect(err).ToNot(HaveOccurred())

		it, err = errmap.Unary(a.Injector())
		Expect(err).ToNot(HaveOccurred())
	})

	call := func(returns error) (any, error) {
		return it(context.Background(), "req",
			&grpc.UnaryServerInfo{FullMethod: "/spec.v1.Books/GetBook"},
			func(context.Context, any) (any, error) {
				if returns != nil {
					return nil, returns
				}

				return "resp", nil
			})
	}

	statusOf := func(err error) *status.Status {
		GinkgoHelper()

		s, ok := status.FromError(err)
		Expect(ok).To(BeTrue())

		return s
	}

	errorInfo := func(err error) *errdetails.ErrorInfo {
		GinkgoHelper()

		for _, detail := range statusOf(err).Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok {
				return info
			}
		}
		Fail("the error carries no ErrorInfo")

		return nil
	}

	It("returns the response when the call succeeded", func() {
		resp, err := call(nil)

		Expect(err).ToNot(HaveOccurred())
		Expect(resp).To(Equal("resp"))
	})

	It("names the service on an error the handler returned", func() {
		_, err := call(grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists"))

		Expect(errorInfo(err).GetDomain()).To(Equal(string(domain)))
		Expect(statusOf(err).Code()).To(Equal(codes.NotFound))
	})

	It("derives a copy, leaving an error declared at package scope alone", func() {
		_, err := call(errBookGone)

		Expect(errorInfo(err).GetDomain()).To(Equal(string(domain)))
		Expect(errorInfo(errBookGone).GetDomain()).To(BeEmpty())
	})

	It("unwraps the error so the wrapper text stays off the wire", func() {
		_, err := call(fmt.Errorf("read books from pool 10.1.2.3: %w",
			grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists")))

		// Passing the wrapper on would put the pool address on the wire.
		Expect(statusOf(err).Message()).To(Equal("no book named books/a exists"))
		Expect(statusOf(err).Code()).To(Equal(codes.NotFound))
	})

	It("replaces an error it cannot classify with Internal", func() {
		_, err := call(fmt.Errorf("query books: %w", sql.ErrNoRows))

		Expect(statusOf(err).Code()).To(Equal(codes.Internal))
		Expect(statusOf(err).Message()).To(Equal("internal error"))
		Expect(errorInfo(err).GetReason()).To(Equal("INTERNAL"))
		Expect(errorInfo(err).GetDomain()).To(Equal(string(domain)))
	})

	It("replaces a bare status error with Internal", func() {
		// An interceptor that built one would otherwise reach the wire without an
		// ErrorInfo.
		_, err := call(status.Error(codes.NotFound, "pq: password auth failed for admin"))

		Expect(statusOf(err).Code()).To(Equal(codes.Internal))
		Expect(statusOf(err).Message()).ToNot(ContainSubstring("password"))
	})

	It("logs the error it replaced", func() {
		_, err := call(errors.New("pq: password auth failed for admin"))
		Expect(err).To(HaveOccurred())

		Expect(logs.String()).To(ContainSubstring("unhandled error"))
		Expect(logs.String()).To(ContainSubstring("password auth failed"))
		Expect(logs.String()).To(ContainSubstring("/spec.v1.Books/GetBook"))
	})

	It("does not log an error the handler returned", func() {
		_, err := call(grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists"))
		Expect(err).To(HaveOccurred())

		// An error the handler chose is not a surprise, so there is nothing to log.
		Expect(logs.String()).To(BeEmpty())
	})

	It("logs the context a wrapper added, which the caller never sees", func() {
		_, err := call(fmt.Errorf("read books from pool 10.1.2.3: %w",
			grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists")))
		Expect(err).To(HaveOccurred())

		Expect(logs.String()).To(ContainSubstring("pool 10.1.2.3"))
		Expect(logs.String()).To(ContainSubstring("/spec.v1.Books/GetBook"))
	})

	It("maps a cancelled context to Canceled", func() {
		_, err := call(fmt.Errorf("query books: %w", context.Canceled))

		// grpc maps context errors itself, but only for errors reaching the transport.
		Expect(statusOf(err).Code()).To(Equal(codes.Canceled))
		Expect(logs.String()).To(BeEmpty())
	})

	It("maps an expired deadline to DeadlineExceeded", func() {
		_, err := call(fmt.Errorf("query books: %w", context.DeadlineExceeded))

		Expect(statusOf(err).Code()).To(Equal(codes.DeadlineExceeded))
	})

	It("keeps the handler code over a context error it wraps", func() {
		_, err := call(grpcerr.Aborted("BOOK_CHANGED", "the book changed underneath"))

		Expect(statusOf(err).Code()).To(Equal(codes.Aborted))
	})

	It("reports a chain that left out the logger", func() {
		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{}),
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = errmap.Unary(a.Injector())

		Expect(err).To(MatchError(ContainSubstring("errmap: failed to invoke the logger")))
	})

	It("reports a chain that left out the domain", func() {
		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(map[string]any{}),
			logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = errmap.Unary(a.Injector())

		Expect(err).To(MatchError(ContainSubstring("errmap: failed to invoke the domain")))
	})
})
