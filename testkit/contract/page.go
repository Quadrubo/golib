package contract

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/quadrubo/golib/testkit/matchers"
)

func describeList[R proto.Message](h *harness[R]) {
	if h.List == nil || !h.CanSeed() {
		return
	}

	ginkgo.Describe("List", func() {
		pages(h)
		tokens(h)
		filters(h)
		orders(h)
	})
}

func pages[R proto.Message](h *harness[R]) {
	ginkgo.It("returns nothing for an empty collection", func(ctx ginkgo.SpecContext) {
		parent := h.NewParent(ctx)

		page := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(h.AbsentName(parent))})

		gomega.Expect(page.Items).To(gomega.BeEmpty())
		gomega.Expect(page.NextPageToken).To(gomega.BeEmpty())
	})

	ginkgo.It("walks the collection by keyset", func(ctx ginkgo.SpecContext) {
		parent := h.NewParent(ctx)
		names := h.FixtureNames(ctx, parent, 2)

		first := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(names...), PageSize: 1})
		gomega.Expect(first.Items).To(gomega.HaveLen(1))
		gomega.Expect(first.NextPageToken).ToNot(gomega.BeEmpty())

		second := h.Listed(ctx, parent, ListRequest{
			Filter:    h.Scope(names...),
			PageSize:  1,
			PageToken: first.NextPageToken,
		})
		gomega.Expect(second.Items).To(gomega.HaveLen(1))
		gomega.Expect(second.NextPageToken).To(gomega.BeEmpty())

		gomega.Expect(append(h.NamesOf(first.Items), h.NamesOf(second.Items)...)).To(gomega.ConsistOf(names))
	})

	ginkgo.It("takes a token under another page_size", func(ctx ginkgo.SpecContext) {
		parent := h.NewParent(ctx)
		names := h.FixtureNames(ctx, parent, 3)

		first := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(names...), PageSize: 1})

		rest := h.Listed(ctx, parent, ListRequest{
			Filter:    h.Scope(names...),
			PageSize:  2,
			PageToken: first.NextPageToken,
		})

		gomega.Expect(rest.Items).To(gomega.HaveLen(2))
		gomega.Expect(rest.NextPageToken).To(gomega.BeEmpty())
	})

	// The default page size and the clamp at the maximum go untested here.
	// Observing either takes one resource more than the size, and the
	// pagination package proves the logic in its own specs.
	ginkgo.It("names page_size on an InvalidArgument for a negative page_size", func(ctx ginkgo.SpecContext) {
		_, err := h.List(ctx, h.NewParent(ctx), ListRequest{PageSize: -1})

		h.ExpectError(err, codes.InvalidArgument, "page_size")
	})
}

func tokens[R proto.Message](h *harness[R]) {
	ginkgo.It("names page_token on an InvalidArgument for a token that does not decode", func(ctx ginkgo.SpecContext) {
		_, err := h.List(ctx, h.NewParent(ctx), ListRequest{PageToken: "not a token"})

		h.ExpectError(err, codes.InvalidArgument, "page_token")
		gomega.Expect(err).To(matchers.HaveErrorMetadata("page_token", "not a token"))
	})

	// token seeds two resources and returns a token issued over them.
	token := func(ctx context.Context, parent string) (string, []string) {
		ginkgo.GinkgoHelper()

		names := h.FixtureNames(ctx, parent, 2)

		return h.Listed(ctx, parent, ListRequest{Filter: h.Scope(names...), PageSize: 1}).NextPageToken, names
	}

	if len(h.Filters) > 0 {
		ginkgo.It("names page_token on an InvalidArgument for a token issued under another filter",
			func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				issued, names := token(ctx, parent)

				_, err := h.List(ctx, parent, ListRequest{Filter: h.Scope(names[0]), PageSize: 1, PageToken: issued})

				h.ExpectError(err, codes.InvalidArgument, "page_token")
			})
	}

	if len(h.Orders) > 0 {
		ginkgo.It("names page_token on an InvalidArgument for a token issued under another order_by",
			func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				issued, names := token(ctx, parent)

				_, err := h.List(ctx, parent, ListRequest{
					Filter:    h.Scope(names...),
					OrderBy:   h.Orders[0] + " desc",
					PageSize:  1,
					PageToken: issued,
				})

				h.ExpectError(err, codes.InvalidArgument, "page_token")
			})
	}

	if h.Parent != nil {
		ginkgo.It("names page_token on an InvalidArgument for a token issued under another parent",
			func(ctx ginkgo.SpecContext) {
				issued, _ := token(ctx, h.NewParent(ctx))

				_, err := h.List(ctx, h.NewParent(ctx), ListRequest{PageSize: 1, PageToken: issued})

				h.ExpectError(err, codes.InvalidArgument, "page_token")
			})

		ginkgo.It("names parent on an InvalidArgument for a malformed parent", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, malformedParent(h.NewParent(ctx)), ListRequest{})

			h.ExpectError(err, codes.InvalidArgument, "parent")
		})

		ginkgo.It("returns a NotFound for a parent that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, absentParent(h.NewParent(ctx)), ListRequest{})

			h.ExpectError(err, codes.NotFound, "")
		})
	}
}
