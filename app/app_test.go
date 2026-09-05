package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
)

func TestApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Suite")
}

type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) add(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type module struct {
	name       string
	rec        *recorder
	provideErr error
	runErr     error
	stopErr    error
}

func (m *module) Name() string { return m.name }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	m.rec.add("provide:" + m.name)
	if m.provideErr != nil {
		return m.provideErr
	}
	do.ProvideNamedValue(i, m.name, m.name)
	return nil
}

func (m *module) Run(ctx context.Context) error {
	m.rec.add("run:" + m.name)
	if m.runErr != nil {
		return m.runErr
	}
	<-ctx.Done()
	return nil
}

func (m *module) Stop(context.Context) error {
	m.rec.add("stop:" + m.name)
	return m.stopErr
}

// blocker blocks while it provides, the way a module waiting on a lock does.
type blocker struct {
	name    string
	rec     *recorder
	blocked chan struct{}
}

func (b *blocker) Name() string { return b.name }

func (b *blocker) Provide(ctx context.Context, _ do.Injector) error {
	b.rec.add("provide:" + b.name)
	close(b.blocked)
	<-ctx.Done()

	return ctx.Err()
}

type provideOnly struct {
	name string
	rec  *recorder
}

func (p *provideOnly) Name() string { return p.name }

func (p *provideOnly) Provide(context.Context, do.Injector) error {
	p.rec.add("provide:" + p.name)
	return nil
}

type stopOnly struct {
	name string
	rec  *recorder
}

func (s *stopOnly) Name() string { return s.name }

func (s *stopOnly) Stop(context.Context) error {
	s.rec.add("stop:" + s.name)
	return nil
}

// watcher records the deadline Stop was given.
type watcher struct {
	name string
	rec  *recorder
	err  error
	left time.Duration
}

func (w *watcher) Name() string { return w.name }

func (w *watcher) Provide(context.Context, do.Injector) error { return nil }

func (w *watcher) Stop(ctx context.Context) error {
	w.err = ctx.Err()
	if deadline, ok := ctx.Deadline(); ok {
		w.left = time.Until(deadline)
	}
	w.rec.add("stop:" + w.name)

	return nil
}

// stubborn ignores its context, which is what the shutdown budget exists to
// cut short.
type stubborn struct {
	name string
	rec  *recorder
}

func (s *stubborn) Name() string { return s.name }

func (s *stubborn) Run(context.Context) error {
	s.rec.add("run:" + s.name)
	// Long enough to outlast any budget a spec sets, short enough not to
	// outlive the suite.
	time.Sleep(5 * time.Second)

	return nil
}

type oneShot struct {
	name string
	rec  *recorder
}

func (o *oneShot) Name() string { return o.name }

func (o *oneShot) Run(context.Context) error {
	o.rec.add("run:" + o.name)
	return nil
}

