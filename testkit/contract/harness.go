package contract

import (
	"github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/proto"
)

type harness[R proto.Message] struct {
	Resource[R]
	Fixtures[R]
	Methods[R]
	Collection
	shape
}

func newHarness[R proto.Message](r Resource[R]) *harness[R] {
	return &harness[R]{
		Resource:   r,
		Fixtures:   r.Fixtures,
		Methods:    r.Methods,
		Collection: r.Collection,
		shape:      newShape[R](),
	}
}

func skipUnless(declared bool, declaration string) {
	ginkgo.GinkgoHelper()

	if !declared {
		ginkgo.Skip("the contract declares no " + declaration)
	}
}
