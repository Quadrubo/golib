package contract

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeSoftDelete[R proto.Message](h *harness[R]) {
	if h.SoftDelete == nil || h.Delete == nil || h.Create == nil || h.Get == nil {
		return
	}

	ginkgo.Describe("soft delete", func() {
		deleted := func(ctx context.Context, parent string) R {
			ginkgo.GinkgoHelper()

			created := h.Fixture(ctx, parent)
			gomega.Expect(h.Delete(ctx, h.NameOf(created), h.EtagOf(created))).To(gomega.Succeed())

			return created
		}

		ginkgo.It("keeps the resource under its delete time", func(ctx ginkgo.SpecContext) {
			created := deleted(ctx, h.NewParent(ctx))

			found := h.Fetch(ctx, h.NameOf(created))

			gomega.Expect(has(found, h.DeleteTimeField)).To(gomega.BeTrue())
			gomega.Expect(h.EtagOf(found)).ToNot(gomega.Equal(h.EtagOf(created)))
		})

		ginkgo.It("leaves the resource out of the list unless show_deleted asks for it", func(ctx ginkgo.SpecContext) {
			skipUnless(h.List != nil, "List")

			parent := h.NewParent(ctx)
			created := deleted(ctx, parent)

			gomega.Expect(h.ListedNames(ctx, parent, ListRequest{}, h.NameOf(created))).To(gomega.BeEmpty())
			gomega.Expect(h.ListedNames(ctx, parent, ListRequest{ShowDeleted: true}, h.NameOf(created))).
				To(gomega.ConsistOf(h.NameOf(created)))
		})

		ginkgo.It("refuses a write to the resource", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Update != nil, "Update")

			created := deleted(ctx, h.NewParent(ctx))

			_, err := h.Update(ctx, h.Patch(h.Fetch(ctx, h.NameOf(created))), h.AnyMask())

			h.ExpectError(err, codes.FailedPrecondition, "")
		})

		ginkgo.It("restores the resource on undelete", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			created := deleted(ctx, parent)

			restored, err := h.SoftDelete.Undelete(ctx, h.NameOf(created))

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(has(restored, h.DeleteTimeField)).To(gomega.BeFalse())
			gomega.Expect(has(h.Fetch(ctx, h.NameOf(created)), h.DeleteTimeField)).To(gomega.BeFalse())

			if h.List != nil {
				gomega.Expect(h.ListedNames(ctx, parent, ListRequest{}, h.NameOf(created))).
					To(gomega.ConsistOf(h.NameOf(created)))
			}
		})

		ginkgo.It("returns an AlreadyExists on an undelete of a resource that is not deleted", func(ctx ginkgo.SpecContext) {
			_, err := h.SoftDelete.Undelete(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))))

			h.ExpectError(err, codes.AlreadyExists, "")
		})

		ginkgo.It("returns a NotFound on an undelete of a resource that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := h.SoftDelete.Undelete(ctx, h.AbsentName(h.NewParent(ctx)))

			h.ExpectError(err, codes.NotFound, "")
		})

		ginkgo.It("names name on an InvalidArgument on an undelete under a malformed name", func(ctx ginkgo.SpecContext) {
			_, err := h.SoftDelete.Undelete(ctx, h.MalformedName(h.NewParent(ctx)))

			h.ExpectError(err, codes.InvalidArgument, "name")
		})

		ginkgo.It("names page_token on an InvalidArgument for a token issued under another show_deleted",
			func(ctx ginkgo.SpecContext) {
				skipUnless(h.List != nil, "List")

				parent := h.NewParent(ctx)
				names := h.FixtureNames(ctx, parent, 2)

				first := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(names...), PageSize: 1})

				_, err := h.List(ctx, parent, ListRequest{
					Filter:      h.Scope(names...),
					PageSize:    1,
					PageToken:   first.NextPageToken,
					ShowDeleted: true,
				})

				h.ExpectError(err, codes.InvalidArgument, "page_token")
			})
	})
}
