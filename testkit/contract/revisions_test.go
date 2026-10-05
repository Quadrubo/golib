package contract_test

import (
	"context"
	"errors"
	"strconv"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/apipb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/quadrubo/golib/apperr"
	"github.com/quadrubo/golib/testkit"
	"github.com/quadrubo/golib/testkit/contract"
)

// journals holds a resource in memory whose message declares no etag and
// whose writes record the replaced root as a revision.
type journals struct {
	mu      sync.Mutex
	entries map[string]*journal
}

type journal struct {
	root      string
	revisions []string
}

func (j *journals) create(_ context.Context, _, id string, mixin *apipb.Mixin) (*apipb.Mixin, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if id == "" {
		id = testkit.ResourceID("mixin")
	}

	if !resourceID.MatchString(id) {
		return nil, apperr.InvalidField("INVALID_ID", "mixin_id", "mixin_id", id, errors.New("malformed")).
			WithDomain(domain)
	}

	name := "/" + id
	if _, ok := j.entries[name]; ok {
		return nil, apperr.AlreadyExists("MIXIN_EXISTS", "mixin", name).WithDomain(domain)
	}

	j.entries[name] = &journal{root: mixin.GetRoot()}

	return &apipb.Mixin{Name: name, Root: mixin.GetRoot()}, nil
}

func (j *journals) get(_ context.Context, name string) (*apipb.Mixin, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry, err := j.find("name", name)
	if err != nil {
		return nil, err
	}

	return &apipb.Mixin{Name: name, Root: entry.root}, nil
}

func (j *journals) update(_ context.Context, mixin *apipb.Mixin, mask []string) (*apipb.Mixin, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry, err := j.find("mixin.name", mixin.GetName())
	if err != nil {
		return nil, err
	}

	for _, path := range mask {
		if path != "root" {
			return nil, apperr.InvalidUpdateMask(errors.New("unsupported path"), &fieldmaskpb.FieldMask{Paths: mask}).
				WithDomain(domain)
		}
	}

	entry.revisions = append(entry.revisions, entry.root)
	entry.root = mixin.GetRoot()

	return &apipb.Mixin{Name: mixin.GetName(), Root: entry.root}, nil
}

func (j *journals) list(_ context.Context, name string, req contract.ListRequest) (contract.RevisionPage, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry, err := j.find("parent", name)
	if err != nil {
		return contract.RevisionPage{}, err
	}

	offset := 0
	if req.PageToken != "" {
		if offset, err = strconv.Atoi(req.PageToken); err != nil {
			return contract.RevisionPage{}, apperr.InvalidPageToken(err, req.PageToken).WithDomain(domain)
		}
	}

	ids := make([]int, 0, len(entry.revisions))
	for id := len(entry.revisions); id > 0; id-- {
		ids = append(ids, id)
	}

	ids = ids[min(offset, len(ids)):]

	page := contract.RevisionPage{}
	if req.PageSize > 0 && len(ids) > int(req.PageSize) {
		ids = ids[:req.PageSize]
		page.NextPageToken = strconv.Itoa(offset + int(req.PageSize))
	}

	for _, id := range ids {
		page.Items = append(page.Items, &apipb.Mixin{
			Name: name + "/revisions/" + strconv.Itoa(id), Root: entry.revisions[id-1],
		})
	}

	return page, nil
}

func (j *journals) revision(_ context.Context, name, id string) (proto.Message, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry, err := j.find("name", name)
	if err != nil {
		return nil, err
	}

	number, err := revisionNumber("name", id)
	if err != nil {
		return nil, err
	}

	if number > len(entry.revisions) {
		return nil, apperr.NotFound("REVISION_MISSING", "revision", name).WithDomain(domain)
	}

	return &apipb.Mixin{Name: name + "/revisions/" + id, Root: entry.revisions[number-1]}, nil
}

func (j *journals) rollback(_ context.Context, name, id string) (*apipb.Mixin, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	number, err := revisionNumber("revision_id", id)
	if err != nil {
		return nil, err
	}

	entry, err := j.find("name", name)
	if err != nil {
		return nil, err
	}

	if number > len(entry.revisions) {
		return nil, apperr.NotFound("REVISION_MISSING", "revision", name).WithDomain(domain)
	}

	entry.revisions = append(entry.revisions, entry.root)
	entry.root = entry.revisions[number-1]

	return &apipb.Mixin{Name: name, Root: entry.root}, nil
}

func revisionNumber(field, id string) (int, error) {
	number, err := strconv.Atoi(id)
	if err == nil && number < 1 {
		err = errors.New("below 1")
	}

	if err != nil {
		return 0, apperr.InvalidField("INVALID_REVISION_ID", field, field, id, err).WithDomain(domain)
	}

	return number, nil
}

func (j *journals) find(field, name string) (*journal, error) {
	if !resourceName.MatchString(name) {
		return nil, apperr.InvalidResourceName(field, errors.New("malformed"), name).WithDomain(domain)
	}

	entry, ok := j.entries[name]
	if !ok {
		return nil, apperr.NotFound("MIXIN_MISSING", "mixin", name).WithDomain(domain)
	}

	return entry, nil
}

var _ = Describe("EtagNone with updates and revisions", func() {
	store := &journals{entries: map[string]*journal{}}

	contract.Describe(contract.Resource[*apipb.Mixin]{
		ErrorDomain: domain,
		Etag:        contract.EtagNone,
		Fixtures: contract.Fixtures[*apipb.Mixin]{
			Full:    func() *apipb.Mixin { return &apipb.Mixin{Root: "shelves"} },
			Minimal: func() *apipb.Mixin { return &apipb.Mixin{} },
		},
		Methods: contract.Methods[*apipb.Mixin]{
			Create: store.create,
			Get:    store.get,
			Update: store.update,
		},
		Revisions: &contract.Revisions[*apipb.Mixin]{
			List:     store.list,
			Get:      store.revision,
			Rollback: store.rollback,
			Snapshot: func(revision proto.Message) *apipb.Mixin {
				return &apipb.Mixin{Root: revision.(*apipb.Mixin).GetRoot()}
			},
		},
	})
})
