package authentication_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	. "github.com/quadrubo/golib/testkit/matchers"

	"github.com/quadrubo/golib/grpcerr"
	"github.com/quadrubo/golib/grpcinterceptor/authentication"
)

func TestAuthentication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authentication Suite")
}

type callerKey struct{}

type authenticator struct {
	seen []string
}

func (a *authenticator) Authenticate(ctx context.Context, credential string) (context.Context, error) {
	a.seen = append(a.seen, credential)
	if credential == "broken" {
		return nil, nil
	}

	if credential != "valid" {
		return nil, grpcerr.Unauthenticated("SESSION_INVALID", "the session is unknown or has ended")
	}

	return context.WithValue(ctx, callerKey{}, "alice"), nil
}

func withAuthorization(values ...string) context.Context {
	md := metadata.MD{}
	for _, value := range values {
		md.Append("authorization", value)
	}

	return metadata.NewIncomingContext(context.Background(), md)
}

type serverStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s serverStream) Context() context.Context { return s.ctx }

var malformed = []TableEntry{
	Entry("another scheme", "Basic dmFsaWQ="),
	Entry("a scheme without a credential", "Bearer "),
	Entry("a credential without a scheme", "valid"),
	Entry("two values", "Bearer valid", "Bearer valid"),
	Entry("a scheme with only spaces", "Bearer   "),
	Entry("whitespace after the credential", "Bearer valid "),
}

