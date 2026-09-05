package matchers_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"

	. "github.com/quadrubo/golib/testkit/matchers"
)

func TestMatchers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Matchers Suite")
}

// withDetails returns an InvalidArgument status error carrying the details.
func withDetails(details ...protoadapt.MessageV1) error {
	GinkgoHelper()

	st, err := status.New(codes.InvalidArgument, "rejected").WithDetails(details...)
	Expect(err).ToNot(HaveOccurred())

	return st.Err()
}

var _ = Describe("HaveViolatedField", func() {
	badRequest := &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
		{Field: "parent"},
		{Field: "page_size"},
	}}

	It("matches a field the BadRequest names", func() {
		Expect(withDetails(badRequest)).To(HaveViolatedField("page_size"))
	})

	It("names the violated fields when the field is not among them", func() {
		matcher := HaveViolatedField("filter")
		rejected := withDetails(badRequest)

		ok, err := matcher.Match(rejected)

		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeFalse())
		Expect(matcher.FailureMessage(rejected)).
			To(ContainSubstring(`the BadRequest names ["parent" "page_size"]`))
	})

	It("fails for a status without a BadRequest", func() {
		Expect(withDetails()).ToNot(HaveViolatedField("parent"))
	})

	It("errors for an error that is no gRPC status", func() {
		_, err := HaveViolatedField("parent").Match(errors.New("plain"))

		Expect(err).To(MatchError(ContainSubstring("expected a gRPC status")))
	})

	It("errors for a value that is no error", func() {
		_, err := HaveViolatedField("parent").Match("parent")

		Expect(err).To(MatchError(ContainSubstring("expected an error")))
	})
})

var _ = Describe("HaveViolationDescription", func() {
	badRequest := &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
		{Field: "parent", Description: "parent is required"},
		{Field: "page_size", Description: "page_size must be positive"},
	}}

	It("matches a description the BadRequest carries", func() {
		Expect(withDetails(badRequest)).To(HaveViolationDescription("page_size must be positive"))
	})

	It("names the carried descriptions when the description is not among them", func() {
		matcher := HaveViolationDescription("filter is too long")
		rejected := withDetails(badRequest)

		ok, err := matcher.Match(rejected)

		Expect(err).ToNot(HaveOccurred())
		Expect(ok).To(BeFalse())
		Expect(matcher.FailureMessage(rejected)).
			To(ContainSubstring(`the BadRequest carries ["parent is required" "page_size must be positive"]`))
	})

	It("fails for a status without a BadRequest", func() {
		Expect(withDetails()).ToNot(HaveViolationDescription("parent is required"))
	})
})

var _ = Describe("HaveReason", func() {
	info := &errdetails.ErrorInfo{Reason: "ETAG_MISMATCH", Domain: "example.test"}

	It("matches the reason the ErrorInfo carries", func() {
		Expect(withDetails(info)).To(HaveReason("ETAG_MISMATCH"))
	})

	It("fails for another reason", func() {
		Expect(withDetails(info)).ToNot(HaveReason("NOT_FOUND"))
	})

	It("fails for a status without an ErrorInfo", func() {
		Expect(withDetails()).ToNot(HaveReason("ETAG_MISMATCH"))
	})
})

var _ = Describe("HaveErrorDomain", func() {
	info := &errdetails.ErrorInfo{Reason: "ETAG_MISMATCH", Domain: "example.test"}

	It("matches the domain the ErrorInfo carries", func() {
		Expect(withDetails(info)).To(HaveErrorDomain("example.test"))
	})

	It("fails for another domain", func() {
		Expect(withDetails(info)).ToNot(HaveErrorDomain("other.test"))
	})

	It("fails for a status without an ErrorInfo", func() {
		Expect(withDetails()).ToNot(HaveErrorDomain("example.test"))
	})
})

var _ = Describe("HaveErrorMetadata", func() {
	info := &errdetails.ErrorInfo{
		Reason:   "INVALID_FILTER",
		Domain:   "example.test",
		Metadata: map[string]string{"filter": `colour = "red"`},
	}

	It("matches the value the ErrorInfo maps the key to", func() {
		Expect(withDetails(info)).To(HaveErrorMetadata("filter", `colour = "red"`))
	})

	It("fails for another value", func() {
		Expect(withDetails(info)).ToNot(HaveErrorMetadata("filter", `colour = "blue"`))
	})

	It("fails for a key the metadata does not hold", func() {
		Expect(withDetails(info)).ToNot(HaveErrorMetadata("page_token", "???"))
	})
})
