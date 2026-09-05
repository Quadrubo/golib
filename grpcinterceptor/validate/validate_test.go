package validate_test

import (
	"context"
	"testing"

	"buf.build/go/protovalidate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/grpcinterceptor/validate"
	specv1 "github.com/quadrubo/golib/grpcinterceptor/validate/testdata/spec/v1"
)

const domain grpcerr.Domain = "example.test"

func TestValidate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validate Suite")
}

var _ = Describe("Unary", func() {
	var (
		it      grpc.UnaryServerInterceptor
		reached bool
	)

	BeforeEach(func() {
		i := do.New()
		do.ProvideValue(i, domain)

		validator, err := protovalidate.New()
		Expect(err).ToNot(HaveOccurred())
		do.ProvideValue(i, validator)

		it, err = validate.Unary(i)
		Expect(err).ToNot(HaveOccurred())

		reached = false
	})

	It("fails to build without the validator the module provides", func() {
		i := do.New()
		do.ProvideValue(i, domain)

		_, err := validate.Unary(i)

		Expect(err).To(MatchError(ContainSubstring("validator")))
	})

	call := func(req any) (any, error) {
		return it(context.Background(), req,
			&grpc.UnaryServerInfo{FullMethod: "/spec.v1.Books/GetBook"},
			func(context.Context, any) (any, error) {
				reached = true

				return "resp", nil
			})
	}

	statusOf := func(err error) *status.Status {
		GinkgoHelper()

		s, ok := status.FromError(err)
		Expect(ok).To(BeTrue())

		return s
	}

	It("calls the handler when the message keeps its rules", func() {
		resp, err := call(&specv1.Book{Title: "The Go Programming Language", Isbn: "9780134190440"})

		Expect(err).ToNot(HaveOccurred())
		Expect(resp).To(Equal("resp"))
		Expect(reached).To(BeTrue())
	})

	It("returns InvalidArgument when a rule fails", func() {
		_, err := call(&specv1.Book{Isbn: "9780134190440"})

		Expect(statusOf(err).Code()).To(Equal(codes.InvalidArgument))
		Expect(reached).To(BeFalse())
	})

	It("attaches a violation naming the field that failed", func() {
		_, err := call(&specv1.Book{Title: "The Go Programming Language", Isbn: "not-an-isbn"})

		// AIP-193 puts the field in a BadRequest so a client does not parse the
		// message.
		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("FieldViolations", ConsistOf(HaveField("Field", "isbn")))))
	})

	It("attaches one violation per broken rule", func() {
		_, err := call(&specv1.Book{Isbn: "not-an-isbn"})

		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("FieldViolations", HaveLen(2))))
	})

	It("builds the error it returns under its own domain", func() {
		_, err := call(&specv1.Book{Isbn: "9780134190440"})

		Expect(statusOf(err).Details()).To(ContainElement(HaveField("Domain", string(domain))))
	})

	It("leaves the field details out of the message", func() {
		_, err := call(&specv1.Book{Isbn: "9780134190440"})

		Expect(statusOf(err).Message()).To(Equal("the request has invalid fields"))
	})

	It("calls the handler when the request is not a proto message", func() {
		resp, err := call("not a proto")

		Expect(err).ToNot(HaveOccurred())
		Expect(resp).To(Equal("resp"))
	})

	It("returns a raw error when a rule fails to compile", func() {
		_, err := call(&specv1.Broken{Name: "anything"})

		// A rule we wrote wrongly goes back raw for errmap to log and replace.
		Expect(err).To(MatchError(ContainSubstring("validate:")))
		_, isStatus := status.FromError(err)
		Expect(isStatus).To(BeFalse())
		Expect(reached).To(BeFalse())
	})
})
