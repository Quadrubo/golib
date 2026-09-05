package contract

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/quadrubo/golib/testkit/matchers"
)

func describeUpdate[R proto.Message](h *harness[R]) {
	if h.Update == nil || h.Create == nil {
		return
	}

	ginkgo.Describe("Update", func() {
		for _, fd := range h.Writable {
			ginkgo.It("writes the "+string(fd.Name())+" the mask selects and nothing else", func(ctx ginkgo.SpecContext) {
				skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

				base := h.Make(ctx, h.NewParent(ctx), h.Minimal())
				full := h.Full()

				updated, err := h.Update(ctx,
					setValue(h.Patch(base), fd, valueOf(full, fd)), []string{string(fd.Name())})

				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(updated).To(gomega.BeComparableTo(full, h.Comparing(fd)...))
				gomega.Expect(updated).To(gomega.BeComparableTo(base, h.Comparing(without(h.Writable, fd)...)...))
				gomega.Expect(h.NameOf(updated)).To(gomega.Equal(h.NameOf(base)))
				gomega.Expect(h.EtagOf(updated)).ToNot(gomega.Equal(h.EtagOf(base)))

				if h.CreateTimeField != nil {
					gomega.Expect(timeOf(updated, h.CreateTimeField)).
						To(gomega.BeTemporally("==", timeOf(base, h.CreateTimeField)))
				}

				if h.UpdateTimeField != nil {
					gomega.Expect(timeOf(updated, h.UpdateTimeField)).
						To(gomega.BeTemporally(">", timeOf(base, h.UpdateTimeField)))
				}

				found := h.Fetch(ctx, h.NameOf(base))
				gomega.Expect(found).To(gomega.BeComparableTo(full, h.Comparing(fd)...))
				gomega.Expect(found).To(gomega.BeComparableTo(base, h.Comparing(without(h.Writable, fd)...)...))
			})
		}

		for _, fd := range h.Optional {
			ginkgo.It("clears the "+string(fd.Name())+" the mask selects", func(ctx ginkgo.SpecContext) {
				skipUnless(h.Full != nil, "Full")

				base := h.Make(ctx, h.NewParent(ctx), h.Full())

				updated, err := h.Update(ctx,
					clearValue(h.Identified(h.Full(), base), fd), []string{string(fd.Name())})

				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(has(updated, fd)).To(gomega.BeFalse())
				gomega.Expect(has(h.Fetch(ctx, h.NameOf(base)), fd)).To(gomega.BeFalse())
			})
		}

		ginkgo.It("writes every writable field without a mask", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

			base := h.Make(ctx, h.NewParent(ctx), h.Minimal())
			full := h.Full()

			updated, err := h.Update(ctx, h.Identified(full, base), nil)

			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(updated).To(gomega.BeComparableTo(full, h.Comparing(h.Writable...)...))
		})

		for _, fd := range h.Immutable {
			if _, ok := differing(fd, fd.Default()); !ok {
				continue
			}

			ginkgo.It("rejects a changed "+string(fd.Name()), func(ctx ginkgo.SpecContext) {
				base := h.Fixture(ctx, h.NewParent(ctx))

				changed, ok := differing(fd, valueOf(base, fd))
				gomega.Expect(ok).To(gomega.BeTrue())

				_, err := h.Update(ctx, setValue(h.Patch(base), fd, changed), []string{string(fd.Name())})

				h.ExpectError(err, codes.InvalidArgument, h.Singular+"."+string(fd.Name()))
			})

			ginkgo.It("accepts an unchanged "+string(fd.Name()), func(ctx ginkgo.SpecContext) {
				base := h.Fixture(ctx, h.NewParent(ctx))

				patch := clearValue(h.Patch(base), fd)
				if has(base, fd) {
					patch = setValue(patch, fd, valueOf(base, fd))
				}

				_, err := h.Update(ctx, patch, []string{string(fd.Name())})

				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		}

		ginkgo.It("names update_mask on an InvalidArgument for a path no field carries", func(ctx ginkgo.SpecContext) {
			_, err := h.Update(ctx, h.Patch(h.Fixture(ctx, h.NewParent(ctx))), []string{"colour"})

			h.ExpectError(err, codes.InvalidArgument, "update_mask")
			gomega.Expect(err).To(matchers.HaveErrorMetadata("update_mask", "colour"))
		})

		if len(h.OutputOnly) > 0 {
			fd := h.OutputOnly[0]

			ginkgo.It("names update_mask on an InvalidArgument for the output-only "+string(fd.Name()),
				func(ctx ginkgo.SpecContext) {
					_, err := h.Update(ctx, h.Patch(h.Fixture(ctx, h.NewParent(ctx))), []string{string(fd.Name())})

					h.ExpectError(err, codes.InvalidArgument, "update_mask")
				})
		}

		ginkgo.It("names update_mask on an InvalidArgument for the name", func(ctx ginkgo.SpecContext) {
			_, err := h.Update(ctx, h.Patch(h.Fixture(ctx, h.NewParent(ctx))), []string{"name"})

			h.ExpectError(err, codes.InvalidArgument, "update_mask")
		})

		ginkgo.It("names the name on an InvalidArgument for a malformed name", func(ctx ginkgo.SpecContext) {
			_, err := h.Update(ctx, h.Addressed(h.Blank(), h.MalformedName(h.NewParent(ctx)), "etag"), h.AnyMask())

			h.ExpectError(err, codes.InvalidArgument, h.Singular+".name")
		})

		ginkgo.It("returns a NotFound for a resource that does not exist", func(ctx ginkgo.SpecContext) {
			_, err := h.Update(ctx, h.Addressed(h.Blank(), h.AbsentName(h.NewParent(ctx)), "etag"), h.AnyMask())

			h.ExpectError(err, codes.NotFound, "")
		})

		ginkgo.It("aborts on an etag another write outdated", func(ctx ginkgo.SpecContext) {
			base := h.Fixture(ctx, h.NewParent(ctx))

			_, err := h.Update(ctx, h.Patch(base), h.AnyMask())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			_, err = h.Update(ctx, h.Patch(base), h.AnyMask())

			h.ExpectEtagMismatchError(err)
		})

		ginkgo.It("rejects an update without an etag", func(ctx ginkgo.SpecContext) {
			base := h.Fixture(ctx, h.NewParent(ctx))

			_, err := h.Update(ctx, h.Addressed(h.Blank(), h.NameOf(base), ""), h.AnyMask())

			h.ExpectError(err, codes.InvalidArgument, "")
		})
	})
}
