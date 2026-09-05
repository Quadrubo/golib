package testkit_test

import (
	"context"
	"errors"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/grpcserver"
	"github.com/quadrubo/golib/logging"
	"github.com/quadrubo/golib/testkit"
)

func TestTestkit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testkit Suite")
}

type fakeResource struct{}

type fakeDep struct {
	startErr error
	settings map[string]any
	started  bool
	stopped  bool
}

func (d *fakeDep) Start(_ context.Context, injector do.Injector) error {
	if d.startErr != nil {
		return d.startErr
	}
	d.started = true
	do.ProvideValue(injector, &fakeResource{})

	return nil
}

func (d *fakeDep) Stop(_ context.Context) error {
	d.stopped = true
	return nil
}

func (d *fakeDep) Settings() map[string]any { return d.settings }

func reflectionOff(settings map[string]any) map[string]any {
	if settings == nil {
		settings = map[string]any{}
	}
	settings["modules"] = map[string]any{
		"grpcserver": map[string]any{"reflection": false},
	}

	return settings
}

func modules(conf app.Module) []app.Module {
	return []app.Module{conf, logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()), grpcserver.Module()}
}

// dying returns from Run at once, which brings the service down before its
// health service ever answers.
type dying struct{}

func (dying) Name() string { return "dying" }

func (dying) Run(context.Context) error { return errors.New("boom") }

func expectReflectionOff(suite *testkit.Suite) {
	GinkgoHelper()

	stream, err := reflectionpb.NewServerReflectionClient(suite.Conn()).ServerReflectionInfo(context.Background())
	Expect(err).ToNot(HaveOccurred())

	// Send races the server's rejection, so the status comes back from Recv.
	_ = stream.Send(&reflectionpb.ServerReflectionRequest{})

	Expect(stream.Recv()).Error().To(MatchError(ContainSubstring("unknown service")))
}

