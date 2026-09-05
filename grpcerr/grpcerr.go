package grpcerr

import (
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

// message is a template only an untyped string constant converts to, so a
// runtime value reaches a message through an Arg alone.
type message string

// reasonPattern is the UPPER_SNAKE format AIP-193 sets for a reason.
var reasonPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*[A-Z0-9]$`)

// keyPattern is the format AIP-193 sets for a metadata key.
var keyPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9_-]+$`)

// placeholderPattern finds a {key} the args did not fill.
var placeholderPattern = regexp.MustCompile(`\{[a-z][a-zA-Z0-9_-]+\}`)

type arg struct{ key, value string }

// Arg carries one value the message names and records the pair in the
// ErrorInfo metadata.
func Arg(key, value string) arg {
	if !keyPattern.MatchString(key) {
		panic(fmt.Sprintf("grpcerr: the metadata key %q is not %s", key, keyPattern))
	}

	return arg{key: key, value: value}
}

// Error copies on every With method, so one declared at package scope is safe
// to derive from.
type Error struct {
	code     codes.Code
	message  string
	reason   string
	domain   Domain
	metadata map[string]string

	resource     *errdetails.ResourceInfo
	badRequest   *errdetails.BadRequest
	precondition *errdetails.PreconditionFailure
}

// build fills each {key} with the quoted value, and panics on an arg the
// message does not name, a {key} no arg fills, or a malformed reason.
func build(code codes.Code, reason string, m message, args []arg) *Error {
	checkReason(reason)

	rendered := string(m)
	var metadata map[string]string
	if len(args) > 0 {
		metadata = make(map[string]string, len(args))
	}

	for _, a := range args {
		placeholder := "{" + a.key + "}"
		if !strings.Contains(rendered, placeholder) {
			panic(fmt.Sprintf("grpcerr: the message %q does not name %s", m, placeholder))
		}

		rendered = strings.ReplaceAll(rendered, placeholder, strconv.Quote(a.value))
		metadata[a.key] = a.value
	}

	if open := placeholderPattern.FindString(rendered); open != "" {
		panic(fmt.Sprintf("grpcerr: no arg fills %s in the message %q", open, m))
	}

	return &Error{code: code, reason: reason, message: rendered, metadata: metadata}
}

func checkReason(reason string) {
	if !reasonPattern.MatchString(reason) || len(reason) > 63 {
		panic(fmt.Sprintf("grpcerr: the reason %q is not UPPER_SNAKE of at most 63 characters", reason))
	}
}

// codes.OK is absent, an error reporting success is a contradiction.

func Canceled(reason string, m message, args ...arg) *Error {
	return build(codes.Canceled, reason, m, args)
}

// codes.Unknown is absent, an error nothing classified is reported as Internal.

func InvalidArgument(reason string, m message, args ...arg) *Error {
	return build(codes.InvalidArgument, reason, m, args)
}

// InvalidArgumentFrom reports a parse error, whose message no constructor
// accepts, and takes the metadata pair for the input it quotes.
func InvalidArgumentFrom(reason string, err error, key, value string) *Error {
	checkReason(reason)
	a := Arg(key, value)

	return &Error{
		code:     codes.InvalidArgument,
		reason:   reason,
		message:  err.Error(),
		metadata: map[string]string{a.key: a.value},
	}
}

func DeadlineExceeded(reason string, m message, args ...arg) *Error {
	return build(codes.DeadlineExceeded, reason, m, args)
}

func NotFound(reason string, m message, args ...arg) *Error {
	return build(codes.NotFound, reason, m, args)
}

func AlreadyExists(reason string, m message, args ...arg) *Error {
	return build(codes.AlreadyExists, reason, m, args)
}

func PermissionDenied(reason string, m message, args ...arg) *Error {
	return build(codes.PermissionDenied, reason, m, args)
}

func ResourceExhausted(reason string, m message, args ...arg) *Error {
	return build(codes.ResourceExhausted, reason, m, args)
}

func FailedPrecondition(reason string, m message, args ...arg) *Error {
	return build(codes.FailedPrecondition, reason, m, args)
}

func Aborted(reason string, m message, args ...arg) *Error {
	return build(codes.Aborted, reason, m, args)
}

func OutOfRange(reason string, m message, args ...arg) *Error {
	return build(codes.OutOfRange, reason, m, args)
}

func Unimplemented(reason string, m message, args ...arg) *Error {
	return build(codes.Unimplemented, reason, m, args)
}

// Internal is what an interceptor returns in place of an error it could not
// classify, so service code returns its own error instead.
func Internal(reason string, m message, args ...arg) *Error {
	return build(codes.Internal, reason, m, args)
}

func Unavailable(reason string, m message, args ...arg) *Error {
	return build(codes.Unavailable, reason, m, args)
}

func DataLoss(reason string, m message, args ...arg) *Error {
	return build(codes.DataLoss, reason, m, args)
}

func Unauthenticated(reason string, m message, args ...arg) *Error {
	return build(codes.Unauthenticated, reason, m, args)
}

func (e *Error) Error() string { return e.message }

func (e *Error) GRPCStatus() *status.Status {
	st := status.New(e.code, e.message)

	st = attach(st, &errdetails.ErrorInfo{
		Reason:   e.reason,
		Domain:   string(e.domain),
		Metadata: e.metadata,
	})

	if e.resource != nil {
		st = attach(st, e.resource)
	}

	if e.badRequest != nil {
		st = attach(st, e.badRequest)
	}

	if e.precondition != nil {
		st = attach(st, e.precondition)
	}

	return st
}

// attach adds one payload at a time, since WithDetails marshals everything it
// is given as a unit and drops the lot when one payload fails.
func attach(st *status.Status, detail proto.Message) *status.Status {
	attached, err := st.WithDetails(protoadapt.MessageV1Of(detail))
	if err != nil {
		return st
	}

	return attached
}

func (e *Error) WithDomain(domain Domain) *Error {
	c := new(*e)
	c.domain = domain

	return c
}

// Metadata adds a metadata value the message does not name.
func (e *Error) Metadata(key, value string) *Error {
	c := new(*e)
	c.metadata = make(map[string]string, len(e.metadata)+1)
	maps.Copy(c.metadata, e.metadata)
	c.metadata[key] = value

	return c
}

// ForResource records the resource in the metadata and on a ResourceInfo.
func (e *Error) ForResource(resourceType, name string) *Error {
	if name == "" {
		return e
	}

	c := e.Metadata("name", name)
	r := new(*c)
	r.resource = &errdetails.ResourceInfo{ResourceType: resourceType, ResourceName: name}

	return r
}

func (e *Error) WithFieldViolations(violations ...*errdetails.BadRequest_FieldViolation) *Error {
	if len(violations) == 0 {
		return e
	}
	c := new(*e)
	c.badRequest = &errdetails.BadRequest{FieldViolations: violations}

	return c
}

// WithPreconditionViolations records the state that blocked the call.
func (e *Error) WithPreconditionViolations(
	violations ...*errdetails.PreconditionFailure_Violation,
) *Error {
	if len(violations) == 0 {
		return e
	}
	c := new(*e)
	c.precondition = &errdetails.PreconditionFailure{Violations: violations}

	return c
}
