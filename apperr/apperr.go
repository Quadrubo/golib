package apperr

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/quadrubo/golib/grpcerr"
)

// InvalidField reports one parse error as a violation of the request field.
func InvalidField(reason, field, key, value string, err error) *grpcerr.Error {
	return grpcerr.InvalidArgumentFrom(reason, err, key, value).
		WithFieldViolations(&errdetails.BadRequest_FieldViolation{
			Field:       field,
			Description: err.Error(),
		})
}

// InvalidResourceName takes the field, since an update names the resource
// under a path such as "book.name".
func InvalidResourceName(field string, err error, name string) *grpcerr.Error {
	return InvalidField("INVALID_RESOURCE_NAME", field, "name", name, err)
}

func InvalidParent(err error, parent string) *grpcerr.Error {
	return InvalidField("INVALID_PARENT", "parent", "parent", parent, err)
}

func InvalidFilter(err error, filter string) *grpcerr.Error {
	return InvalidField("INVALID_FILTER", "filter", "filter", filter, err)
}

func InvalidOrderBy(err error, orderBy string) *grpcerr.Error {
	return InvalidField("INVALID_ORDER_BY", "order_by", "order_by", orderBy, err)
}

func InvalidPageToken(err error, token string) *grpcerr.Error {
	return InvalidField("INVALID_PAGE_TOKEN", "page_token", "page_token", token, err)
}

func InvalidUpdateMask(err error, mask *fieldmaskpb.FieldMask) *grpcerr.Error {
	return InvalidField("INVALID_UPDATE_MASK", "update_mask", "update_mask",
		strings.Join(mask.GetPaths(), ","), err)
}

// ImmutableField reports a mask selecting the immutable field under a changed
// value. field is the path the violation names, such as "book.colour".
func ImmutableField(field, path string) *grpcerr.Error {
	return InvalidField("IMMUTABLE_FIELD", field, "field", path,
		fmt.Errorf("unsupported change of the immutable field %q", path))
}

func InvalidRevisionID(id string) *grpcerr.Error {
	badID := grpcerr.InvalidArgument("INVALID_REVISION_ID",
		"unsupported revision id {revision_id}", grpcerr.Arg("revision_id", id))

	return badID.WithFieldViolations(&errdetails.BadRequest_FieldViolation{
		Field:       "revision_id",
		Description: badID.Error(),
	})
}

// The resource name identifies its collection, so the messages name no type.
// The reason and the ResourceInfo type carry it for a program.

func NotFound(reason, resourceType, name string) *grpcerr.Error {
	return grpcerr.NotFound(reason, "the resource {name} does not exist",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func AlreadyExists(reason, resourceType, name string) *grpcerr.Error {
	return grpcerr.AlreadyExists(reason, "the resource {name} already exists",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func Deleted(reason, resourceType, name string) *grpcerr.Error {
	return grpcerr.FailedPrecondition(reason, "the resource {name} is deleted",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func AlreadyDeleted(reason, resourceType, name string) *grpcerr.Error {
	return grpcerr.NotFound(reason, "the resource {name} is already deleted",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func NotDeleted(reason, resourceType, name string) *grpcerr.Error {
	return grpcerr.AlreadyExists(reason, "the resource {name} is not deleted",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func EtagMismatch(resourceType, name string) *grpcerr.Error {
	return grpcerr.Aborted("ETAG_MISMATCH", "the resource {name} carries a newer etag",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func ConcurrentUpdate(resourceType, name string) *grpcerr.Error {
	return grpcerr.Aborted("CONCURRENT_WRITE",
		"the resource {name} was written between the read and the update",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}

func ConcurrentDelete(resourceType, name string) *grpcerr.Error {
	return grpcerr.Aborted("CONCURRENT_WRITE",
		"the resource {name} was written between the read and the delete",
		grpcerr.Arg("name", name)).
		ForResource(resourceType, name)
}
