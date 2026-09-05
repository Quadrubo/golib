package app

import "time"

const (
	// DefaultShutdownTimeout stays under the supervisor's grace period before
	// the kill, which for `docker stop` defaults to 10s.
	DefaultShutdownTimeout = 8 * time.Second

	DefaultDrainTimeout = 7 * time.Second
	DefaultStopTimeout  = 1 * time.Second
)

type options struct {
	shutdownTimeout time.Duration
	drainTimeout    time.Duration
	stopTimeout     time.Duration
}

type Option func(*options)

// WithShutdownTimeout sets the ceiling WithDrainTimeout and WithStopTimeout have
// to fit inside, and comes from the supervisor's grace period rather than
// anything this process decides.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
}

// WithDrainTimeout bounds how long the runnable modules have to return once
// their context is cancelled, after which they are abandoned. WithDrainTimeout
// and WithStopTimeout together have to fit inside WithShutdownTimeout.
func WithDrainTimeout(d time.Duration) Option {
	return func(o *options) { o.drainTimeout = d }
}

// WithStopTimeout bounds releasing what the modules hold, on its own budget
// rather than what draining left over. WithDrainTimeout and WithStopTimeout
// together have to fit inside WithShutdownTimeout.
func WithStopTimeout(d time.Duration) Option {
	return func(o *options) { o.stopTimeout = d }
}
