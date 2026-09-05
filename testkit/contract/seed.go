package contract

import (
	"context"
	"path"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/quadrubo/golib/testkit"
)

// malformedID violates the lowercase id pattern of every collection.
const malformedID = "UPPER"

// Name returns the resource name the id takes under the parent.
func (h *harness[R]) Name(parent, id string) string {
	if parent == "" {
		return h.Plural + "/" + id
	}

	return parent + "/" + h.Plural + "/" + id
}

func (h *harness[R]) MalformedName(parent string) string {
	return h.Name(parent, malformedID)
}

func (h *harness[R]) AbsentName(parent string) string {
	if h.IDs.Absent != "" {
		return h.Name(parent, h.IDs.Absent)
	}

	return h.Name(parent, testkit.ResourceID("absent"))
}

func malformedParent(parent string) string {
	return path.Dir(parent) + "/" + malformedID
}

func absentParent(parent string) string {
	return path.Dir(parent) + "/" + testkit.ResourceID("absent")
}

// NewParent returns the parent the spec creates under, empty for a top-level
// collection.
func (h *harness[R]) NewParent(ctx context.Context) string {
	ginkgo.GinkgoHelper()

	if h.Parent == nil {
		return ""
	}

	return h.Parent.New(ctx)
}

// NewID returns the id a create supplies, empty when the server assigns them.
func (h *harness[R]) NewID() string {
	if h.IDs.ServerAssigned {
		return ""
	}

	return testkit.ResourceID(h.Singular)
}

// Blank returns the resource a spec creates when its fields do not matter.
func (h *harness[R]) Blank() R {
	if h.Minimal != nil {
		return h.Minimal()
	}

	var zero R

	return zero.ProtoReflect().New().Interface().(R)
}

// CanSeed reports whether the specs can create a resource at all.
func (h *harness[R]) CanSeed() bool {
	return h.Create != nil || h.Seed != nil
}

// Make creates the resource under the parent and fails the spec on an error.
func (h *harness[R]) Make(ctx context.Context, parent string, resource R) R {
	ginkgo.GinkgoHelper()

	created, err := h.Create(ctx, parent, h.NewID(), resource)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return created
}

// Fixture creates a resource under the parent whose fields do not matter.
func (h *harness[R]) Fixture(ctx context.Context, parent string) R {
	ginkgo.GinkgoHelper()

	if h.Create == nil {
		return h.Seed(ctx, parent)
	}

	return h.Make(ctx, parent, h.Blank())
}

// FixtureNames creates the count of fixtures and returns their names.
func (h *harness[R]) FixtureNames(ctx context.Context, parent string, count int) []string {
	ginkgo.GinkgoHelper()

	names := make([]string, 0, count)
	for range count {
		names = append(names, h.NameOf(h.Fixture(ctx, parent)))
	}

	return names
}

// Fetch gets the resource and fails the spec on an error.
func (h *harness[R]) Fetch(ctx context.Context, name string) R {
	ginkgo.GinkgoHelper()

	found, err := h.Get(ctx, name)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return found
}

// Listed lists under the request and fails the spec on an error.
func (h *harness[R]) Listed(ctx context.Context, parent string, req ListRequest) Page[R] {
	ginkgo.GinkgoHelper()

	page, err := h.List(ctx, parent, req)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return page
}

// ListedNames returns the names the scoped list under the request returns.
func (h *harness[R]) ListedNames(ctx context.Context, parent string, req ListRequest, names ...string) []string {
	ginkgo.GinkgoHelper()

	req.Filter = h.Scoped(req.Filter, names...)

	return h.NamesOf(h.Listed(ctx, parent, req).Items)
}

// Filterable reports whether the List filters by the field.
func (h *harness[R]) Filterable(field string) bool {
	for _, declared := range h.Filters {
		if declared == field {
			return true
		}
	}

	return false
}

// Scope returns the filter matching the named resources alone, or an empty
// filter when the List filters by nothing, which a fresh parent scopes then.
func (h *harness[R]) Scope(names ...string) string {
	if !h.Filterable("name") {
		return ""
	}

	terms := make([]string, 0, len(names))
	for _, name := range names {
		terms = append(terms, "name = "+quote(name))
	}

	return "(" + strings.Join(terms, " OR ") + ")"
}

// Scoped joins the scope and the expression with AND.
func (h *harness[R]) Scoped(expression string, names ...string) string {
	scope := h.Scope(names...)
	if scope == "" {
		return expression
	}

	if expression == "" {
		return scope
	}

	return scope + " AND (" + expression + ")"
}

func (h *harness[R]) NamesOf(items []R) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, h.NameOf(item))
	}

	return names
}

func (h *harness[R]) NameOf(r R) string {
	return r.ProtoReflect().Get(h.NameField).String()
}

// IDOf returns the last segment of the resource name.
func (h *harness[R]) IDOf(r R) string {
	return path.Base(h.NameOf(r))
}

// EtagOf returns the etag of the resource, empty for a resource without one.
func (h *harness[R]) EtagOf(r R) string {
	if h.EtagField == nil {
		return ""
	}

	return r.ProtoReflect().Get(h.EtagField).String()
}

// Identified returns a copy of the resource carrying the name and etag of the
// stored one, which a write needs.
func (h *harness[R]) Identified(resource, stored R) R {
	return h.Addressed(resource, h.NameOf(stored), h.EtagOf(stored))
}

// Addressed returns a copy of the resource carrying the name and etag.
func (h *harness[R]) Addressed(resource R, name, etag string) R {
	copied := setValue(resource, h.NameField, protoreflect.ValueOfString(name))

	if h.EtagField == nil {
		return copied
	}

	return setValue(copied, h.EtagField, protoreflect.ValueOfString(etag))
}

// Patch returns a write to the stored resource whose fields do not matter.
func (h *harness[R]) Patch(stored R) R {
	return h.Identified(h.Blank(), stored)
}

// AnyMask returns a mask selecting one writable field, for a write whose
// fields do not matter.
func (h *harness[R]) AnyMask() []string {
	if len(h.Writable) == 0 {
		return nil
	}

	return []string{string(h.Writable[0].Name())}
}
