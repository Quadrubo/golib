package contract

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Resource describes how the specs reach one resource. A nil closure skips
// the specs that need it, so a read-only resource sets Get and List alone.
// Every closure takes the context of the spec calling it.
type Resource[R proto.Message] struct {
	// ErrorDomain is the domain every error carries on its ErrorInfo.
	ErrorDomain string

	IDs IDs

	// Parent stays nil for a top-level collection.
	Parent *Parent

	Fixtures   Fixtures[R]
	Methods    Methods[R]
	Collection Collection

	SoftDelete *SoftDelete[R]
	Revisions  *Revisions[R]
	Batch      *Batch[R]
	Views      *Views[R]
}

// IDs describes the ids of a collection that departs from the lowercase
// client-supplied id AIP-133 gives a create.
type IDs struct {
	// ServerAssigned opts out of the client-supplied id, for a collection
	// whose ids the server assigns alone.
	ServerAssigned bool

	// Absent is a well-formed id no resource carries, for a collection whose
	// ids follow another pattern than the lowercase one.
	Absent string

	// Generated asserts on an id the server assigned, for a service that
	// guarantees a format.
	Generated func(id string)
}

// Parent describes the parent a child collection sits under.
type Parent struct {
	// New returns a fresh parent for one spec.
	New func(ctx context.Context) string

	// Delete deletes the parent, which takes the resources along.
	Delete func(ctx context.Context, parent string)
}

// Fixtures describes how the specs come by resources.
type Fixtures[R proto.Message] struct {
	// Full returns a resource with every field a create takes set. Minimal
	// returns one with the required fields alone.
	Full    func() R
	Minimal func() R

	// Seed stands in for a resource without a Create, such as a revision a
	// write records.
	Seed func(ctx context.Context, parent string) R

	// Derived seeds a resource whose field carries a value, for a filterable
	// or orderable field no write sets.
	Derived map[string]func(ctx context.Context, parent string) R
}

// Methods holds the standard methods of the resource.
type Methods[R proto.Message] struct {
	// Create creates the resource under the parent, and under the id when
	// the call supplies one.
	Create func(ctx context.Context, parent, id string, resource R) (R, error)
	Get    func(ctx context.Context, name string) (R, error)
	Update func(ctx context.Context, resource R, mask []string) (R, error)
	Delete func(ctx context.Context, name, etag string) error
	List   func(ctx context.Context, parent string, req ListRequest) (Page[R], error)
}

// Collection describes what the List of the resource takes.
type Collection struct {
	// Filters and Orders name the fields the List filters and orders by.
	Filters         []string
	Orders          []string
	LeadingWildcard bool
}

// ListRequest carries what a List call takes besides the parent.
type ListRequest struct {
	Filter      string
	OrderBy     string
	PageSize    int32
	PageToken   string
	ShowDeleted bool
}

// Page is what a List call returns.
type Page[R proto.Message] struct {
	Items         []R
	NextPageToken string
}

// RevisionPage is what a List call over revisions returns.
type RevisionPage struct {
	Items         []proto.Message
	NextPageToken string
}

// SoftDelete describes a resource Delete marks instead of removes.
type SoftDelete[R proto.Message] struct {
	Undelete func(ctx context.Context, name string) (R, error)
}

// Revisions describes a resource whose writes record revisions.
type Revisions[R proto.Message] struct {
	List     func(ctx context.Context, name string, req ListRequest) (RevisionPage, error)
	Get      func(ctx context.Context, name, id string) (proto.Message, error)
	Rollback func(ctx context.Context, name, id string) (R, error)
	Snapshot func(revision proto.Message) R
}

// Batch describes a resource with a batch create.
type Batch[R proto.Message] struct {
	Create   func(ctx context.Context, parent string, items []R) ([]R, error)
	MaxItems int
}

// Views describes a resource whose Get and List take a view.
type Views[R proto.Message] struct {
	Get  func(ctx context.Context, name string, view int32) (R, error)
	List func(ctx context.Context, parent string, req ListRequest, view int32) (Page[R], error)

	// Seed returns a resource whose expensive fields carry values.
	Seed func(ctx context.Context, parent string) R

	// Basic and Full are the views leaving out and filling the expensive
	// fields. GetDefault and ListDefault are what an unspecified view means.
	Basic, Full             int32
	GetDefault, ListDefault int32
	Expensive               []string
}
