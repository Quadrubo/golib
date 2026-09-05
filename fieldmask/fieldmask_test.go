package fieldmask_test

import (
	"testing"

	"buf.build/go/protovalidate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/quadrubo/golib/fieldmask"
	specv1 "github.com/quadrubo/golib/grpcinterceptor/validate/testdata/spec/v1"
)

func TestFieldmask(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fieldmask Suite")
}

var _ = Describe("Parse", func() {
	paths := fieldmask.Paths{Writable: []string{"title", "isbn"}, Immutable: []string{"author"}}

	It("returns the paths the mask selects", func() {
		mask, err := fieldmask.Parse(&fieldmaskpb.FieldMask{Paths: []string{"isbn"}}, paths)

		Expect(err).ToNot(HaveOccurred())
		Expect(mask.Has("isbn")).To(BeTrue())
		Expect(mask.Has("title")).To(BeFalse())
	})

	It("returns every writable path for an omitted mask", func() {
		mask, err := fieldmask.Parse(nil, paths)

		Expect(err).ToNot(HaveOccurred())
		Expect(mask.Has("title")).To(BeTrue())
		Expect(mask.Has("isbn")).To(BeTrue())
		Expect(mask.Has("author")).To(BeFalse())
	})

	It("returns every writable path for a mask that carries no path", func() {
		mask, err := fieldmask.Parse(&fieldmaskpb.FieldMask{}, paths)

		Expect(err).ToNot(HaveOccurred())
		Expect(mask.Has("title")).To(BeTrue())
	})

	It("takes an immutable path the mask selects", func() {
		mask, err := fieldmask.Parse(&fieldmaskpb.FieldMask{Paths: []string{"author"}}, paths)

		Expect(err).ToNot(HaveOccurred())
		Expect(mask.Has("author")).To(BeTrue())
	})

	It("returns an error naming a path outside the writable and immutable sets", func() {
		_, err := fieldmask.Parse(&fieldmaskpb.FieldMask{Paths: []string{"create_time"}}, paths)

		Expect(err).To(MatchError(ContainSubstring("create_time")))
	})
})

var _ = Describe("Filter", func() {
	paths := fieldmask.Paths{Writable: []string{"title", "isbn"}}

	// isbn breaks its pattern and title breaks its length, so either field
	// reports a violation once the mask selects it.
	invalid := &specv1.Book{Title: "", Isbn: "not-an-isbn"}

	It("runs the rules of a field the mask selects", func() {
		mask, err := fieldmask.Parse(&fieldmaskpb.FieldMask{Paths: []string{"isbn"}}, paths)
		Expect(err).ToNot(HaveOccurred())

		err = protovalidate.Validate(invalid, protovalidate.WithFilter(fieldmask.Filter(mask)))

		Expect(err).To(MatchError(ContainSubstring("isbn")))
		Expect(err).ToNot(MatchError(ContainSubstring("title")))
	})

	It("skips the rules of a field the mask leaves out", func() {
		mask, err := fieldmask.Parse(&fieldmaskpb.FieldMask{Paths: []string{"title"}}, paths)
		Expect(err).ToNot(HaveOccurred())

		err = protovalidate.Validate(invalid, protovalidate.WithFilter(fieldmask.Filter(mask)))

		Expect(err).To(MatchError(ContainSubstring("title")))
		Expect(err).ToNot(MatchError(ContainSubstring("isbn")))
	})

	It("runs no rule for a mask that selects nothing", func() {
		err := protovalidate.Validate(invalid, protovalidate.WithFilter(fieldmask.Filter(fieldmask.Mask{})))

		Expect(err).ToNot(HaveOccurred())
	})
})
