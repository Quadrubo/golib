package app

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrDrainTimeout means a module ignored its context and was abandoned.
var ErrDrainTimeout = errors.New("app: modules still running at the shutdown deadline")

// drain waits for the runnable modules to return, and names the ones that have
// not returned by the deadline alongside whatever the others failed with.
func drain(active *running, deadline time.Time) error {
	done := make(chan struct{})
	go func() {
		active.wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	select {
	case <-done:
		return active.err()
	case <-timer.C:
	}

	return errors.Join(
		active.err(),
		fmt.Errorf("%w: %s", ErrDrainTimeout, strings.Join(active.list(), ", ")))
}

// running tracks the runnables that have not returned and the failures of
// those that have.
type running struct {
	wg sync.WaitGroup

	mu    sync.Mutex
	names map[string]struct{}
	errs  []error
}

func (r *running) add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.names[name] = struct{}{}
}

func (r *running) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.names, name)
}

func (r *running) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errs = append(r.errs, err)
}

func (r *running) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return errors.Join(r.errs...)
}

func (r *running) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := slices.Collect(maps.Keys(r.names))
	slices.Sort(names)

	return names
}
