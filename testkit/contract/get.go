package contract

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeGet[R proto.Message](h *harness[R]) {
	if h.Get == nil {
		return
	}

	ginkgo.Describe("Get", func() {
		ginkgo.It("returns the resource the create returned", func(ctx ginkgo.SpecContext) {
			skipUnless(h.CanSeed(), "Create or Seed")

			created := h.Fixture(ctx, h.NewParent(ctx))

			found := h.Fetch(ctx, h.NameOf(created))

			gomega.Expect(h.NameOf(found)).To(gomega.Equal(h.NameOf(created)))
			gomega.Expect(h.EtagOf(found)).To(gomega.Equal(h.EtagOf(created)))
			gomega.Expect(found).To(gomega.BeComparableTo(created, h.Comparing(h.Writable...)...))
		})

		ginkgo.It("names name on an InvalidArgument for a malformed name", func(ctx ginkgo.SpecContext) {
			_, err := h.Get(ctx, h.MalformedName(h.NewParent(ctx)))

			h.ExpectError(err, codes.InvalidArgument, "name")
		})

		ginkgo.It("returns a NotFound for a resource that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := h.Get(ctx, h.AbsentName(h.NewParent(ctx)))

			h.ExpectError(err, codes.NotFound, "")
		})
	})
}
