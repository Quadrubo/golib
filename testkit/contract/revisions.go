package contract

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeRevisions[R proto.Message](h *harness[R]) {
	if h.Revisions == nil || h.Create == nil || h.Update == nil {
		return
	}

	r := h.Revisions

	ginkgo.Describe("revisions", func() {
		// written creates a Minimal resource and writes Full over it, which
		// records the Minimal values as the first revision.
		written := func(ctx context.Context, parent string) (base, updated R) {
			ginkgo.GinkgoHelper()

			skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

			base = h.Make(ctx, parent, h.Minimal())

			updated, err := h.Update(ctx, h.Identified(h.Full(), base), nil)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			return base, updated
		}

		// twice writes Full and then Minimal over a Minimal resource, which
		// records two revisions.
		twice := func(ctx context.Context, parent string) R {
			ginkgo.GinkgoHelper()

			base, updated := written(ctx, parent)

			_, err := h.Update(ctx, h.Identified(h.Minimal(), updated), nil)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			return base
		}

		ginkgo.It("holds no revision for a resource no write touched", func(ctx ginkgo.SpecContext) {
			created := h.Fixture(ctx, h.NewParent(ctx))

			page, err := r.List(ctx, h.NameOf(created), ListRequest{})

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(page.Items).To(gomega.BeEmpty())
			gomega.Expect(page.NextPageToken).To(gomega.BeEmpty())
		})

		ginkgo.It("records the replaced values as a revision on each write, the newest first", func(ctx ginkgo.SpecContext) {
			base := twice(ctx, h.NewParent(ctx))

			page, err := r.List(ctx, h.NameOf(base), ListRequest{})

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(revisionNames(page)).To(gomega.Equal([]string{
				h.NameOf(base) + "/revisions/2",
				h.NameOf(base) + "/revisions/1",
			}))

			gomega.Expect(r.Snapshot(page.Items[0])).To(gomega.BeComparableTo(h.Full(), h.Comparing(h.Writable...)...))
			gomega.Expect(r.Snapshot(page.Items[1])).To(gomega.BeComparableTo(h.Minimal(), h.Comparing(h.Writable...)...))
		})

		ginkgo.It("walks the revisions by keyset", func(ctx ginkgo.SpecContext) {
			base := twice(ctx, h.NewParent(ctx))

			first, err := r.List(ctx, h.NameOf(base), ListRequest{PageSize: 1})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(first.Items).To(gomega.HaveLen(1))
			gomega.Expect(first.NextPageToken).ToNot(gomega.BeEmpty())

			second, err := r.List(ctx, h.NameOf(base), ListRequest{PageSize: 1, PageToken: first.NextPageToken})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(second.Items).To(gomega.HaveLen(1))
			gomega.Expect(second.NextPageToken).To(gomega.BeEmpty())

			gomega.Expect(append(revisionNames(first), revisionNames(second)...)).To(gomega.Equal([]string{
				h.NameOf(base) + "/revisions/2",
				h.NameOf(base) + "/revisions/1",
			}))
		})

		ginkgo.It("names page_token on an InvalidArgument for a token that does not decode", func(ctx ginkgo.SpecContext) {
			_, err := r.List(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))), ListRequest{PageToken: "not a token"})

			h.ExpectError(err, codes.InvalidArgument, "page_token")
		})

		ginkgo.It("names parent on an InvalidArgument for a list under a malformed resource name",
			func(ctx ginkgo.SpecContext) {
				_, err := r.List(ctx, h.MalformedName(h.NewParent(ctx)), ListRequest{})

				h.ExpectError(err, codes.InvalidArgument, "parent")
			})

		ginkgo.It("returns a NotFound for a list under a resource that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := r.List(ctx, h.AbsentName(h.NewParent(ctx)), ListRequest{})

			h.ExpectError(err, codes.NotFound, "")
		})

		ginkgo.It("returns the revision", func(ctx ginkgo.SpecContext) {
			base, _ := written(ctx, h.NewParent(ctx))

			revision, err := r.Get(ctx, h.NameOf(base), "1")

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(r.Snapshot(revision)).To(gomega.BeComparableTo(h.Minimal(), h.Comparing(h.Writable...)...))
		})

		ginkgo.It("returns a NotFound for a revision the resource does not hold", func(ctx ginkgo.SpecContext) {
			_, err := r.Get(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))), "99")

			h.ExpectError(err, codes.NotFound, "")
		})

		for _, id := range []string{"0", "abc"} {
			ginkgo.It("names name on an InvalidArgument for the revision "+id, func(ctx ginkgo.SpecContext) {
				_, err := r.Get(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))), id)

				h.ExpectError(err, codes.InvalidArgument, "name")
			})
		}

		ginkgo.It("writes the snapshot back on rollback and records the replaced values", func(ctx ginkgo.SpecContext) {
			base, updated := written(ctx, h.NewParent(ctx))

			restored, err := r.Rollback(ctx, h.NameOf(base), "1")

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(restored).To(gomega.BeComparableTo(h.Minimal(), h.Comparing(h.Writable...)...))
			gomega.Expect(h.EtagOf(restored)).ToNot(gomega.Equal(h.EtagOf(updated)))

			page, err := r.List(ctx, h.NameOf(base), ListRequest{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(page.Items).To(gomega.HaveLen(2))
			gomega.Expect(r.Snapshot(page.Items[0])).To(gomega.BeComparableTo(h.Full(), h.Comparing(h.Writable...)...))
		})

		ginkgo.It("returns a NotFound on a rollback to a revision the resource does not hold",
			func(ctx ginkgo.SpecContext) {
				_, err := r.Rollback(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))), "99")

				h.ExpectError(err, codes.NotFound, "")
			})

		for _, id := range []string{"0", "abc"} {
			ginkgo.It("names revision_id on an InvalidArgument for a rollback to the revision "+id,
				func(ctx ginkgo.SpecContext) {
					_, err := r.Rollback(ctx, h.NameOf(h.Fixture(ctx, h.NewParent(ctx))), id)

					h.ExpectError(err, codes.InvalidArgument, "revision_id")
				})
		}

		ginkgo.It("returns a NotFound on a rollback of a resource that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := r.Rollback(ctx, h.AbsentName(h.NewParent(ctx)), "1")

			h.ExpectError(err, codes.NotFound, "")
		})

		if h.SoftDelete != nil && h.Delete != nil {
			ginkgo.It("refuses a rollback of a deleted resource", func(ctx ginkgo.SpecContext) {
				base, updated := written(ctx, h.NewParent(ctx))
				gomega.Expect(h.Delete(ctx, h.NameOf(base), h.EtagOf(updated))).To(gomega.Succeed())

				_, err := r.Rollback(ctx, h.NameOf(base), "1")

				h.ExpectError(err, codes.FailedPrecondition, "")
			})
		}
	})
}

// revisionNames returns the names of the revisions on the page.
func revisionNames(page RevisionPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		reflected := item.ProtoReflect()
		names = append(names, reflected.Get(reflected.Descriptor().Fields().ByName("name")).String())
	}

	return names
}
