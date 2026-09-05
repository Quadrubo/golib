package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/samber/do/v2"
)

type Module interface {
	Name() string
}

type Provider interface {
	Module
	Provide(ctx context.Context, i do.Injector) error
}

type Runner interface {
	Module
	Run(ctx context.Context) error
}

type Stopper interface {
	Module
	Stop(ctx context.Context) error
}

// DrainBudget is how much time a Run has to return once its context is cancelled.
// A module that drains inside Run should read it before it provides, to check
// that its own drain time fits.
type DrainBudget time.Duration

func (b DrainBudget) String() string { return time.Duration(b).String() }

type App struct {
	injector do.Injector
	modules  []Module
	opts     options

	shutdownOnce sync.Once
	shutdownErr  error
}

func New(ctx context.Context, modules []Module, opts ...Option) (*App, error) {
	o := options{
		shutdownTimeout: DefaultShutdownTimeout,
		drainTimeout:    DefaultDrainTimeout,
		stopTimeout:     DefaultStopTimeout,
	}
	for _, opt := range opts {
		opt(&o)
	}

	// A deadline that has already passed would make every shutdown look like a timeout.
	if o.drainTimeout <= 0 {
		return nil, fmt.Errorf("app: the drain timeout must be positive, got %s", o.drainTimeout)
	}
	if o.stopTimeout <= 0 {
		return nil, fmt.Errorf("app: the stop timeout must be positive, got %s", o.stopTimeout)
	}

	if total := o.drainTimeout + o.stopTimeout; total > o.shutdownTimeout {
		return nil, fmt.Errorf(
			"app: the drain timeout of %s and the stop timeout of %s total %s, "+
				"which overruns the %s shutdown timeout",
			o.drainTimeout, o.stopTimeout, total, o.shutdownTimeout)
	}

	if err := checkDuplicateNames(modules); err != nil {
		return nil, err
	}

	a := &App{injector: do.New(), modules: modules, opts: o}
	do.ProvideValue(a.injector, DrainBudget(o.drainTimeout))

	for i, module := range a.modules {
		provider, ok := module.(Provider)
		if !ok {
			continue
		}

		if err := provider.Provide(ctx, a.injector); err != nil {
			provideErr := fmt.Errorf("app: failed to provide %q: %w", module.Name(), err)
			return nil, errors.Join(provideErr, a.unwind(i))
		}
	}

	return a, nil
}

func checkDuplicateNames(modules []Module) error {
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		name := module.Name()
		if _, taken := seen[name]; taken {
			return fmt.Errorf("app: two modules are named %q", name)
		}

		seen[name] = struct{}{}
	}

	return nil
}

func (a *App) Injector() do.Injector {
	return a.injector
}

func (a *App) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	active := &running{names: map[string]struct{}{}}

	for _, module := range a.modules {
		runnable, ok := module.(Runner)
		if !ok {
			continue
		}

		active.add(runnable.Name())
		active.wg.Add(1)

		go func() {
			// Brings the app down as soon as any module returns, error or not.
			defer cancelRun()
			defer active.wg.Done()
			defer active.remove(runnable.Name())

			if err := runnable.Run(runCtx); err != nil {
				active.fail(fmt.Errorf("app: failed to run %q: %w", runnable.Name(), err))
			}
		}()
	}

	<-runCtx.Done()

	runErr := drain(active, time.Now().Add(a.opts.drainTimeout))

	// Gives Stop its own budget so a hung module cannot eat the time the other
	// modules need to release what they hold.
	stopCtx, cancel := context.WithTimeout(context.Background(), a.opts.stopTimeout)
	defer cancel()

	return errors.Join(runErr, a.Shutdown(stopCtx))
}

func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		a.shutdownErr = teardown(ctx, a.modules)
	})

	return a.shutdownErr
}

// unwind releases the modules that provided successfully before module n failed.
func (a *App) unwind(n int) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.opts.stopTimeout)
	defer cancel()

	// Skips the modules that never provided, since they hold nothing to release.
	provided := make([]Module, 0, n)
	for _, module := range a.modules[:n] {
		if _, ok := module.(Provider); ok {
			provided = append(provided, module)
		}
	}

	return teardown(ctx, provided)
}

// teardown releases modules in reverse, so a module is always torn down before
// whatever it was built on top of.
func teardown(ctx context.Context, modules []Module) error {
	var errs []error
	for i := len(modules) - 1; i >= 0; i-- {
		module, ok := modules[i].(Stopper)
		if !ok {
			continue
		}

		if err := module.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("app: failed to stop %q: %w", module.Name(), err))
		}
	}

	return errors.Join(errs...)
}
