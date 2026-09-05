package validate

import (
	"context"
	"fmt"

	"buf.build/go/protovalidate"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
)

// Module provides the protovalidate.Validator the interceptor and every
// handler share.
func Module() app.Module { return &module{} }

type module struct{}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "validate" }

// Provide builds the one validator the interceptor and every handler share,
// which compiles the rules of a message once however many callers validate it.
func (m *module) Provide(_ context.Context, i do.Injector) error {
	validator, err := protovalidate.New()
	if err != nil {
		return fmt.Errorf("validate: failed to build the validator: %w", err)
	}

	do.ProvideValue(i, validator)

	return nil
}
