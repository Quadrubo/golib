package contract

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/quadrubo/golib/testkit"
)

func describeCreate[R proto.Message](h *harness[R]) {
	if h.Create == nil {
		return
	}

	ginkgo.Describe("Create", func() {
		ginkgo.It("stores every field the call sets", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Full != nil, "Full")

			parent := h.NewParent(ctx)
			full := h.Full()

			created := h.Make(ctx, parent, full)

			gomega.Expect(created).To(gomega.BeComparableTo(full, h.Comparing(h.Creatable...)...))
			gomega.Expect(h.NameOf(created)).To(gomega.HavePrefix(h.Name(parent, "")))

			if h.EtagField != nil {
				gomega.Expect(h.EtagOf(created)).ToNot(gomega.BeEmpty())
			}

			h.ExpectFreshTimestamps(created)
		})

		ginkgo.It("stores the required fields alone", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Minimal != nil, "Minimal")

			created := h.Make(ctx, h.NewParent(ctx), h.Minimal())

			for _, fd := range h.Optional {
				gomega.Expect(has(created, fd)).To(gomega.BeFalse(), "%s came back set", fd.Name())
			}

			h.ExpectFreshTimestamps(created)
		})

		for _, fd := range h.Required {
			if !enforceable(fd) {
				continue
			}

			ginkgo.It("rejects a resource without "+string(fd.Name()), func(ctx ginkgo.SpecContext) {
				skipUnless(h.Minimal != nil, "Minimal")

				_, err := h.Create(ctx, h.NewParent(ctx), h.NewID(), clearValue(h.Minimal(), fd))

				h.ExpectError(err, codes.InvalidArgument, h.Singular+"."+string(fd.Name()))
			})
		}

		ginkgo.It("ignores the name the resource carries", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)
			carried := h.Name(parent, testkit.ResourceID("carried"))

			created := h.Make(ctx, parent, setValue(h.Blank(), h.NameField, protoreflect.ValueOfString(carried)))

			gomega.Expect(h.NameOf(created)).ToNot(gomega.Equal(carried))
		})

		for _, fd := range h.OutputOnly {
			if _, ok := differing(fd, fd.Default()); !ok {
				continue
			}

			ginkgo.It("ignores the "+string(fd.Name())+" the resource carries", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)

				// The reference create shows the value the server derives.
				// The garbage differs from that value, so a derived value
				// never matches the garbage by chance.
				reference := h.Make(ctx, parent, h.Blank())

				garbage, ok := differing(fd, valueOf(reference, fd))
				gomega.Expect(ok).To(gomega.BeTrue())

				sent := setValue(h.Blank(), fd, garbage)
				created := h.Make(ctx, parent, sent)

				gomega.Expect(created).ToNot(gomega.BeComparableTo(sent, h.Comparing(fd)...))
				gomega.Expect(h.Fetch(ctx, h.NameOf(created))).ToNot(gomega.BeComparableTo(sent, h.Comparing(fd)...))
			})
		}

		ginkgo.It("generates an id of its own when the call supplies none", func(ctx ginkgo.SpecContext) {
			parent := h.NewParent(ctx)

			first, err := h.Create(ctx, parent, "", h.Blank())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			second, err := h.Create(ctx, parent, "", h.Blank())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(h.IDOf(first)).ToNot(gomega.BeEmpty())
			gomega.Expect(h.IDOf(first)).ToNot(gomega.Equal(h.IDOf(second)))

			if h.IDs.Generated != nil {
				h.IDs.Generated(h.IDOf(first))
			}
		})

		if !h.IDs.ServerAssigned {
			ginkgo.It("creates the resource under the id the call supplies", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				id := testkit.ResourceID(h.Singular)

				created, err := h.Create(ctx, parent, id, h.Blank())

				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(h.NameOf(created)).To(gomega.Equal(h.Name(parent, id)))
			})

			ginkgo.It("returns an AlreadyExists for an id a resource already carries", func(ctx ginkgo.SpecContext) {
				parent := h.NewParent(ctx)
				id := testkit.ResourceID(h.Singular)

				_, err := h.Create(ctx, parent, id, h.Blank())
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				_, err = h.Create(ctx, parent, id, h.Blank())

				h.ExpectError(err, codes.AlreadyExists, "")
			})

			ginkgo.It("rejects an id outside the id pattern", func(ctx ginkgo.SpecContext) {
				_, err := h.Create(ctx, h.NewParent(ctx), malformedID, h.Blank())

				h.ExpectError(err, codes.InvalidArgument, h.Singular+"_id")
			})
		}

		if h.Parent != nil {
			ginkgo.It("names parent on an InvalidArgument for a malformed parent", func(ctx ginkgo.SpecContext) {
				_, err := h.Create(ctx, malformedParent(h.NewParent(ctx)), h.NewID(), h.Blank())

				h.ExpectError(err, codes.InvalidArgument, "parent")
			})

			ginkgo.It("returns a NotFound for a parent that does not exist", func(ctx ginkgo.SpecContext) {
				_, err := h.Create(ctx, absentParent(h.NewParent(ctx)), h.NewID(), h.Blank())

				h.ExpectError(err, codes.NotFound, "")
			})
		}
	})
}
