package contract

import (
	"context"
	"fmt"
	"slices"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/quadrubo/golib/testkit/matchers"
)

func filters[R proto.Message](h *harness[R]) {
	if len(h.Filters) == 0 {
		return
	}

	ginkgo.Describe("filter", func() {
		for _, field := range h.Filters {
			if fd, ok := h.Fields[field]; ok {
				fieldFilters(h, field, fd)
			}
		}

		ginkgo.It("composes restrictions with AND, OR, NOT and parentheses", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Filterable("name"), "name filter")

			parent := h.NewParent(ctx)
			names := h.FixtureNames(ctx, parent, 3)

			found := h.Listed(ctx, parent, ListRequest{Filter: fmt.Sprintf(
				"(name = %s OR name = %s OR name = %s) AND NOT name = %s",
				quote(names[0]), quote(names[1]), quote(names[2]), quote(names[2]))})

			gomega.Expect(h.NamesOf(found.Items)).To(gomega.ConsistOf(names[0], names[1]))
		})

		ginkgo.It("names filter on an InvalidArgument for a field no resource matches on", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, h.NewParent(ctx), ListRequest{Filter: `colour = "red"`})

			h.ExpectError(err, codes.InvalidArgument, "filter")
			gomega.Expect(err).To(matchers.HaveErrorMetadata("filter", `colour = "red"`))
		})

		ginkgo.It("names filter on an InvalidArgument for a filter that does not parse", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, h.NewParent(ctx), ListRequest{Filter: h.Filters[0] + " ="})

			h.ExpectError(err, codes.InvalidArgument, "filter")
		})

		ginkgo.It("names filter on an InvalidArgument for the has operator", func(ctx ginkgo.SpecContext) {
			_, err := h.List(ctx, h.NewParent(ctx), ListRequest{Filter: h.Filters[0] + ":x"})

			h.ExpectError(err, codes.InvalidArgument, "filter")
		})

		for _, fd := range h.OutputOnly {
			if h.Filterable(string(fd.Name())) {
				continue
			}

			ginkgo.It("names filter on an InvalidArgument for the unfilterable "+string(fd.Name()),
				func(ctx ginkgo.SpecContext) {
					_, err := h.List(ctx, h.NewParent(ctx), ListRequest{Filter: fmt.Sprintf("%s = 1", fd.Name())})

					h.ExpectError(err, codes.InvalidArgument, "filter")
				})

			break
		}
	})
}

