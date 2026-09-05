package apperr_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/quadrubo/golib/apperr"
)

func TestApperr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apperr Suite")
}

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

var _ = Describe("field errors", func() {
	It("reports the parse error as a violation of the field", func() {
		parseErr := errors.New(`"colour" is not a filterable field`)

		err := apperr.InvalidFilter(parseErr, `colour = "red"`)

		s := statusOf(err)
		Expect(s.Code()).To(Equal(codes.InvalidArgument))
		Expect(s.Message()).To(Equal(`"colour" is not a filterable field`))
		Expect(errorInfo(err).GetReason()).To(Equal("INVALID_FILTER"))
		Expect(errorInfo(err).GetMetadata()).To(
			HaveKeyWithValue("filter", `colour = "red"`))
		Expect(s.Details()).To(ContainElement(
			HaveField("FieldViolations", ContainElement(SatisfyAll(
				HaveField("Field", "filter"),
				HaveField("Description", `"colour" is not a filterable field`))))))
	})

	It("names the violated field apart from the metadata key", func() {
		err := apperr.InvalidResourceName("book.name", errors.New("no id"), "books/A")

		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("FieldViolations", ContainElement(
				HaveField("Field", "book.name")))))
		Expect(errorInfo(err).GetMetadata()).To(
			HaveKeyWithValue("name", "books/A"))
	})

	It("builds every field error", func() {
		parseErr := errors.New("unreadable")
		mask := &fieldmaskpb.FieldMask{Paths: []string{"colour"}}

		Expect(apperr.InvalidField("INVALID_COLOUR", "colour", "colour", "red", parseErr)).
			ToNot(BeNil())
		Expect(apperr.InvalidResourceName("name", parseErr, "books/a")).ToNot(BeNil())
		Expect(apperr.InvalidParent(parseErr, "shelves/a")).ToNot(BeNil())
		Expect(apperr.InvalidFilter(parseErr, `colour = "red"`)).ToNot(BeNil())
		Expect(apperr.InvalidOrderBy(parseErr, "colour")).ToNot(BeNil())
		Expect(apperr.InvalidPageToken(parseErr, "???")).ToNot(BeNil())
		Expect(apperr.InvalidUpdateMask(parseErr, mask)).ToNot(BeNil())
		Expect(apperr.InvalidRevisionID("zero")).ToNot(BeNil())
	})

	It("names the immutable field the mask selects under a changed value", func() {
		err := apperr.ImmutableField("book.colour", "colour")

		Expect(statusOf(err).Code()).To(Equal(codes.InvalidArgument))
		Expect(errorInfo(err).GetReason()).To(Equal("IMMUTABLE_FIELD"))
		Expect(statusOf(err).Details()).To(ContainElement(
			HaveField("FieldViolations", ContainElement(
				HaveField("Field", "book.colour")))))
		Expect(errorInfo(err).GetMetadata()).To(HaveKeyWithValue("field", "colour"))
	})
})

var _ = Describe("resource errors", func() {
	It("carries the type and the name on the ResourceInfo", func() {
		err := apperr.NotFound("BOOK_MISSING", "spec.test/Book", "books/a")

		s := statusOf(err)
		Expect(s.Code()).To(Equal(codes.NotFound))
		Expect(s.Message()).To(Equal(`the resource "books/a" does not exist`))
		Expect(errorInfo(err).GetReason()).To(Equal("BOOK_MISSING"))
		Expect(s.Details()).To(ContainElement(SatisfyAll(
			HaveField("ResourceType", "spec.test/Book"),
			HaveField("ResourceName", "books/a"))))
	})

	It("builds every resource error", func() {
		book := "books/a"

		Expect(apperr.NotFound("BOOK_MISSING", "spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.AlreadyExists("BOOK_EXISTS", "spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.Deleted("BOOK_DELETED", "spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.AlreadyDeleted("BOOK_ALREADY_DELETED", "spec.test/Book", book)).
			ToNot(BeNil())
		Expect(apperr.NotDeleted("BOOK_NOT_DELETED", "spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.EtagMismatch("spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.ConcurrentUpdate("spec.test/Book", book)).ToNot(BeNil())
		Expect(apperr.ConcurrentDelete("spec.test/Book", book)).ToNot(BeNil())
	})
})
