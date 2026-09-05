package grpcserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/grpcserver"
	"github.com/quadrubo/golib/logging"
)

func TestGRPCServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GRPC Server Suite")
}

const blockerMethod = "/grpcserver.test.Blocker/Block"

// anyPort leaves the port to the operating system, which addrOf reads back.
const anyPort = "127.0.0.1:0"

// blocker holds a call open until its context is cancelled, which is what a
// hard stop has to cut through.
type blocker struct {
	entered chan struct{}
}

func (b *blocker) desc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "grpcserver.test.Blocker",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Block",
			Handler: func(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				if err := dec(new(emptypb.Empty)); err != nil {
					return nil, err
				}
				close(b.entered)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}},
	}
}

var _ = Describe("Server", func() {
	// addrOf reads back the address the server bound, which port 0 leaves to
	// the operating system.
	addrOf := func(a *app.App) string {
		GinkgoHelper()
		return string(do.MustInvoke[grpcserver.Addr](a.Injector()))
	}

	start := func(settings map[string]any) *app.App {
		GinkgoHelper()
		a, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(settings),
				logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
				grpcserver.Module(),
			})
		Expect(err).ToNot(HaveOccurred())
		return a
	}

	// The server takes registrations until Run hands it to Serve.
	register := func(a *app.App, desc *grpc.ServiceDesc) {
		GinkgoHelper()
		do.MustInvoke[*grpc.Server](a.Injector()).RegisterService(desc, struct{}{})
	}

	runInBackground := func(a *app.App) (chan error, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()

		return done, cancel
	}

	dial := func(addr string) *grpc.ClientConn {
		GinkgoHelper()
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(conn.Close)
		return conn
	}

	// checkOn reuses a connection, so it still answers once GracefulStop has
	// stopped accepting new ones.
	checkOn := func(conn *grpc.ClientConn) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			return err
		}

		if resp.Status != healthpb.HealthCheckResponse_SERVING {
			return fmt.Errorf("status is %s", resp.Status)
		}

		return nil
	}

	serving := func(addr string) error {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		defer conn.Close()

		return checkOn(conn)
	}

	listsServices := func(addr string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		stream, err := reflectionpb.NewServerReflectionClient(dial(addr)).ServerReflectionInfo(ctx)
		if err != nil {
			return err
		}

		req := &reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
		}
		if err := stream.Send(req); err != nil {
			return err
		}

		_, err = stream.Recv()
		return err
	}

	It("serves until the context is cancelled", func() {
		a := start(map[string]any{"modules.grpcserver.addr": anyPort})
		addr := addrOf(a)

		done, cancel := runInBackground(a)

		Eventually(func() error { return serving(addr) }).Should(Succeed())
		cancel()

		Eventually(done).Should(Receive(BeNil()))
		Expect(serving(addr)).ToNot(Succeed())
	})

	It("fails the boot on an address it cannot bind", func() {
		_, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(map[string]any{"modules.grpcserver.addr": "not-an-address"}),
				logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
				grpcserver.Module(),
			})

		Expect(err).To(MatchError(ContainSubstring("failed to listen on not-an-address")))
	})

	// Equal is the case the rule exists for: Run returns as the deadline lands.
	// The budget is the drain slice, not the whole shutdown timeout.
	It("refuses a grace period that uses up the whole drain budget", func() {
		_, err := app.New(
			context.Background(),
			[]app.Module{
				config.StaticModule(map[string]any{"modules.grpcserver.grace_period": "8s"}),
				logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
				grpcserver.Module(),
			},
			app.WithShutdownTimeout(10*time.Second),
			app.WithDrainTimeout(8*time.Second),
		)

		Expect(err).To(MatchError(ContainSubstring("grace_period of 8s has to be less than the 8s drain budget")))
	})

	It("reports a module list that left out the logger", func() {
		_, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(map[string]any{}),
				grpcserver.Module(),
			})

		Expect(err).To(MatchError(ContainSubstring("grpcserver: failed to invoke the logger")))
	})

	// Asserted on the logger itself rather than through grpclog.SetLoggerV2,
	// which the module installs once per process.
	It("routes grpc's own logging into the logger it is given", func() {
		out := &bytes.Buffer{}

		grpcserver.GRPCLogger(slog.New(slog.NewTextHandler(out, nil))).
			Errorf("connection %s went away", "abc")

		Expect(out.String()).To(ContainSubstring("connection abc went away"))
		Expect(out.String()).To(ContainSubstring("component=grpc"))
		Expect(out.String()).To(ContainSubstring("level=ERROR"))
	})

	It("leaves health off when it is turned off", func() {
		a := start(map[string]any{
			"modules.grpcserver.addr":   anyPort,
			"modules.grpcserver.health": false,
		})
		addr := addrOf(a)

		_, cancel := runInBackground(a)
		DeferCleanup(cancel)

		Eventually(func() error { return listsServices(addr) }).Should(Succeed())
		Expect(serving(addr)).To(MatchError(ContainSubstring("unknown service")))
	})

	It("provides the health server so modules can report their own status", func() {
		a := start(map[string]any{"modules.grpcserver.addr": anyPort})

		Expect(do.Invoke[*health.Server](a.Injector())).ToNot(BeNil())
	})

	It("reports itself unhealthy while it drains", func() {
		held := &blocker{entered: make(chan struct{})}

		a := start(map[string]any{
			"modules.grpcserver.addr":         anyPort,
			"modules.grpcserver.grace_period": "2s",
		})
		addr := addrOf(a)
		register(a, held.desc())

		done, cancel := runInBackground(a)

		conn := dial(addr)
		go func() {
			_ = conn.Invoke(context.Background(), blockerMethod, &emptypb.Empty{}, &emptypb.Empty{})
		}()

		Eventually(held.entered).Should(BeClosed())

		// GracefulStop sends GOAWAY, so a call started after it opens a fresh
		// connection and is refused. A stream opened before it still delivers.
		watchCtx, stopWatch := context.WithCancel(context.Background())
		DeferCleanup(stopWatch)

		watch, err := healthpb.NewHealthClient(conn).Watch(watchCtx, &healthpb.HealthCheckRequest{})
		Expect(err).ToNot(HaveOccurred())

		serving, err := watch.Recv()
		Expect(err).ToNot(HaveOccurred())
		Expect(serving.GetStatus()).To(Equal(healthpb.HealthCheckResponse_SERVING))

		cancel()

		draining, err := watch.Recv()
		Expect(err).ToNot(HaveOccurred())
		Expect(draining.GetStatus()).To(Equal(healthpb.HealthCheckResponse_NOT_SERVING))

		Eventually(done, 3*time.Second).Should(Receive(BeNil()))
	})

	It("answers reflection by default", func() {
		a := start(map[string]any{"modules.grpcserver.addr": anyPort})
		addr := addrOf(a)

		_, cancel := runInBackground(a)
		DeferCleanup(cancel)

		Eventually(func() error { return listsServices(addr) }).Should(Succeed())
	})

	It("leaves reflection off when it is turned off", func() {
		a := start(map[string]any{
			"modules.grpcserver.addr":       anyPort,
			"modules.grpcserver.reflection": false,
		})
		addr := addrOf(a)

		_, cancel := runInBackground(a)
		DeferCleanup(cancel)

		Eventually(func() error { return serving(addr) }).Should(Succeed())
		Expect(listsServices(addr)).To(MatchError(ContainSubstring("unknown service")))
	})

	It("drops a call that outlasts the grace period", func() {
		held := &blocker{entered: make(chan struct{})}

		a := start(map[string]any{
			"modules.grpcserver.addr":         anyPort,
			"modules.grpcserver.grace_period": "100ms",
		})
		addr := addrOf(a)
		register(a, held.desc())

		done, cancel := runInBackground(a)

		called := make(chan error, 1)
		conn := dial(addr)
		go func() {
			called <- conn.Invoke(context.Background(), blockerMethod, &emptypb.Empty{}, &emptypb.Empty{})
		}()

		Eventually(held.entered).Should(BeClosed())
		cancel()

		// Without the grace period this waits out the app's drain deadline.
		Eventually(done, 2*time.Second).Should(Receive(BeNil()))
		Eventually(called).Should(Receive(HaveOccurred()))
	})

	It("answers a message beyond max_receive_bytes with ResourceExhausted", func() {
		a := start(map[string]any{"modules.grpcserver.addr": anyPort})
		addr := addrOf(a)

		_, cancel := runInBackground(a)
		DeferCleanup(cancel)
		Eventually(func() error { return serving(addr) }).Should(Succeed())

		_, err := healthpb.NewHealthClient(dial(addr)).Check(context.Background(),
			&healthpb.HealthCheckRequest{Service: strings.Repeat("x", 5*1024*1024)})

		Expect(status.Code(err)).To(Equal(codes.ResourceExhausted))
	})

	It("takes a message under a raised max_receive_bytes", func() {
		a := start(map[string]any{
			"modules.grpcserver.addr":              anyPort,
			"modules.grpcserver.max_receive_bytes": "16MiB",
		})
		addr := addrOf(a)

		_, cancel := runInBackground(a)
		DeferCleanup(cancel)
		Eventually(func() error { return serving(addr) }).Should(Succeed())

		_, err := healthpb.NewHealthClient(dial(addr)).Check(context.Background(),
			&healthpb.HealthCheckRequest{Service: strings.Repeat("x", 5*1024*1024)})

		// The health server answers NotFound for a service name it does not
		// know, so the message passed the cap.
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})
})
