package contract

import (
	"context"
	"slices"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/quadrubo/golib/testkit/matchers"
)

func orders[R proto.Message](h *harness[R]) {
	if len(h.Orders) == 0 {
		return
	}

	ginkgo.Describe("order_by", func() {
		for _, field := range h.Orders {
			if fd, ok := h.Fields[field]; ok {
				fieldOrders(h, field, fd)
			}
		}

		if len(h.Orders) > 1 {
			ginkgo.It("orders by several fields", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				names := h.FixtureNames(ctx, parent, 2)

				found := h.ListedNames(ctx, parent, ListRequest{OrderBy: strings.Join(h.Orders[:2], ", ")}, names...)

				gomega.Expect(found).To(gomega.ConsistOf(names))
			})

			ginkgo.It("names order_by on an InvalidArgument for mixed directions", func(ctx ginkgo.SpecContext) {
				_, err := h.List(ctx, h.NewParent(ctx), ListRequest{OrderBy: h.Orders[0] + ", " + h.Orders[1] + " desc"})

				h.ExpectError(err, codes.InvalidArgument, "order_by")
			})
		}

		ginkgo.It("names order_by on an InvalidArgument for a field named twice", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, h.NewParent(ctx), ListRequest{OrderBy: h.Orders[0] + ", " + h.Orders[0]})

			h.ExpectError(err, codes.InvalidArgument, "order_by")
		})

		ginkgo.It("names order_by on an InvalidArgument for a field no resource sorts by", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, h.NewParent(ctx), ListRequest{OrderBy: "colour"})

			h.ExpectError(err, codes.InvalidArgument, "order_by")
			gomega.Expect(err).To(matchers.HaveErrorMetadata("order_by", "colour"))
		})

		ginkgo.It("names order_by on an InvalidArgument for a direction that is neither asc nor desc",
			func(ctx ginkgo.SpecContext) {
				_, err := h.List(ctx, h.NewParent(ctx), ListRequest{OrderBy: h.Orders[0] + " sideways"})

				h.ExpectError(err, codes.InvalidArgument, "order_by")
			})
	})
}

// fieldOrders registers the order specs over one field.
func fieldOrders[R proto.Message](h *harness[R], field string, fd protoreflect.FieldDescriptor) {
	ginkgo.Describe(field, func() {
		ginkgo.It("orders ascending and descending as reverses", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			carrier, other := pair(ctx, h, parent, fd)
			names := []string{h.NameOf(carrier), h.NameOf(other)}

			ascending := h.ListedNames(ctx, parent, ListRequest{OrderBy: field}, names...)
			descending := h.ListedNames(ctx, parent, ListRequest{OrderBy: field + " desc"}, names...)

			gomega.Expect(ascending).To(gomega.ConsistOf(names))
			gomega.Expect(descending).To(gomega.Equal(reversed(ascending)))

			if has(other, fd) {
				if before, ok := less(fd, valueOf(other, fd), valueOf(carrier, fd)); ok && before {
					gomega.Expect(ascending).To(gomega.Equal([]string{h.NameOf(other), h.NameOf(carrier)}))
				} else if ok {
					gomega.Expect(ascending).To(gomega.Equal([]string{h.NameOf(carrier), h.NameOf(other)}))
				}
			}
		})

		ginkgo.It("walks equal values by keyset without a repeat", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			names := twins(ctx, h, parent, fd)

			first := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(names...), OrderBy: field, PageSize: 1})
			gomega.Expect(first.Items).To(gomega.HaveLen(1))
			gomega.Expect(first.NextPageToken).ToNot(gomega.BeEmpty())

			second := h.Listed(ctx, parent, ListRequest{
				Filter:    h.Scope(names...),
				OrderBy:   field,
				PageSize:  1,
				PageToken: first.NextPageToken,
			})
			gomega.Expect(second.Items).To(gomega.HaveLen(1))
			gomega.Expect(second.NextPageToken).To(gomega.BeEmpty())

			gomega.Expect(append(h.NamesOf(first.Items), h.NamesOf(second.Items)...)).To(gomega.ConsistOf(names))
		})
	})
}

// twins seeds two resources carrying the same value in the field.
func twins[R proto.Message](
	ctx context.Context,
	h *harness[R],
	parent string,
	fd protoreflect.FieldDescriptor,
) []string {
	ginkgo.GinkgoHelper()

	field := string(fd.Name())

	switch {
	case slices.Contains(h.Creatable, fd):
		skipUnless(h.Full != nil, "Full")

		return []string{h.NameOf(h.Make(ctx, parent, h.Full())), h.NameOf(h.Make(ctx, parent, h.Full()))}
	case h.Derived[field] != nil:
		return []string{h.NameOf(h.Derived[field](ctx, parent)), h.NameOf(h.Derived[field](ctx, parent))}
	default:
		return h.FixtureNames(ctx, parent, 2)
	}
}

func reversed(names []string) []string {
	out := slices.Clone(names)
	slices.Reverse(out)

	return out
}
