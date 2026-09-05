package etag_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/etag"
)

func TestEtag(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etag Suite")
}

var _ = Describe("Encode", func() {
	updateTime := time.Date(2026, time.August, 3, 12, 0, 0, 123456000, time.UTC)

	It("quotes the value as RFC 7232 requires", func() {
		Expect(etag.Encode(updateTime)).To(MatchRegexp(`^"[A-Za-z0-9_-]+"$`))
	})

	It("changes when the timestamp moves by a microsecond", func() {
		Expect(etag.Encode(updateTime)).
			ToNot(Equal(etag.Encode(updateTime.Add(time.Microsecond))))
	})

	It("drops the nanoseconds below microsecond resolution", func() {
		Expect(etag.Encode(updateTime.Add(999 * time.Nanosecond))).
			To(Equal(etag.Encode(updateTime)))
	})

	It("encodes the instant rather than the zone it is read in", func() {
		Expect(etag.Encode(updateTime.In(time.FixedZone("test", 3600)))).
			To(Equal(etag.Encode(updateTime)))
	})

	It("encodes the instant as eight big-endian bytes of epoch microseconds", func() {
		Expect(etag.Encode(updateTime)).To(Equal(`"AAZYI0cYEkA"`))
	})
})
