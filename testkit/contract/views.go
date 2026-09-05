package contract

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeViews[R proto.Message](h *harness[R]) {
	if h.Views == nil {
		return
	}

	v := h.Views

	ginkgo.Describe("views", func() {
		expensive := func(resource R, present bool) {
			ginkgo.GinkgoHelper()

			for _, field := range v.Expensive {
				gomega.Expect(has(resource, h.Fields[field])).To(gomega.Equal(present), "%s under the view", field)
			}
		}

		// listed returns the seeded resource as the list under the view returns it.
		listed := func(ctx context.Context, parent string, seeded R, view int32) R {
			ginkgo.GinkgoHelper()

			page, err := v.List(ctx, parent, ListRequest{Filter: h.Scope(h.NameOf(seeded))}, view)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(page.Items).To(gomega.HaveLen(1))

			return page.Items[0]
		}

		ginkgo.It("fills the expensive fields under the full view alone on a get", func(ctx ginkgo.SpecContext) {
			skipUnless(v.Get != nil && v.Seed != nil, "Get and Seed")

			seeded := v.Seed(ctx, h.NewParent(ctx))

			basic, err := v.Get(ctx, h.NameOf(seeded), v.Basic)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			expensive(basic, false)

			full, err := v.Get(ctx, h.NameOf(seeded), v.Full)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			expensive(full, true)
		})

		ginkgo.It("fills the expensive fields under the full view alone on a list", func(ctx ginkgo.SpecContext) {
			skipUnless(v.List != nil && v.Seed != nil, "List and Seed")

			parent := h.NewParent(ctx)
			seeded := v.Seed(ctx, parent)

			expensive(listed(ctx, parent, seeded, v.Basic), false)
			expensive(listed(ctx, parent, seeded, v.Full), true)
		})

		ginkgo.It(fmt.Sprintf("takes an unspecified view as %d on a get", v.GetDefault), func(ctx ginkgo.SpecContext) {
			skipUnless(v.Get != nil && v.Seed != nil, "Get and Seed")

			seeded := v.Seed(ctx, h.NewParent(ctx))

			found, err := v.Get(ctx, h.NameOf(seeded), 0)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			expensive(found, v.GetDefault == v.Full)
		})

		ginkgo.It(fmt.Sprintf("takes an unspecified view as %d on a list", v.ListDefault), func(ctx ginkgo.SpecContext) {
			skipUnless(v.List != nil && v.Seed != nil, "List and Seed")

			parent := h.NewParent(ctx)
			seeded := v.Seed(ctx, parent)

			expensive(listed(ctx, parent, seeded, 0), v.ListDefault == v.Full)
		})

		ginkgo.It("rejects a view outside the enum", func(ctx ginkgo.SpecContext) {
			skipUnless(v.Get != nil, "Get")

			_, err := v.Get(ctx, h.AbsentName(h.NewParent(ctx)), 99)

			h.ExpectError(err, codes.InvalidArgument, "view")
		})
	})
}
