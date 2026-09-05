package contract

import (
	"sync"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// racers is the number of writes racing on one etag.
const racers = 4

func describeEtag[R proto.Message](h *harness[R]) {
	if h.Create == nil || h.EtagField == nil {
		return
	}

	ginkgo.Describe("etag", func() {
		ginkgo.It("is the same etag on the create, the get and the list", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Get != nil && h.List != nil, "Get and List")

			parent := h.NewParent(ctx)
			created := h.Fixture(ctx, parent)

			gomega.Expect(h.EtagOf(h.Fetch(ctx, h.NameOf(created)))).To(gomega.Equal(h.EtagOf(created)))

			listed := h.Listed(ctx, parent, ListRequest{Filter: h.Scope(h.NameOf(created))})
			gomega.Expect(listed.Items).To(gomega.HaveLen(1))
			gomega.Expect(h.EtagOf(listed.Items[0])).To(gomega.Equal(h.EtagOf(created)))
		})

		ginkgo.It("lets exactly one of the writes racing on it through", func(ctx ginkgo.SpecContext) {
			skipUnless(h.Update != nil, "Update")

			base := h.Fixture(ctx, h.NewParent(ctx))

			var (
				wg   sync.WaitGroup
				mu   sync.Mutex
				errs []error
			)

			for range racers {
				wg.Go(func() {
					_, err := h.Update(ctx, h.Patch(base), h.AnyMask())

					mu.Lock()
					defer mu.Unlock()

					errs = append(errs, err)
				})
			}

			wg.Wait()

			succeeded := 0
			for _, err := range errs {
				if err == nil {
					succeeded++

					continue
				}

				gomega.Expect(status.Code(err)).To(gomega.Equal(codes.Aborted), "the status is %v", err)
			}

			gomega.Expect(succeeded).To(gomega.Equal(1))
		})
	})
}
