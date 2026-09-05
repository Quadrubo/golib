package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Suite")
}

var _ = Describe("Logging", func() {
	inject := func(settings map[string]any) (do.Injector, error) {
		// Provide replaces the process-wide default, so put it back afterwards.
		restore := slog.Default()
		DeferCleanup(func() { slog.SetDefault(restore) })

		a, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(settings),
				logging.Module(),
			})
		if err != nil {
			return nil, err
		}

		return a.Injector(), nil
	}

	logger := func(settings map[string]any) *slog.Logger {
		GinkgoHelper()

		i, err := inject(settings)
		Expect(err).ToNot(HaveOccurred())

		log, err := do.Invoke[*slog.Logger](i)
		Expect(err).ToNot(HaveOccurred())

		return log
	}

	// writing boots a logger onto a buffer, so a spec reads the records it wrote.
	writing := func(settings map[string]any) (*slog.Logger, *bytes.Buffer) {
		GinkgoHelper()

		out := &bytes.Buffer{}
		a, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(settings),
				logging.Module(logging.WithWriter(out), logging.WithoutDefault()),
			})
		Expect(err).ToNot(HaveOccurred())

		return do.MustInvoke[*slog.Logger](a.Injector()), out
	}

	It("provides a logger the rest of the app can resolve", func() {
		Expect(logger(map[string]any{})).ToNot(BeNil())
	})

	It("hands the configured level to the handler", func() {
		log := logger(map[string]any{"modules.logging.level": "debug"})

		Expect(log.Enabled(context.Background(), slog.LevelDebug)).To(BeTrue())
	})

	It("makes the provided logger the process default", func() {
		log := logger(map[string]any{"modules.logging.format": logging.FormatJSON})

		Expect(slog.Default()).To(BeIdenticalTo(log))
	})

	It("leaves the process default alone when told to", func() {
		restore := slog.Default()
		DeferCleanup(func() { slog.SetDefault(restore) })

		a, err := app.New(
			context.Background(), []app.Module{
				config.StaticModule(map[string]any{}),
				logging.Module(logging.WithoutDefault()),
			})
		Expect(err).ToNot(HaveOccurred())

		Expect(slog.Default()).To(BeIdenticalTo(restore))
		Expect(do.MustInvoke[*slog.Logger](a.Injector())).ToNot(BeIdenticalTo(restore))
	})

	It("writes text where it is told to", func() {
		log, out := writing(map[string]any{})

		log.Info("serving", slog.String("addr", ":50051"))

		Expect(out.String()).To(ContainSubstring("msg=serving"))
		Expect(out.String()).To(ContainSubstring("addr=:50051"))
	})

	It("writes json when asked for it", func() {
		log, out := writing(map[string]any{"modules.logging.format": logging.FormatJSON})

		log.Info("serving", slog.String("addr", ":50051"))

		Expect(out.String()).To(ContainSubstring(`"msg":"serving"`))
		Expect(out.String()).To(ContainSubstring(`"addr":":50051"`))
	})

	It("drops records below the level", func() {
		log, out := writing(map[string]any{"modules.logging.level": "warn"})

		log.Info("chatter")
		log.Warn("trouble")

		Expect(out.String()).ToNot(ContainSubstring("chatter"))
		Expect(out.String()).To(ContainSubstring("trouble"))
	})

	It("hands the configured format to the handler", func() {
		log := logger(map[string]any{"modules.logging.format": logging.FormatJSON})

		Expect(log.Handler()).To(BeAssignableToTypeOf(&slog.JSONHandler{}))
	})

	It("writes text when nothing sets the format", func() {
		log := logger(map[string]any{})

		Expect(log.Handler()).To(BeAssignableToTypeOf(&slog.TextHandler{}))
	})

	It("logs at info when nothing sets the level", func() {
		log := logger(map[string]any{})

		Expect(log.Enabled(context.Background(), slog.LevelDebug)).To(BeFalse())
		Expect(log.Enabled(context.Background(), slog.LevelInfo)).To(BeTrue())
	})

	It("refuses a format it cannot write", func() {
		_, err := inject(map[string]any{"modules.logging.format": "xml"})

		Expect(err).To(MatchError(ContainSubstring("modules.logging.format")))
		Expect(err).To(MatchError(ContainSubstring(`failed rule "oneof=text json"`)))
	})
})