var _ = Describe("App", func() {
	var rec *recorder

	BeforeEach(func() {
		rec = &recorder{}
	})

	runInBackground := func(a *app.App) (chan error, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()

		return done, cancel
	}

	booted := func(modules []app.Module, opts ...app.Option) *app.App {
		GinkgoHelper()

		a, err := app.New(context.Background(), modules, opts...)
		Expect(err).ToNot(HaveOccurred())

		return a
	}

	// stopped runs the app, cancels it, and returns what Run returned.
	stopped := func(a *app.App) error {
		GinkgoHelper()

		done, cancel := runInBackground(a)
		cancel()

		var err error
		Eventually(done).Should(Receive(&err))

		return err
	}

	Describe("startup", func() {
		It("provides modules in order and shares one injector", func() {
			a := booted([]app.Module{
				&module{name: "a", rec: rec},
				&module{name: "b", rec: rec},
			})

			Expect(rec.snapshot()).To(Equal([]string{"provide:a", "provide:b"}))
			Expect(do.InvokeNamed[string](a.Injector(), "a")).To(Equal("a"))
			Expect(do.InvokeNamed[string](a.Injector(), "b")).To(Equal("b"))
		})

		It("gives up providing when its context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			b := &blocker{name: "b", rec: rec, blocked: make(chan struct{})}

			done := make(chan error, 1)
			go func() {
				defer GinkgoRecover()
				_, err := app.New(ctx, []app.Module{&module{name: "a", rec: rec}, b})
				done <- err
			}()

			Eventually(b.blocked).Should(BeClosed())
			Consistently(done, 100*time.Millisecond).ShouldNot(Receive())
			cancel()

			var err error
			Eventually(done).Should(Receive(&err))
			Expect(err).To(MatchError(context.Canceled))
			// A boot that gave up still holds whatever it provided first.
			Expect(rec.snapshot()).To(ContainElement("stop:a"))
		})

		It("unwinds the modules it already provided when a later one fails", func() {
			boom := errors.New("boom")

			_, err := app.New(context.Background(), []app.Module{
				&module{name: "a", rec: rec},
				&module{name: "b", rec: rec, provideErr: boom},
				&module{name: "c", rec: rec},
			})

			Expect(err).To(MatchError(boom))
			Expect(err.Error()).To(ContainSubstring(`failed to provide "b"`))
			Expect(rec.snapshot()).To(Equal([]string{"provide:a", "provide:b", "stop:a"}))
		})

		It("reports a teardown that fails while unwinding", func() {
			provideBoom := errors.New("provide boom")
			shutdownBoom := errors.New("shutdown boom")

			_, err := app.New(context.Background(), []app.Module{
				&module{name: "a", rec: rec, stopErr: shutdownBoom},
				&module{name: "b", rec: rec, provideErr: provideBoom},
			})

			// Losing the teardown error would hide a leaked resource behind the
			// failure that triggered the unwind.
			Expect(err).To(MatchError(provideBoom))
			Expect(err).To(MatchError(shutdownBoom))
			Expect(err.Error()).To(ContainSubstring(`failed to stop "a"`))
		})

		It("unwinds on the stop budget, since unwinding is releasing", func() {
			w := &watcher{name: "w", rec: rec}

			_, err := app.New(
				context.Background(),
				[]app.Module{w, &module{name: "b", rec: rec, provideErr: errors.New("boom")}},
				app.WithShutdownTimeout(20*time.Second),
				app.WithDrainTimeout(9*time.Second),
				app.WithStopTimeout(2*time.Second),
			)
			Expect(err).To(HaveOccurred())

			Expect(w.left).To(BeNumerically("<", 3*time.Second))
			Expect(w.left).To(BeNumerically(">", time.Second))
		})

		It("leaves modules that never provided out of the unwind", func() {
			_, err := app.New(context.Background(), []app.Module{
				&module{name: "a", rec: rec},
				&stopOnly{name: "s", rec: rec},
				&module{name: "b", rec: rec, provideErr: errors.New("boom")},
			})

			Expect(err).To(HaveOccurred())
			Expect(rec.snapshot()).ToNot(ContainElement("stop:s"))
			Expect(rec.snapshot()).To(ContainElement("stop:a"))
		})

		It("publishes its drain budget, so a module can size its own drain", func() {
			a := booted(
				[]app.Module{&module{name: "a", rec: rec}},
				app.WithDrainTimeout(3*time.Second),
			)

			budget := do.MustInvoke[app.DrainBudget](a.Injector())

			Expect(time.Duration(budget)).To(Equal(3 * time.Second))
			// Errors format the budget, so it has to read as a duration and not a count.
			Expect(budget.String()).To(Equal("3s"))
		})

		It("refuses two modules under one name", func() {
			_, err := app.New(context.Background(), []app.Module{
				&module{name: "a", rec: rec},
				&module{name: "a", rec: rec},
			})

			Expect(err).To(MatchError(ContainSubstring(`two modules are named "a"`)))
			Expect(rec.snapshot()).To(BeEmpty())
		})

		DescribeTable("refuses budgets that cannot work",
			func(want string, opts ...app.Option) {
				_, err := app.New(context.Background(), nil, opts...)

				Expect(err).To(MatchError(ContainSubstring(want)))
			},
			Entry("a drain timeout with no room in it",
				"drain timeout must be positive",
				app.WithDrainTimeout(0)),
			Entry("a stop timeout with no room in it",
				"stop timeout must be positive",
				app.WithStopTimeout(0)),
			Entry("budgets that together overrun the shutdown timeout",
				"total 11s, which overruns the 10s shutdown timeout",
				app.WithShutdownTimeout(10*time.Second),
				app.WithDrainTimeout(9*time.Second),
				app.WithStopTimeout(2*time.Second)),
		)

		It("accepts budgets that exactly fill the shutdown timeout", func() {
			_, err := app.New(
				context.Background(), nil,
				app.WithShutdownTimeout(10*time.Second),
				app.WithDrainTimeout(9*time.Second),
				app.WithStopTimeout(time.Second),
			)

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("running", func() {
		It("skips modules that do not implement the optional interfaces", func() {
			a := booted([]app.Module{&provideOnly{name: "a", rec: rec}})

			done, cancel := runInBackground(a)

			Consistently(done, 50*time.Millisecond).ShouldNot(Receive())
			cancel()

			Eventually(done).Should(Receive(BeNil()))
			Expect(rec.snapshot()).To(Equal([]string{"provide:a"}))
		})

		It("runs every runnable and shuts down in reverse on cancellation", func() {
			a := booted([]app.Module{
				&module{name: "a", rec: rec},
				&module{name: "b", rec: rec},
			})

			done, cancel := runInBackground(a)

			Eventually(rec.snapshot).Should(ContainElements("run:a", "run:b"))
			cancel()

			Eventually(done).Should(Receive(BeNil()))

			calls := rec.snapshot()
			Expect(calls[len(calls)-2:]).To(Equal([]string{"stop:b", "stop:a"}))
		})

		It("stops the app when a runnable returns on its own", func() {
			a := booted([]app.Module{
				&oneShot{name: "once", rec: rec},
				&module{name: "blocker", rec: rec},
			})

			done := make(chan error, 1)
			go func() { done <- a.Run(context.Background()) }()

			Eventually(done).Should(Receive(BeNil()))
			Expect(rec.snapshot()).To(ContainElement("stop:blocker"))
		})

		It("abandons the modules that ignore their context, and names them in order", func() {
			// Deliberately unsorted, and enough of them that map order is unlikely
			// to come out sorted by accident.
			a := booted(
				[]app.Module{
					&module{name: "polite", rec: rec},
					&stubborn{name: "echo", rec: rec},
					&stubborn{name: "alpha", rec: rec},
					&stubborn{name: "delta", rec: rec},
					&stubborn{name: "bravo", rec: rec},
					&stubborn{name: "charlie", rec: rec},
				},
				app.WithDrainTimeout(50*time.Millisecond),
			)

			done, cancel := runInBackground(a)
			began := time.Now()
			cancel()

			var runErr error
			Eventually(done).Should(Receive(&runErr))

			Expect(runErr).To(MatchError(app.ErrDrainTimeout))
			Expect(runErr.Error()).To(HaveSuffix("alpha, bravo, charlie, delta, echo"))
			Expect(runErr.Error()).ToNot(ContainSubstring("polite"))

			// Without WithDrainTimeout this waits out the 7s default instead.
			Expect(time.Since(began)).To(BeNumerically("<", time.Second))
		})

		It("shuts down even when draining timed out", func() {
			a := booted(
				[]app.Module{&module{name: "a", rec: rec}, &stubborn{name: "stuck", rec: rec}},
				app.WithDrainTimeout(50*time.Millisecond),
			)

			Expect(stopped(a)).To(MatchError(app.ErrDrainTimeout))
			Expect(rec.snapshot()).To(ContainElement("stop:a"))
		})

		It("hands Stop its own budget", func() {
			w := &watcher{name: "w", rec: rec}

			a := booted(
				[]app.Module{w, &module{name: "a", rec: rec}},
				app.WithDrainTimeout(time.Second),
				app.WithStopTimeout(2*time.Second),
			)

			Expect(stopped(a)).To(Succeed())

			Expect(w.err).ToNot(HaveOccurred())
			Expect(w.left).To(BeNumerically(">", time.Second))
		})

		It("gives Stop its full budget even after draining overran", func() {
			w := &watcher{name: "w", rec: rec}

			a := booted(
				[]app.Module{w, &stubborn{name: "stuck", rec: rec}},
				app.WithDrainTimeout(50*time.Millisecond),
				app.WithStopTimeout(2*time.Second),
			)

			Expect(stopped(a)).To(MatchError(app.ErrDrainTimeout))

			// The module that hung is abandoned either way, so releasing what the
			// others hold does not get charged for its overrun.
			Expect(w.err).ToNot(HaveOccurred())
			Expect(w.left).To(BeNumerically(">", time.Second))
			Expect(rec.snapshot()).To(ContainElement("stop:w"))
		})

		It("returns a run failure and still shuts down", func() {
			boom := errors.New("boom")

			a := booted([]app.Module{
				&module{name: "a", rec: rec},
				&module{name: "b", rec: rec, runErr: boom},
			})

			runErr := a.Run(context.Background())

			Expect(runErr).To(MatchError(boom))
			Expect(runErr.Error()).To(ContainSubstring(`failed to run "b"`))
			Expect(rec.snapshot()).To(ContainElements("stop:a", "stop:b"))
		})

		It("reports every module that failed to run", func() {
			first := errors.New("first boom")
			second := errors.New("second boom")

			a := booted([]app.Module{
				&module{name: "a", rec: rec, runErr: first},
				&module{name: "b", rec: rec, runErr: second},
			})

			runErr := a.Run(context.Background())

			Expect(runErr).To(MatchError(first))
			Expect(runErr).To(MatchError(second))
		})

		It("reports a module that failed before the drain deadline", func() {
			boom := errors.New("boom")

			a := booted(
				[]app.Module{
					&module{name: "a", rec: rec, runErr: boom},
					&stubborn{name: "stuck", rec: rec},
				},
				app.WithDrainTimeout(50*time.Millisecond),
			)

			runErr := a.Run(context.Background())

			Expect(runErr).To(MatchError(boom))
			Expect(runErr).To(MatchError(app.ErrDrainTimeout))
		})
	})

	Describe("shutdown", func() {
		It("reports a module that fails to shut down", func() {
			boom := errors.New("boom")

			a := booted([]app.Module{&module{name: "a", rec: rec, stopErr: boom}})

			stopErr := a.Shutdown(context.Background())

			Expect(stopErr).To(MatchError(boom))
			Expect(stopErr.Error()).To(ContainSubstring(`failed to stop "a"`))
		})

		It("runs once however often it is called", func() {
			a := booted([]app.Module{&module{name: "a", rec: rec}})

			Expect(stopped(a)).To(Succeed())

			// A caller that defensively shuts down after Run must not tear down twice.
			Expect(a.Shutdown(context.Background())).To(Succeed())

			Expect(rec.snapshot()).To(Equal([]string{"provide:a", "run:a", "stop:a"}))
		})
	})
})
