package validate

import (
	"errors"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"

	"github.com/quadrubo/golib/fieldmask"
	"github.com/quadrubo/golib/grpcerr"
)

// Masked runs the rules the resource declares against the fields the mask
// selects, which an Update handler owns because IGNORE_ALWAYS on the resource
// stops the interceptor. Violations are reported under the request field the
// resource nests in, so the path matches what the client sent.
func Masked(
	validator protovalidate.Validator,
	msg proto.Message,
	mask fieldmask.Mask,
	field string,
) error {
	err := validator.Validate(msg, protovalidate.WithFilter(fieldmask.Filter(mask)))
	if err == nil {
		return nil
	}

	var invalid *protovalidate.ValidationError
	if !errors.As(err, &invalid) {
		return err
	}

	violations := FieldViolations(invalid)
	for _, violation := range violations {
		violation.Field = field + "." + violation.GetField()
	}

	return grpcerr.InvalidArgument("INVALID_FIELDS", "the request has invalid fields").
		WithFieldViolations(violations...)
}
