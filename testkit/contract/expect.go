package contract

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/quadrubo/golib/testkit/matchers"
)

// ExpectError checks the error against the code, the AIP-193 shape, and
// against the field a BadRequest names when the spec expects one.
func (h *harness[R]) ExpectError(err error, code codes.Code, field string) {
	ginkgo.GinkgoHelper()

	gomega.Expect(err).To(gomega.HaveOccurred())
	gomega.Expect(status.Code(err)).To(gomega.Equal(code), "the status is %v", err)
	gomega.Expect(status.Convert(err).Message()).ToNot(gomega.BeEmpty())
	gomega.Expect(err).To(matchers.HaveErrorDomain(h.ErrorDomain))

	if field != "" {
		gomega.Expect(err).To(matchers.HaveViolatedField(field))
	}
}

func (h *harness[R]) ExpectEtagMismatchError(err error) {
	ginkgo.GinkgoHelper()

	h.ExpectError(err, codes.Aborted, "")
	gomega.Expect(err).To(matchers.HaveReason("ETAG_MISMATCH"))
}

func (h *harness[R]) ExpectFreshTimestamps(created R) {
	ginkgo.GinkgoHelper()

	if h.CreateTimeField == nil {
		return
	}

	createTime := timeOf(created, h.CreateTimeField)
	gomega.Expect(createTime).To(gomega.BeTemporally("~", time.Now(), time.Minute))

	// A resource the server processes after the create carries a moved
	// update_time by the time the spec sees it.
	if h.UpdateTimeField != nil {
		gomega.Expect(timeOf(created, h.UpdateTimeField)).To(gomega.BeTemporally(">=", createTime))
	}

	if h.DeleteTimeField != nil {
		gomega.Expect(has(created, h.DeleteTimeField)).To(gomega.BeFalse())
	}
}
