package protoconv_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/quadrubo/golib/protoconv"
)

func TestProtoconv(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Protoconv Suite")
}

var _ = Describe("TimeToProto", func() {
	It("returns the instant the time names", func() {
		at := time.Date(2026, time.August, 1, 10, 0, 0, 500, time.UTC)

		Expect(protoconv.TimeToProto(at).AsTime()).To(BeTemporally("==", at))
	})

	It("returns nil for the zero time", func() {
		Expect(protoconv.TimeToProto(time.Time{})).To(BeNil())
	})

	It("keeps the epoch apart from the zero time", func() {
		Expect(protoconv.TimeToProto(time.Unix(0, 0))).ToNot(BeNil())
	})
})

var _ = Describe("TimeFromProto", func() {
	It("returns the instant the timestamp names", func() {
		at := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)

		Expect(protoconv.TimeFromProto(protoconv.TimeToProto(at))).To(HaveValue(BeTemporally("==", at)))
	})

	It("returns nil for a nil timestamp", func() {
		Expect(protoconv.TimeFromProto(nil)).To(BeNil())
	})
})

var _ = Describe("DurationToProto", func() {
	It("returns the duration it was given", func() {
		d := 90 * time.Minute

		Expect(protoconv.DurationToProto(&d).AsDuration()).To(Equal(90 * time.Minute))
	})

	It("returns nil for a nil duration", func() {
		Expect(protoconv.DurationToProto(nil)).To(BeNil())
	})
})

var _ = Describe("DurationFromProto", func() {
	It("keeps the precision the wire carried", func() {
		d := durationpb.New(1500 * time.Millisecond)

		Expect(protoconv.DurationFromProto(d)).To(HaveValue(Equal(1500 * time.Millisecond)))
	})

	It("returns nil for a nil duration", func() {
		Expect(protoconv.DurationFromProto(nil)).To(BeNil())
	})
})