// fieldFilters registers the filter specs over one field.
func fieldFilters[R proto.Message](h *harness[R], field string, fd protoreflect.FieldDescriptor) {
	if field == "name" {
		nameFilters(h)

		return
	}

	ginkgo.Describe(field, func() {
		ginkgo.It("matches the resource the value belongs to", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			carrier, other := pair(ctx, h, parent, fd)

			found := h.ListedNames(ctx, parent,
				ListRequest{Filter: field + " = " + literal(fd, valueOf(carrier, fd))},
				h.NameOf(carrier), h.NameOf(other))

			gomega.Expect(found).To(gomega.ConsistOf(h.NameOf(carrier)))
		})

		ginkgo.It("orders the values under the comparators", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			carrier, other := pair(ctx, h, parent, fd)

			// A second Full carrying another value stands in for a Minimal
			// that leaves the field unset.
			if !has(other, fd) && slices.Contains(h.Creatable, fd) {
				another, ok := differing(fd, valueOf(carrier, fd))
				if !ok {
					ginkgo.Skip(field + " takes no second value")
				}

				other = h.Make(ctx, parent, setValue(h.Full(), fd, another))
			}

			if !has(other, fd) {
				ginkgo.Skip("the other resource leaves " + field + " unset")
			}

			lo, hi := carrier, other
			if before, ok := less(fd, valueOf(other, fd), valueOf(carrier, fd)); !ok {
				ginkgo.Skip(field + " orders no values")
			} else if before {
				lo, hi = other, carrier
			}

			names := []string{h.NameOf(lo), h.NameOf(hi)}
			loValue := literal(fd, valueOf(lo, fd))
			hiValue := literal(fd, valueOf(hi, fd))

			for comparator, want := range map[string][]string{
				"!= " + hiValue: {h.NameOf(lo)},
				"< " + hiValue:  {h.NameOf(lo)},
				"<= " + loValue: {h.NameOf(lo)},
				"> " + loValue:  {h.NameOf(hi)},
				">= " + hiValue: {h.NameOf(hi)},
			} {
				found := h.ListedNames(ctx, parent, ListRequest{Filter: field + " " + comparator}, names...)
				gomega.Expect(found).To(gomega.ConsistOf(want), "under %s %s", field, comparator)
			}
		})

		if textual(fd) {
			ginkgo.It("matches a prefix under a trailing wildcard", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				carrier, other := pair(ctx, h, parent, fd)
				text := valueOf(carrier, fd).String()

				found := h.ListedNames(ctx, parent,
					ListRequest{Filter: field + " = " + quote(text[:len(text)/2+1]+"*")},
					h.NameOf(carrier), h.NameOf(other))

				gomega.Expect(found).To(gomega.ConsistOf(h.NameOf(carrier)))
			})

			ginkgo.It("takes a leading wildcard as the collection declares", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				carrier, other := pair(ctx, h, parent, fd)
				text := valueOf(carrier, fd).String()
				filter := h.Scoped(field+" = "+quote("*"+text[len(text)/2:]), h.NameOf(carrier), h.NameOf(other))

				page, err := h.List(ctx, parent, ListRequest{Filter: filter})

				if h.LeadingWildcard {
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					gomega.Expect(h.NamesOf(page.Items)).To(gomega.ConsistOf(h.NameOf(carrier)))
				} else {
					h.ExpectError(err, codes.InvalidArgument, "filter")
				}
			})
		}

		if fd.Kind() == protoreflect.EnumKind {
			ginkgo.It("names filter on an InvalidArgument for a value given by number", func(ctx ginkgo.SpecContext) {
				_, err := h.List(ctx, h.NewParent(ctx), ListRequest{Filter: field + " = 1"})

				h.ExpectError(err, codes.InvalidArgument, "filter")
			})
		}
	})
}

func nameFilters[R proto.Message](h *harness[R]) {
	ginkgo.Describe("name", func() {
		ginkgo.It("matches the resource the name belongs to", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			names := h.FixtureNames(ctx, parent, 2)

			found := h.Listed(ctx, parent, ListRequest{Filter: "name = " + quote(names[0])})

			gomega.Expect(h.NamesOf(found.Items)).To(gomega.ConsistOf(names[0]))
		})

		ginkgo.It("names filter on an InvalidArgument for a wildcard", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)

			_, err := h.List(ctx, parent, ListRequest{Filter: "name = " + quote(h.Name(parent, "")+"*")})

			h.ExpectError(err, codes.InvalidArgument, "filter")
		})
	})
}

// pair seeds a resource carrying a value in the field and one carrying
// another value or none, and skips the spec when nothing seeds the field.
func pair[R proto.Message](
	ctx context.Context,
	h *harness[R],
	parent string,
	fd protoreflect.FieldDescriptor,
) (carrier, other R) {
	ginkgo.GinkgoHelper()

	field := string(fd.Name())

	switch {
	case slices.Contains(h.Creatable, fd):
		skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

		return h.Make(ctx, parent, h.Full()), h.Make(ctx, parent, h.Minimal())
	case h.Derived[field] != nil:
		return h.Fetch(ctx, h.NameOf(h.Derived[field](ctx, parent))), h.Fixture(ctx, parent)
	case fd == h.NameField || fd == h.CreateTimeField || fd == h.UpdateTimeField:
		first := h.Fixture(ctx, parent)

		return h.Fixture(ctx, parent), first
	default:
		ginkgo.Skip("no write sets " + field + " and the contract declares no Derived seed for it")

		return carrier, other
	}
}
