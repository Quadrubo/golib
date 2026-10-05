package contract

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/apipb"

	specv1 "github.com/quadrubo/golib/grpcinterceptor/validate/testdata/spec/v1"
)

var _ = ginkgo.Describe("etagPolicy", func() {
	get := func(context.Context, string) (*apipb.Mixin, error) { return nil, nil }
	update := func(context.Context, *apipb.Mixin, []string) (*apipb.Mixin, error) { return nil, nil }
	remove := func(context.Context, string, string) error { return nil }

	ginkgo.DescribeTable("runs the default policy of a message without an etag field",
		func(methods Methods[*apipb.Mixin], expected EtagPolicy) {
			h := newHarness(Resource[*apipb.Mixin]{Methods: methods})

			gomega.Expect(h.etagPolicy()).To(gomega.Equal(expected))
		},
		ginkgo.Entry("as EtagNone without Update and Delete", Methods[*apipb.Mixin]{Get: get}, EtagNone),
		ginkgo.Entry("as EtagRequired with Delete", Methods[*apipb.Mixin]{Get: get, Delete: remove}, EtagRequired),
		ginkgo.Entry("as EtagRequired with Update", Methods[*apipb.Mixin]{Get: get, Update: update}, EtagRequired),
	)

	ginkgo.It("runs the default policy of a message with an etag field as EtagRequired", func() {
		h := newHarness(Resource[*specv1.Shelf]{})

		gomega.Expect(h.etagPolicy()).To(gomega.Equal(EtagRequired))
	})

	ginkgo.It("runs a declared policy as declared", func() {
		h := newHarness(Resource[*apipb.Mixin]{Etag: EtagNone, Methods: Methods[*apipb.Mixin]{Delete: remove}})

		gomega.Expect(h.etagPolicy()).To(gomega.Equal(EtagNone))
	})
})