var _ = Describe("Interceptor", func() {
	var (
		stub   *authenticator
		unary  grpc.UnaryServerInterceptor
		stream grpc.StreamServerInterceptor
	)

	BeforeEach(func() {
		stub = &authenticator{}

		i := do.New()
		do.ProvideValue[authentication.Authenticator](i, stub)
		do.ProvideValue(i, authentication.Anonymous{
			Services: []string{"grpc.health.v1.Health"},
			Methods:  []string{"/spec.v1.Authentication/SignIn"},
		})

		var err error
		unary, err = authentication.Unary(i)
		Expect(err).ToNot(HaveOccurred())
		stream, err = authentication.Stream(i)
		Expect(err).ToNot(HaveOccurred())
	})

	callUnary := func(ctx context.Context, method string) (any, error) {
		return unary(ctx, "req", &grpc.UnaryServerInfo{FullMethod: method},
			func(ctx context.Context, _ any) (any, error) { return ctx.Value(callerKey{}), nil })
	}

	callStream := func(ctx context.Context, method string) (any, error) {
		var caller any
		err := stream(nil, serverStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method},
			func(_ any, ss grpc.ServerStream) error {
				caller = ss.Context().Value(callerKey{})

				return nil
			})

		return caller, err
	}

	Context("on a unary call", func() {
		It("runs the handler under the context the authenticator returns", func() {
			caller, err := callUnary(withAuthorization("Bearer valid"), "/spec.v1.Books/GetBook")

			Expect(err).ToNot(HaveOccurred())
			Expect(caller).To(Equal("alice"))
		})

		It("reads the scheme without regard to case", func() {
			caller, err := callUnary(withAuthorization("bearer valid"), "/spec.v1.Books/GetBook")

			Expect(err).ToNot(HaveOccurred())
			Expect(caller).To(Equal("alice"))
		})

		It("reads a credential after several spaces", func() {
			caller, err := callUnary(withAuthorization("Bearer  valid"), "/spec.v1.Books/GetBook")

			Expect(err).ToNot(HaveOccurred())
			Expect(caller).To(Equal("alice"))
		})

		It("returns the error of the authenticator", func() {
			_, err := callUnary(withAuthorization("Bearer stale"), "/spec.v1.Books/GetBook")

			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
			Expect(err).To(HaveReason("SESSION_INVALID"))
		})

		It("fails a call when the authenticator returns neither a context nor an error", func() {
			_, err := callUnary(withAuthorization("Bearer broken"), "/spec.v1.Books/GetBook")

			Expect(err).To(MatchError(ContainSubstring("authentication: the authenticator returned no context")))
		})

		It("rejects a call without a credential before the authenticator", func() {
			_, err := callUnary(context.Background(), "/spec.v1.Books/GetBook")

			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
			Expect(err).To(HaveReason("CREDENTIAL_MISSING"))
			Expect(stub.seen).To(BeEmpty())
		})

		DescribeTable("rejects authorization metadata that is no single bearer credential",
			func(values ...string) {
				_, err := callUnary(withAuthorization(values...), "/spec.v1.Books/GetBook")

				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err).To(HaveReason("CREDENTIAL_MALFORMED"))
				Expect(stub.seen).To(BeEmpty())
			},
			malformed,
		)

		It("runs an anonymous method without a credential", func() {
			_, err := callUnary(context.Background(), "/spec.v1.Authentication/SignIn")

			Expect(err).ToNot(HaveOccurred())
		})

		It("runs every method of an anonymous service without a credential", func() {
			_, err := callUnary(context.Background(), "/grpc.health.v1.Health/Check")

			Expect(err).ToNot(HaveOccurred())
		})

		It("skips the authenticator on an anonymous method that carries a credential", func() {
			_, err := callUnary(withAuthorization("Bearer stale"), "/spec.v1.Authentication/SignIn")

			Expect(err).ToNot(HaveOccurred())
			Expect(stub.seen).To(BeEmpty())
		})

		It("keeps a method that only shares a prefix with an anonymous one closed", func() {
			_, err := callUnary(context.Background(), "/spec.v1.Authentication/SignInAgain")

			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})

		It("reads the service up to the last slash of the method, as grpc routes it", func() {
			_, err := callUnary(context.Background(), "/grpc.health.v1.Health/spec.v1.Books/GetBook")

			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})

	Context("on a stream call", func() {
		It("runs the handler under the context the authenticator returns", func() {
			caller, err := callStream(withAuthorization("Bearer valid"), "/spec.v1.Books/WatchBooks")

			Expect(err).ToNot(HaveOccurred())
			Expect(caller).To(Equal("alice"))
		})

		It("rejects a call without a credential", func() {
			_, err := callStream(context.Background(), "/spec.v1.Books/WatchBooks")

			Expect(err).To(HaveReason("CREDENTIAL_MISSING"))
		})

		It("returns the error of the authenticator", func() {
			_, err := callStream(withAuthorization("Bearer stale"), "/spec.v1.Books/WatchBooks")

			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
			Expect(err).To(HaveReason("SESSION_INVALID"))
		})

		It("fails a call when the authenticator returns neither a context nor an error", func() {
			_, err := callStream(withAuthorization("Bearer broken"), "/spec.v1.Books/WatchBooks")

			Expect(err).To(MatchError(ContainSubstring("authentication: the authenticator returned no context")))
		})

		DescribeTable("rejects authorization metadata that is no single bearer credential",
			func(values ...string) {
				_, err := callStream(withAuthorization(values...), "/spec.v1.Books/WatchBooks")

				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err).To(HaveReason("CREDENTIAL_MALFORMED"))
				Expect(stub.seen).To(BeEmpty())
			},
			malformed,
		)

		It("runs a method of an anonymous service without a credential", func() {
			_, err := callStream(context.Background(), "/grpc.health.v1.Health/Watch")

			Expect(err).ToNot(HaveOccurred())
			Expect(stub.seen).To(BeEmpty())
		})
	})
})

var _ = Describe("building the interceptors", func() {
	build := map[string]func(do.Injector) error{
		"Unary": func(i do.Injector) error {
			_, err := authentication.Unary(i)

			return err
		},
		"Stream": func(i do.Injector) error {
			_, err := authentication.Stream(i)

			return err
		},
	}

	for name, interceptor := range build {
		It("reports a chain that left out the authenticator for "+name, func() {
			i := do.New()
			do.ProvideValue(i, authentication.Anonymous{})

			Expect(interceptor(i)).To(MatchError(ContainSubstring("failed to invoke the authenticator")))
		})

		It("reports a chain that left out the anonymous calls for "+name, func() {
			i := do.New()
			do.ProvideValue[authentication.Authenticator](i, &authenticator{})

			Expect(interceptor(i)).To(MatchError(ContainSubstring("failed to invoke the anonymous calls")))
		})
	}
})
