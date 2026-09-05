package contract

import (
	"github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/proto"
)

// Describe registers the specs every AIP resource passes, under a container
// named after the collection.
func Describe[R proto.Message](r Resource[R]) bool {
	h := newHarness(r)

	return ginkgo.Describe("Contract "+h.Plural, func() {
		describeFixtures(h)
		describeCreate(h)
		describeGet(h)
		describeUpdate(h)
		describeDelete(h)
		describeList(h)
		describeEtag(h)
		describeSoftDelete(h)
		describeRevisions(h)
		describeBatch(h)
		describeViews(h)
	})
}
