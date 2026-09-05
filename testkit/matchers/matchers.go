package matchers

import (
	"fmt"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

// HaveViolatedField matches an error whose BadRequest detail names the field.
func HaveViolatedField(field string) types.GomegaMatcher {
	return &detailMatcher{
		expectation: fmt.Sprintf("to carry a BadRequest naming the field %q", field),
		match: func(st *status.Status) (bool, string) {
			bad, ok := detailOf[*errdetails.BadRequest](st)
			if !ok {
				return false, "the status carries no BadRequest"
			}

			fields := make([]string, 0, len(bad.GetFieldViolations()))
			for _, violation := range bad.GetFieldViolations() {
				if violation.GetField() == field {
					return true, ""
				}

				fields = append(fields, violation.GetField())
			}

			return false, fmt.Sprintf("the BadRequest names %q", fields)
		},
	}
}

// HaveViolationDescription matches an error whose BadRequest detail carries
// a violation with the description.
func HaveViolationDescription(description string) types.GomegaMatcher {
	return &detailMatcher{
		expectation: fmt.Sprintf("to carry a BadRequest with the description %q", description),
		match: func(st *status.Status) (bool, string) {
			bad, ok := detailOf[*errdetails.BadRequest](st)
			if !ok {
				return false, "the status carries no BadRequest"
			}

			descriptions := make([]string, 0, len(bad.GetFieldViolations()))
			for _, violation := range bad.GetFieldViolations() {
				if violation.GetDescription() == description {
					return true, ""
				}

				descriptions = append(descriptions, violation.GetDescription())
			}

			return false, fmt.Sprintf("the BadRequest carries %q", descriptions)
		},
	}
}

// HaveReason matches an error whose ErrorInfo detail carries the reason.
func HaveReason(reason string) types.GomegaMatcher {
	return &detailMatcher{
		expectation: fmt.Sprintf("to carry an ErrorInfo with the reason %q", reason),
		match: func(st *status.Status) (bool, string) {
			info, ok := detailOf[*errdetails.ErrorInfo](st)
			if !ok {
				return false, "the status carries no ErrorInfo"
			}

			if info.GetReason() != reason {
				return false, fmt.Sprintf("the ErrorInfo carries the reason %q", info.GetReason())
			}

			return true, ""
		},
	}
}

// HaveErrorDomain matches an error whose ErrorInfo detail carries the domain.
func HaveErrorDomain(domain string) types.GomegaMatcher {
	return &detailMatcher{
		expectation: fmt.Sprintf("to carry an ErrorInfo with the domain %q", domain),
		match: func(st *status.Status) (bool, string) {
			info, ok := detailOf[*errdetails.ErrorInfo](st)
			if !ok {
				return false, "the status carries no ErrorInfo"
			}

			if info.GetDomain() != domain {
				return false, fmt.Sprintf("the ErrorInfo carries the domain %q", info.GetDomain())
			}

			return true, ""
		},
	}
}

// HaveErrorMetadata matches an error whose ErrorInfo detail maps the key to
// the value.
func HaveErrorMetadata(key, value string) types.GomegaMatcher {
	return &detailMatcher{
		expectation: fmt.Sprintf("to carry an ErrorInfo mapping %q to %q", key, value),
		match: func(st *status.Status) (bool, string) {
			info, ok := detailOf[*errdetails.ErrorInfo](st)
			if !ok {
				return false, "the status carries no ErrorInfo"
			}

			found, ok := info.GetMetadata()[key]
			if !ok {
				return false, fmt.Sprintf("the ErrorInfo metadata has no %q", key)
			}

			if found != value {
				return false, fmt.Sprintf("the ErrorInfo maps %q to %q", key, found)
			}

			return true, ""
		},
	}
}

// detailOf returns the first detail of the status that is a T.
func detailOf[T any](st *status.Status) (T, bool) {
	for _, detail := range st.Details() {
		if typed, ok := detail.(T); ok {
			return typed, true
		}
	}

	var zero T

	return zero, false
}

// detailMatcher runs match over the status of the actual error and keeps what
// match found for the failure message.
type detailMatcher struct {
	expectation string
	match       func(st *status.Status) (ok bool, found string)
	found       string
}

func (m *detailMatcher) Match(actual any) (bool, error) {
	err, ok := actual.(error)
	if !ok {
		return false, fmt.Errorf("matchers: expected an error, got %s", format.Object(actual, 1))
	}

	st, ok := status.FromError(err)
	if !ok {
		return false, fmt.Errorf("matchers: expected a gRPC status, got %s", format.Object(actual, 1))
	}

	ok, m.found = m.match(st)

	return ok, nil
}

func (m *detailMatcher) FailureMessage(actual any) string {
	return format.Message(actual, m.expectation) + ", but " + m.found
}

func (m *detailMatcher) NegatedFailureMessage(actual any) string {
	return format.Message(actual, "not "+m.expectation)
}
