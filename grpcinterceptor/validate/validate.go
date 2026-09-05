package validate

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"github.com/samber/do/v2"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/quadrubo/golib/grpcerr"
)

// Unary returns the interceptor that runs the protovalidate rules a request
// message declares, before the handler sees it.
func Unary(i do.Injector) (grpc.UnaryServerInterceptor, error) {
	validator, err := do.Invoke[protovalidate.Validator](i)
	if err != nil {
		return nil, fmt.Errorf("validate: failed to invoke the validator: %w", err)
	}

	domain, err := do.Invoke[grpcerr.Domain](i)
	if err != nil {
		return nil, fmt.Errorf("validate: failed to invoke the domain: %w", err)
	}

	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (any, error) {
		msg, ok := req.(proto.Message)
		if !ok {
			return next(ctx, req)
		}

		err := validator.Validate(msg)
		if err == nil {
			return next(ctx, req)
		}

		var invalid *protovalidate.ValidationError
		if errors.As(err, &invalid) {
			return nil, grpcerr.InvalidArgument("INVALID_FIELDS", "the request has invalid fields").
				WithDomain(domain).
				WithFieldViolations(FieldViolations(invalid)...)
		}

		// A rule that failed to compile is the server's fault, not the caller's.
		return nil, fmt.Errorf("validate: %w", err)
	}, nil
}

// FieldViolations reports the violations protovalidate collected as the detail
// AIP-193 asks an InvalidArgument to carry.
func FieldViolations(err *protovalidate.ValidationError) []*errdetails.BadRequest_FieldViolation {
	violations := make([]*errdetails.BadRequest_FieldViolation, 0, len(err.Violations))
	for _, violation := range err.Violations {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       protovalidate.FieldPathString(violation.Proto.GetField()),
			Description: violation.Proto.GetMessage(),
		})
	}

	return violations
}
