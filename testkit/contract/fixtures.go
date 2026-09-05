package contract

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
)

// describeFixtures checks the fixtures the field specs build on.
func describeFixtures[R proto.Message](h *harness[R]) {
	ginkgo.Describe("fixtures", func() {
		ginkgo.It("declares a resource with a name", func() {
			gomega.Expect(h.NameField).ToNot(gomega.BeNil(), "the message declares no name field")
		})

		ginkgo.It("declares an etag on a resource that takes writes", func() {
			skipUnless(h.Update != nil || h.Delete != nil, "Update or Delete")

			gomega.Expect(h.EtagField).ToNot(gomega.BeNil(), "the message declares no etag field")
		})

		ginkgo.It("sets every field a create takes in Full", func() {
			skipUnless(h.Full != nil, "Full")

			full := h.Full()
			for _, fd := range h.Creatable {
				gomega.Expect(has(full, fd)).To(gomega.BeTrue(), "Full leaves %s unset", fd.Name())
			}
		})

		ginkgo.It("sets the required fields alone in Minimal", func() {
			skipUnless(h.Minimal != nil, "Minimal")

			minimal := h.Minimal()
			for _, fd := range h.Required {
				gomega.Expect(has(minimal, fd)).To(gomega.BeTrue(), "Minimal leaves %s unset", fd.Name())
			}

			for _, fd := range h.Optional {
				gomega.Expect(has(minimal, fd)).To(gomega.BeFalse(), "Minimal sets %s", fd.Name())
			}
		})

		ginkgo.It("differs between Full and Minimal in every field a create takes", func() {
			skipUnless(h.Full != nil && h.Minimal != nil, "Full and Minimal")

			full, minimal := h.Full(), h.Minimal()
			for _, fd := range h.Creatable {
				gomega.Expect(full).ToNot(gomega.BeComparableTo(minimal, h.Comparing(fd)...),
					"Full and Minimal share %s", fd.Name())
			}
		})
	})
}
