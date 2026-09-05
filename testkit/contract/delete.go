package contract

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeDelete[R proto.Message](h *harness[R]) {
	if h.Delete == nil || h.Create == nil {
		return
	}

	ginkgo.Describe("Delete", func() {
		ginkgo.It("removes the resource", func(ctx ginkgo.SpecContext) {
			created := h.Fixture(ctx, h.NewParent(ctx))

			gomega.Expect(h.Delete(ctx, h.NameOf(created), h.EtagOf(created))).To(gomega.Succeed())

			if h.SoftDelete == nil {
				_, err := h.Get(ctx, h.NameOf(created))
				h.ExpectError(err, codes.NotFound, "")
			}
		})

		ginkgo.It("returns a NotFound for a resource that is already deleted", func(ctx ginkgo.SpecContext) {
			created := h.Fixture(ctx, h.NewParent(ctx))

			gomega.Expect(h.Delete(ctx, h.NameOf(created), h.EtagOf(created))).To(gomega.Succeed())

			// A soft delete is a write, so the deleted resource carries a new etag.
			etag := h.EtagOf(created)
			if h.SoftDelete != nil {
				etag = h.EtagOf(h.Fetch(ctx, h.NameOf(created)))
			}

			err := h.Delete(ctx, h.NameOf(created), etag)

			h.ExpectError(err, codes.NotFound, "")
		})

		ginkgo.It("names name on an InvalidArgument for a malformed name", func(ctx ginkgo.SpecContext) {
			err := h.Delete(ctx, h.MalformedName(h.NewParent(ctx)), "etag")

			h.ExpectError(err, codes.InvalidArgument, "name")
		})

		ginkgo.It("returns a NotFound for a resource that does not exist", func(ctx ginkgo.SpecContext) {
			err := h.Delete(ctx, h.AbsentName(h.NewParent(ctx)), "etag")

			h.ExpectError(err, codes.NotFound, "")
		})

		ginkgo.It("aborts on an etag another write outdated", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Update != nil, "Update")

			base := h.Fixture(ctx, h.NewParent(ctx))

			_, err := h.Update(ctx, h.Patch(base), h.AnyMask())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = h.Delete(ctx, h.NameOf(base), h.EtagOf(base))

			h.ExpectEtagMismatchError(err)
		})

		ginkgo.It("rejects a delete without an etag", func(ctx ginkgo.SpecContext) {
			created := h.Fixture(ctx, h.NewParent(ctx))

			err := h.Delete(ctx, h.NameOf(created), "")

			h.ExpectError(err, codes.InvalidArgument, "etag")
		})

		if h.Parent != nil && h.Parent.Delete != nil {
			ginkgo.It("takes the resource along with its parent", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				created := h.Fixture(ctx, parent)

				h.Parent.Delete(ctx, parent)

				_, err := h.Get(ctx, h.NameOf(created))
				h.ExpectError(err, codes.NotFound, "")

				if h.List != nil {
					_, err = h.List(ctx, parent, ListRequest{})
					h.ExpectError(err, codes.NotFound, "")
				}
			})
		}
	})
}
