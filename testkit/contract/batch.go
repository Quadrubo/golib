package contract

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func describeBatch[R proto.Message](h *harness[R]) {
	if h.Batch == nil {
		return
	}

	b := h.Batch

	ginkgo.Describe("batch create", func() {
		ginkgo.It("creates the resources in request order", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

			created, err := b.Create(ctx, h.NewParent(ctx), []R{h.Full(), h.Minimal()})

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(created).To(gomega.HaveLen(2))
			gomega.Expect(created[0]).To(gomega.BeComparableTo(h.Full(), h.Comparing(h.Writable...)...))
			gomega.Expect(created[1]).To(gomega.BeComparableTo(h.Minimal(), h.Comparing(h.Writable...)...))

			for _, item := range created {
				gomega.Expect(h.EtagOf(item)).ToNot(gomega.BeEmpty())
				h.ExpectFreshTimestamps(item)
			}
		})

		for _, fd := range h.Required {
			if !enforceable(fd) {
				continue
			}

			ginkgo.It("creates none of the resources when one lacks "+string(fd.Name()), func(ctx ginkgo.SpecContext) {
				skipUnless(h.Minimal != nil, "Minimal")

				parent := h.NewParent(ctx)

				_, err := b.Create(ctx, parent, []R{h.Minimal(), clearValue(h.Minimal(), fd)})

				h.ExpectError(err, codes.InvalidArgument, fmt.Sprintf("requests[1].%s.%s", h.Singular, fd.Name()))

				if h.List != nil && h.Parent != nil {
					gomega.Expect(h.Listed(ctx, parent, ListRequest{}).Items).To(gomega.BeEmpty())
				}
			})

			break
		}

		ginkgo.It("names requests on an InvalidArgument for an empty batch", func(ctx ginkgo.SpecContext) {
			_, err := b.Create(ctx, h.NewParent(ctx), nil)

			h.ExpectError(err, codes.InvalidArgument, "requests")
		})

		if b.MaxItems > 0 {
			ginkgo.It("names requests on an InvalidArgument for a batch above the maximum", func(ctx ginkgo.SpecContext) {
				items := make([]R, 0, b.MaxItems+1)
				for range b.MaxItems + 1 {
					items = append(items, h.Blank())
				}

				_, err := b.Create(ctx, h.NewParent(ctx), items)

				h.ExpectError(err, codes.InvalidArgument, "requests")
			})
		}

		if h.Parent != nil {
			ginkgo.It("names parent on an InvalidArgument for a malformed parent", func(ctx ginkgo.SpecContext) {
				_, err := b.Create(ctx, malformedParent(h.NewParent(ctx)), []R{h.Blank()})

				h.ExpectError(err, codes.InvalidArgument, "parent")
			})

			ginkgo.It("returns a NotFound for a parent that does not exist", func(ctx ginkgo.SpecContext) {
				_, err := b.Create(ctx, absentParent(h.NewParent(ctx)), []R{h.Blank()})

				h.ExpectError(err, codes.NotFound, "")
			})
		}
	})
}