var _ = Describe("Boot", func() {
	ctx := context.Background()

	boot := func(opts testkit.Options) (*testkit.Suite, error) {
		suite, err := testkit.Boot(ctx, opts)
		if suite != nil {
			DeferCleanup(func() { Expect(suite.Stop()).To(Succeed()) })
		}
		return suite, err
	}

	expectReflectionOff := func(suite *testkit.Suite) {
		GinkgoHelper()

		stream, err := reflectionpb.NewServerReflectionClient(suite.Conn()).ServerReflectionInfo(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Send races the server's rejection, so the status comes back from Recv.
		_ = stream.Send(&reflectionpb.ServerReflectionRequest{})

		Expect(stream.Recv()).Error().To(MatchError(ContainSubstring("unknown service")))
	}

	It("serves once it returns", func() {
		suite, err := boot(testkit.Options{Modules: modules})
		Expect(err).ToNot(HaveOccurred())

		resp, err := healthpb.NewHealthClient(suite.Conn()).Check(ctx, &healthpb.HealthCheckRequest{})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.GetStatus()).To(Equal(healthpb.HealthCheckResponse_SERVING))
	})

	It("gives each service its own port", func() {
		first, err := boot(testkit.Options{Modules: modules})
		Expect(err).ToNot(HaveOccurred())

		second, err := boot(testkit.Options{Modules: modules})
		Expect(err).ToNot(HaveOccurred())

		Expect(first.Addr()).ToNot(Equal(second.Addr()))
	})

	It("stops serving on stop", func() {
		suite, err := testkit.Boot(ctx, testkit.Options{Modules: modules})
		Expect(err).ToNot(HaveOccurred())

		conn := suite.Conn()
		Expect(suite.Stop()).To(Succeed())

		_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		Expect(err).To(HaveOccurred())
	})

	It("reports a service that dies before it serves", func() {
		// Health is off, so the wait cannot succeed before the service dies.
		_, err := boot(testkit.Options{
			Modules: func(conf app.Module) []app.Module {
				return append(modules(conf), dying{})
			},
			Settings: map[string]any{
				"modules": map[string]any{"grpcserver": map[string]any{"health": false}},
			},
		})

		Expect(err).To(MatchError(ContainSubstring("stopped before it was ready")))
	})

	It("reports a service that cannot boot", func() {
		_, err := boot(testkit.Options{
			Modules: func(conf app.Module) []app.Module {
				return []app.Module{conf, grpcserver.Module()}
			},
		})

		Expect(err).To(MatchError(ContainSubstring("grpcserver: failed to invoke the logger")))
	})

	It("passes settings through to the service", func() {
		suite, err := boot(testkit.Options{
			Modules:  modules,
			Settings: reflectionOff(nil),
		})
		Expect(err).ToNot(HaveOccurred())

		expectReflectionOff(suite)
	})

	It("merges the dependency settings into the service", func() {
		suite, err := boot(testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{&fakeDep{settings: reflectionOff(nil)}},
		})
		Expect(err).ToNot(HaveOccurred())

		expectReflectionOff(suite)
	})

	It("lets the caller's settings override a dependency's", func() {
		suite, err := boot(testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{&fakeDep{settings: reflectionOff(nil)}},
			Settings: map[string]any{
				"modules": map[string]any{
					"grpcserver": map[string]any{"reflection": true},
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())

		stream, err := reflectionpb.NewServerReflectionClient(suite.Conn()).ServerReflectionInfo(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(stream.Send(&reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
		})).To(Succeed())

		Expect(stream.Recv()).Error().ToNot(HaveOccurred())
	})

	It("holds what a dependency provided in the injector", func() {
		suite, err := boot(testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{&fakeDep{}},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(do.Invoke[*fakeResource](suite.Injector())).ToNot(BeNil())
	})

	It("stops the dependencies with the service", func() {
		dep := &fakeDep{}

		suite, err := testkit.Boot(ctx, testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{dep},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(suite.Stop()).To(Succeed())
		Expect(dep.stopped).To(BeTrue())
	})

	It("takes the dependencies down when the service cannot boot", func() {
		dep := &fakeDep{}

		_, err := testkit.Boot(ctx, testkit.Options{
			Modules: func(conf app.Module) []app.Module {
				return []app.Module{conf, grpcserver.Module()}
			},
			Dependencies: []testkit.Dependency{dep},
		})

		Expect(err).To(HaveOccurred())
		Expect(dep.stopped).To(BeTrue())
	})

	It("takes down the dependencies that came up when another fails", func() {
		good := &fakeDep{}
		bad := &fakeDep{startErr: errors.New("no image")}

		_, err := testkit.Boot(ctx, testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{good, bad},
		})

		Expect(err).To(MatchError(ContainSubstring("failed to start")))
		Expect(good.stopped).To(BeTrue())
	})
})

var _ = Describe("Rebooted", func() {
	ctx := context.Background()

	modules := func(conf app.Module) []app.Module {
		return []app.Module{conf, logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()), grpcserver.Module()}
	}

	reboot := func(suite *testkit.Suite, overrides map[string]any) (*testkit.Suite, error) {
		rebooted, err := suite.Rebooted(ctx, overrides)
		if rebooted != nil {
			DeferCleanup(func() { Expect(rebooted.Stop()).To(Succeed()) })
		}

		return rebooted, err
	}

	boot := func(opts testkit.Options) *testkit.Suite {
		GinkgoHelper()

		suite, err := testkit.Boot(ctx, opts)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(suite.Stop()).To(Succeed()) })

		return suite
	}

	It("serves on a port of its own", func() {
		suite := boot(testkit.Options{Modules: modules})

		rebooted, err := reboot(suite, nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(rebooted.Addr()).ToNot(Equal(suite.Addr()))
		Expect(healthpb.NewHealthClient(rebooted.Conn()).Check(ctx, &healthpb.HealthCheckRequest{})).
			Error().ToNot(HaveOccurred())
	})

	It("overrides one setting and keeps the rest", func() {
		suite := boot(testkit.Options{
			Modules:      modules,
			Dependencies: []testkit.Dependency{&fakeDep{settings: map[string]any{"modules": map[string]any{"grpcserver": map[string]any{"max_receive_bytes": "1MiB"}}}}},
		})

		rebooted, err := reboot(suite, reflectionOff(nil))
		Expect(err).ToNot(HaveOccurred())

		expectReflectionOff(rebooted)
	})

	It("keeps the dependencies up when it stops", func() {
		dep := &fakeDep{}
		suite := boot(testkit.Options{Modules: modules, Dependencies: []testkit.Dependency{dep}})

		rebooted, err := suite.Rebooted(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(rebooted.Stop()).To(Succeed())

		Expect(dep.stopped).To(BeFalse())
		Expect(healthpb.NewHealthClient(suite.Conn()).Check(ctx, &healthpb.HealthCheckRequest{})).
			Error().ToNot(HaveOccurred())
	})

	It("holds what the dependencies of the suite provided in the injector", func() {
		suite := boot(testkit.Options{Modules: modules, Dependencies: []testkit.Dependency{&fakeDep{}}})

		rebooted, err := reboot(suite, nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(do.Invoke[*fakeResource](rebooted.Injector())).ToNot(BeNil())
	})
})

var _ = Describe("ResourceID", func() {
	It("returns an id under the prefix within the AIP-122 id pattern", func() {
		Expect(testkit.ResourceID("walked")).To(MatchRegexp(`^walked-[a-z0-9]{1,13}$`))
	})

	It("returns a different id on every call", func() {
		Expect(testkit.ResourceID("walked")).ToNot(Equal(testkit.ResourceID("walked")))
	})
})
