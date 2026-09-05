package logging

import "io"

type options struct {
	writer     io.Writer
	setDefault bool
}

type Option func(*options)

// WithWriter sends records somewhere other than os.Stdout.
func WithWriter(w io.Writer) Option {
	return func(o *options) { o.writer = w }
}

// WithoutDefault leaves the package-level slog default alone, so a process can
// boot a logger without every other one in it changing underneath.
func WithoutDefault() Option {
	return func(o *options) { o.setDefault = false }
}
