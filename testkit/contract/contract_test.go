package contract_test

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/apipb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/quadrubo/golib/apperr"
	specv1 "github.com/quadrubo/golib/grpcinterceptor/validate/testdata/spec/v1"
	"github.com/quadrubo/golib/testkit"
	"github.com/quadrubo/golib/testkit/contract"
)

func TestContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contract Suite")
}

const domain = "contract.example.test"

var (
	resourceName = regexp.MustCompile(`^/[a-z0-9-]+$`)
	resourceID   = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// mixins holds a resource in memory whose message declares no etag.
type mixins struct {
	mu    sync.Mutex
	names map[string]bool
}

func (m *mixins) seed(_ context.Context, _ string) *apipb.Mixin {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := "/" + testkit.ResourceID("mixin")
	m.names[name] = true

	return &apipb.Mixin{Name: name}
}

func (m *mixins) get(_ context.Context, name string) (*apipb.Mixin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.check(name); err != nil {
		return nil, err
	}

	return &apipb.Mixin{Name: name}, nil
}

func (m *mixins) delete(_ context.Context, name, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.check(name); err != nil {
		return err
	}

	delete(m.names, name)

	return nil
}

func (m *mixins) batchCreate(_ context.Context, _ string, items []*apipb.Mixin) ([]*apipb.Mixin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(items) == 0 {
		return nil, apperr.InvalidField("EMPTY_BATCH", "requests", "requests", "", errors.New("empty")).
			WithDomain(domain)
	}

	created := make([]*apipb.Mixin, 0, len(items))
	for _, item := range items {
		name := "/" + testkit.ResourceID("mixin")
		m.names[name] = true
		created = append(created, &apipb.Mixin{Name: name, Root: item.GetRoot()})
	}

	return created, nil
}

func (m *mixins) check(name string) error {
	if !resourceName.MatchString(name) {
		return apperr.InvalidResourceName("name", errors.New("malformed"), name).WithDomain(domain)
	}

	if !m.names[name] {
		return apperr.NotFound("MIXIN_MISSING", "mixin", name).WithDomain(domain)
	}

	return nil
}

func mixinResource(policy contract.EtagPolicy) contract.Resource[*apipb.Mixin] {
	store := &mixins{names: map[string]bool{}}

	return contract.Resource[*apipb.Mixin]{
		ErrorDomain: domain,
		Etag:        policy,
		Fixtures:    contract.Fixtures[*apipb.Mixin]{Seed: store.seed},
		Methods: contract.Methods[*apipb.Mixin]{
			Get:    store.get,
			Delete: store.delete,
		},
	}
}

// shelves holds a resource in memory whose message declares an etag.
type shelves struct {
	mu           sync.Mutex
	stored       map[string]*specv1.Shelf
	writes       int
	etagRequired bool
}

func (s *shelves) create(_ context.Context, _, id string, shelf *specv1.Shelf) (*specv1.Shelf, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == "" {
		id = testkit.ResourceID("shelf")
	}

	if !resourceID.MatchString(id) {
		return nil, apperr.InvalidField("INVALID_ID", "shelf_id", "shelf_id", id, errors.New("malformed")).
			WithDomain(domain)
	}

	name := "/" + id
	if _, ok := s.stored[name]; ok {
		return nil, apperr.AlreadyExists("SHELF_EXISTS", "shelf", name).WithDomain(domain)
	}

	return s.store(name, shelf.GetTheme()), nil
}

func (s *shelves) seed(_ context.Context, _ string) *specv1.Shelf {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store("/"+testkit.ResourceID("shelf"), "")
}

func (s *shelves) get(_ context.Context, name string) (*specv1.Shelf, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, err := s.find("name", name)
	if err != nil {
		return nil, err
	}

	return proto.Clone(stored).(*specv1.Shelf), nil
}

func (s *shelves) update(_ context.Context, shelf *specv1.Shelf, mask []string) (*specv1.Shelf, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, err := s.find("shelf.name", shelf.GetName())
	if err != nil {
		return nil, err
	}

	for _, path := range mask {
		if path != "theme" {
			return nil, apperr.InvalidUpdateMask(errors.New("unsupported path"), &fieldmaskpb.FieldMask{Paths: mask}).
				WithDomain(domain)
		}
	}

	if err := s.checkEtag(stored, shelf.GetEtag()); err != nil {
		return nil, err
	}

	return s.store(stored.GetName(), shelf.GetTheme()), nil
}

func (s *shelves) delete(_ context.Context, name, etag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, err := s.find("name", name)
	if err != nil {
		return err
	}

	if err := s.checkEtag(stored, etag); err != nil {
		return err
	}

	delete(s.stored, name)

	return nil
}

func (s *shelves) store(name, theme string) *specv1.Shelf {
	s.writes++

	stored := &specv1.Shelf{Name: name, Theme: theme, Etag: strconv.Itoa(s.writes)}
	s.stored[name] = stored

	return proto.Clone(stored).(*specv1.Shelf)
}

func (s *shelves) find(field, name string) (*specv1.Shelf, error) {
	if !resourceName.MatchString(name) {
		return nil, apperr.InvalidResourceName(field, errors.New("malformed"), name).WithDomain(domain)
	}

	stored, ok := s.stored[name]
	if !ok {
		return nil, apperr.NotFound("SHELF_MISSING", "shelf", name).WithDomain(domain)
	}

	return stored, nil
}

func (s *shelves) checkEtag(stored *specv1.Shelf, etag string) error {
	switch {
	case etag == "" && s.etagRequired:
		return apperr.InvalidField("ETAG_MISSING", "etag", "etag", "", errors.New("missing")).WithDomain(domain)
	case etag != "" && etag != stored.GetEtag():
		return apperr.EtagMismatch("shelf", stored.GetName()).WithDomain(domain)
	default:
		return nil
	}
}

func shelfResource(policy contract.EtagPolicy) contract.Resource[*specv1.Shelf] {
	store := &shelves{stored: map[string]*specv1.Shelf{}, etagRequired: policy == contract.EtagRequired}

	return contract.Resource[*specv1.Shelf]{
		ErrorDomain: domain,
		Etag:        policy,
		Fixtures: contract.Fixtures[*specv1.Shelf]{
			Full:    func() *specv1.Shelf { return &specv1.Shelf{Theme: "poetry"} },
			Minimal: func() *specv1.Shelf { return &specv1.Shelf{} },
		},
		Methods: contract.Methods[*specv1.Shelf]{
			Create: store.create,
			Get:    store.get,
			Update: store.update,
			Delete: store.delete,
		},
	}
}

var _ = Describe("EtagRequired", func() {
	contract.Describe(shelfResource(contract.EtagRequired))
})

var _ = Describe("EtagRequired on a read-only message without an etag field", func() {
	resource := mixinResource(contract.EtagRequired)
	resource.Methods.Delete = nil

	contract.Describe(resource)
})

var _ = Describe("EtagOptional", func() {
	contract.Describe(shelfResource(contract.EtagOptional))
})

var _ = Describe("EtagNone", func() {
	contract.Describe(mixinResource(contract.EtagNone))
})

var _ = Describe("EtagNone with a batch create", func() {
	store := &mixins{names: map[string]bool{}}

	contract.Describe(contract.Resource[*apipb.Mixin]{
		ErrorDomain: domain,
		Etag:        contract.EtagNone,
		Fixtures: contract.Fixtures[*apipb.Mixin]{
			Full:    func() *apipb.Mixin { return &apipb.Mixin{Root: "shelves"} },
			Minimal: func() *apipb.Mixin { return &apipb.Mixin{} },
		},
		Batch: &contract.Batch[*apipb.Mixin]{Create: store.batchCreate},
	})
})

var _ = Describe("A resource without Create that takes updates", func() {
	store := &shelves{stored: map[string]*specv1.Shelf{}, etagRequired: true}

	contract.Describe(contract.Resource[*specv1.Shelf]{
		ErrorDomain: domain,
		Fixtures: contract.Fixtures[*specv1.Shelf]{
			Seed:    store.seed,
			Full:    func() *specv1.Shelf { return &specv1.Shelf{Theme: "poetry"} },
			Minimal: func() *specv1.Shelf { return &specv1.Shelf{} },
		},
		Methods: contract.Methods[*specv1.Shelf]{
			Get:    store.get,
			Update: store.update,
		},
	})
})

// volumes holds a resource in memory whose Delete sets a delete time.
type volumes struct {
	mu      sync.Mutex
	volumes map[string]*specv1.Volume
}

func (v *volumes) seed(_ context.Context, _ string) *specv1.Volume {
	v.mu.Lock()
	defer v.mu.Unlock()

	volume := &specv1.Volume{Name: "/" + testkit.ResourceID("volume")}
	v.volumes[volume.GetName()] = volume

	return proto.Clone(volume).(*specv1.Volume)
}

func (v *volumes) get(_ context.Context, name string) (*specv1.Volume, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	volume, err := v.find(name)
	if err != nil {
		return nil, err
	}

	return proto.Clone(volume).(*specv1.Volume), nil
}

func (v *volumes) delete(_ context.Context, name, _ string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	volume, err := v.find(name)
	if err != nil {
		return err
	}

	if volume.GetDeleteTime() != nil {
		return apperr.NotFound("VOLUME_MISSING", "volume", name).WithDomain(domain)
	}

	volume.DeleteTime = timestamppb.Now()

	return nil
}

func (v *volumes) undelete(_ context.Context, name string) (*specv1.Volume, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	volume, err := v.find(name)
	if err != nil {
		return nil, err
	}

	if volume.GetDeleteTime() == nil {
		return nil, apperr.AlreadyExists("VOLUME_EXISTS", "volume", name).WithDomain(domain)
	}

	volume.DeleteTime = nil

	return proto.Clone(volume).(*specv1.Volume), nil
}

func (v *volumes) find(name string) (*specv1.Volume, error) {
	if !resourceName.MatchString(name) {
		return nil, apperr.InvalidResourceName("name", errors.New("malformed"), name).WithDomain(domain)
	}

	volume, ok := v.volumes[name]
	if !ok {
		return nil, apperr.NotFound("VOLUME_MISSING", "volume", name).WithDomain(domain)
	}

	return volume, nil
}

var _ = Describe("A resource without Create", func() {
	store := &volumes{volumes: map[string]*specv1.Volume{}}

	contract.Describe(contract.Resource[*specv1.Volume]{
		ErrorDomain: domain,
		Etag:        contract.EtagNone,
		Fixtures:    contract.Fixtures[*specv1.Volume]{Seed: store.seed},
		Methods: contract.Methods[*specv1.Volume]{
			Get:    store.get,
			Delete: store.delete,
		},
		SoftDelete: &contract.SoftDelete[*specv1.Volume]{Undelete: store.undelete},
	})
})
