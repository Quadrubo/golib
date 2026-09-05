package validate_test

import (
	"buf.build/go/protovalidate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/fieldmask"
	"github.com/quadrubo/golib/grpcinterceptor/validate"
	specv1 "github.com/quadrubo/golib/grpcinterceptor/validate/testdata/spec/v1"
)

var _ = Describe("Masked", func() {
	var validator protovalidate.Validator

	BeforeEach(func() {
		var err error
		validator, err = protovalidate.New()
		Expect(err).ToNot(HaveOccurred())
	})

	mask := fieldmask.Mask{"title": {}}

	It("passes a message whose selected fields hold", func() {
		// isbn breaks its rule, and the mask leaves it out.
		err := validate.Masked(validator,
			&specv1.Book{Title: "Alto", Isbn: "not-an-isbn"}, mask, "book")

		Expect(err).ToNot(HaveOccurred())
	})

	It("reports a selected violation under the field the resource nests in", func() {
		err := validate.Masked(validator, &specv1.Book{Isbn: "9780134190440"}, mask, "book")

		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

		var fields []string
		for _, detail := range status.Convert(err).Details() {
			if bad, ok := detail.(*errdetails.BadRequest); ok {
				for _, violation := range bad.GetFieldViolations() {
					fields = append(fields, violation.GetField())
				}
			}
		}
		Expect(fields).To(ConsistOf("book.title"))
	})

	It("passes through the error of a rule that fails to compile", func() {
		err := validate.Masked(validator,
			&specv1.Broken{Name: "x"}, fieldmask.Mask{"name": {}}, "broken")

		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).ToNot(Equal(codes.InvalidArgument))
	})
})
