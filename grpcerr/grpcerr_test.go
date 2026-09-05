package grpcerr_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/grpcerr"
)

func TestGrpcerr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpcerr Suite")
}

// The fields are unexported, so the specs read back the status a caller gets.
func statusOf(err error) *status.Status {
	GinkgoHelper()

	s, ok := status.FromError(err)
	Expect(ok).To(BeTrue())

	return s
}

func errorInfo(err error) *errdetails.ErrorInfo {
	GinkgoHelper()

	for _, detail := range statusOf(err).Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	Fail("the error carries no ErrorInfo")

	return nil
}

var _ = Describe("Error", func() {
	countOf := func(err error, want any) int {
		GinkgoHelper()

		found := 0
		for _, detail := range statusOf(err).Details() {
			if fmt.Sprintf("%T", detail) == fmt.Sprintf("%T", want) {
				found++
			}
		}

		return found
	}

	It("returns the code and message it was built with", func() {
		s := statusOf(grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists"))

		Expect(s.Code()).To(Equal(codes.NotFound))
		Expect(s.Message()).To(Equal("no book named books/a exists"))
	})

	It("attaches an ErrorInfo to every error, which AIP-193 requires", func() {
		Expect(errorInfo(grpcerr.Aborted("BOOK_CHANGED", "the book changed underneath"))).
			ToNot(BeNil())
	})

	It("reports the reason it was built with", func() {
		err := grpcerr.FailedPrecondition("BOOK_IN_USE", "the book is still in use")

		Expect(errorInfo(err).GetReason()).To(Equal("BOOK_IN_USE"))
	})

	It("panics on a reason outside the UPPER_SNAKE format", func() {
		Expect(func() {
			_ = grpcerr.NotFound("book-missing", "gone")
		}).To(PanicWith(ContainSubstring(`"book-missing"`)))
	})

	It("replaces the placeholder with the quoted value and records the pair", func() {
		err := grpcerr.NotFound("BOOK_MISSING", "no book named {name} exists",
			grpcerr.Arg("name", "books/a"))

		Expect(err.Error()).To(Equal(`no book named "books/a" exists`))
		Expect(errorInfo(err).GetMetadata()).To(
			HaveKeyWithValue("name", "books/a"))
	})

	It("panics on a placeholder no arg fills", func() {
		Expect(func() {
			_ = grpcerr.NotFound("BOOK_MISSING", "no book named {name} exists")
		}).To(PanicWith(ContainSubstring("{name}")))
	})

	It("panics on an arg the message does not name", func() {
		Expect(func() {
			_ = grpcerr.NotFound("BOOK_MISSING", "the book is gone",
				grpcerr.Arg("name", "books/a"))
		}).To(PanicWith(ContainSubstring("{name}")))
	})

	It("panics on a metadata key outside the AIP-193 format", func() {
		Expect(func() {
			grpcerr.Arg("book.name", "books/a")
		}).To(PanicWith(ContainSubstring(`"book.name"`)))
	})

	It("takes the message and the metadata pair from a parse error", func() {
		parseErr := fmt.Errorf("%q is not a filterable field", "colour")

		err := grpcerr.InvalidArgumentFrom("INVALID_FILTER", parseErr, "filter", `colour = "red"`)

		s := statusOf(err)
		Expect(s.Code()).To(Equal(codes.InvalidArgument))
		Expect(s.Message()).To(Equal(`"colour" is not a filterable field`))
		Expect(errorInfo(err).GetReason()).To(Equal("INVALID_FILTER"))
		Expect(errorInfo(err).GetMetadata()).To(
			HaveKeyWithValue("filter", `colour = "red"`))
	})

	It("reports the domain it was given", func() {
		err := grpcerr.NotFound("BOOK_MISSING", "gone").WithDomain("example.test")

		Expect(errorInfo(err).GetDomain()).To(Equal("example.test"))
	})

	It("reports the metadata it was given", func() {
		err := grpcerr.FailedPrecondition("BOOK_IN_USE", "the book is in use").
			Metadata("resource", "books/main").
			Metadata("use_count", "3")

		Expect(errorInfo(err).GetMetadata()).To(Equal(map[string]string{
			"resource":  "books/main",
			"use_count": "3",
		}))
	})

	It("reports a resource on the detail and in the metadata at once", func() {
		err := grpcerr.NotFound("BOOK_MISSING", "gone").
			ForResource("spec.test/Book", "books/main")

		Expect(statusOf(err).Details()).To(ContainElement(SatisfyAll(
			HaveField("ResourceType", "spec.test/Book"),
			HaveField("ResourceName", "books/main"))))
		Expect(statusOf(err).Details()).To(ContainElement(
			SatisfyAll(
				BeAssignableToTypeOf(&errdetails.ErrorInfo{}),
				HaveField("Metadata", HaveKeyWithValue("name", "books/main")))))
	})

	It("reports no resource for a name it was not given", func() {
		err := grpcerr.NotFound("BOOK_MISSING", "gone").ForResource("spec.test/Book", "")

		Expect(statusOf(err).Details()).ToNot(ContainElement(
			BeAssignableToTypeOf(&errdetails.ResourceInfo{})))
		Expect(statusOf(err).Details()).To(ContainElement(
			SatisfyAll(
				BeAssignableToTypeOf(&errdetails.ErrorInfo{}),
				HaveField("Metadata", BeEmpty()))))
	})

	It("attaches a BadRequest for the field violations", func() {
		err := grpcerr.InvalidArgument("INVALID_FIELDS", "the request has invalid fields").
			WithFieldViolations(&errdetails.BadRequest_FieldViolation{
				Field:       "update_mask",
				Description: "unknown path 'colour'",
			})

		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("FieldViolations", HaveLen(1))))
	})

	It("attaches a PreconditionFailure for the violations", func() {
		err := grpcerr.FailedPrecondition("BOOK_IN_USE", "the book is still in use").
			WithPreconditionViolations(&errdetails.PreconditionFailure_Violation{
				Type: "BOOK_IN_USE", Subject: "books/main",
			})

		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("Violations", HaveLen(1))))
	})

	It("replaces a detail instead of adding a second of its type", func() {
		err := grpcerr.NotFound("BOOK_MISSING", "gone").
			ForResource("spec.test/Book", "books/a").
			ForResource("spec.test/Book", "books/b")

		// AIP-193 allows each detail type once, so the second call replaces the first.
		Expect(countOf(err, &errdetails.ResourceInfo{})).To(Equal(1))
		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("ResourceName", "books/b")))
	})

	It("returns a copy, leaving the error it was built from unchanged", func() {
		shared := grpcerr.NotFound("BOOK_MISSING", "gone")

		one := shared.WithDomain("one.test").ForResource("spec.test/Book", "books/a")
		two := shared.WithDomain("two.test").ForResource("spec.test/Book", "books/b")

		// Whatever renders a package-level error sets the domain on a copy.
		Expect(errorInfo(shared).GetDomain()).To(BeEmpty())
		Expect(errorInfo(one).GetDomain()).To(Equal("one.test"))
		Expect(errorInfo(two).GetDomain()).To(Equal("two.test"))
		Expect(countOf(shared, &errdetails.ResourceInfo{})).To(BeZero())
	})

	It("returns a copy of the metadata, not the map it was built from", func() {
		shared := grpcerr.NotFound("BOOK_MISSING", "gone").Metadata("resource", "books/a")

		one := shared.Metadata("attempt", "1")

		Expect(errorInfo(shared).GetMetadata()).To(HaveLen(1))
		Expect(errorInfo(one).GetMetadata()).To(HaveLen(2))
	})

	It("keeps the ErrorInfo when another payload cannot be marshalled", func() {
		unmarshalable := "colour" + string([]byte{0xff, 0xfe})

		err := grpcerr.InvalidArgument("INVALID_FIELDS", "the request has invalid fields").
			WithFieldViolations(&errdetails.BadRequest_FieldViolation{Field: unmarshalable})

		// A proto3 string has to be valid UTF-8.
		Expect(errorInfo(err).GetReason()).To(Equal("INVALID_FIELDS"))
		Expect(countOf(err, &errdetails.BadRequest{})).To(BeZero())
	})

	It("skips the BadRequest when there are no violations", func() {
		err := grpcerr.InvalidArgument("INVALID_FIELDS", "bad").WithFieldViolations()

		Expect(countOf(err, &errdetails.BadRequest{})).To(BeZero())
	})

	It("returns only the message from Error", func() {
		// Error reaches the logs, so it must not carry more than the status does.
		Expect(grpcerr.NotFound("BOOK_MISSING", "no book named books/a exists").Error()).
			To(Equal("no book named books/a exists"))
	})

	It("loses its message to the wrapper when grpc unwraps it", func() {
		wrapped := fmt.Errorf("read books from pool 10.1.2.3: %w",
			grpcerr.NotFound("BOOK_MISSING", "gone"))

		// status.FromError rebuilds the message from the chain when it unwraps.
		Expect(statusOf(wrapped).Message()).To(ContainSubstring("pool 10.1.2.3"))
	})
})

var _ = Describe("Module", func() {
	provide := func(settings map[string]any) (grpcerr.Domain, error) {
		GinkgoHelper()

		a, err := app.New(context.Background(), []app.Module{
			config.StaticModule(settings),
			grpcerr.Module(),
		})
		if err != nil {
			return "", err
		}

		return do.Invoke[grpcerr.Domain](a.Injector())
	}

	It("provides the domain the config sets", func() {
		domain, err := provide(map[string]any{
			"modules.grpcerr.domain": "hosted.example",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(domain).To(Equal(grpcerr.Domain("hosted.example")))
	})

	It("refuses to provide a domain no layer sets", func() {
		_, err := provide(map[string]any{})

		Expect(err).To(MatchError(ContainSubstring("domain")))
	})
})
