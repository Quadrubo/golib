# Tests

Specs use Ginkgo and live in the blackbox `package x_test`, so they exercise
the API that a caller sees. A package with a single spec file keeps `RunSpecs`
in that file rather than in a separate `_suite_test.go`.

Tests are written through the public API, and reach inside a package only where
testing through it would be materially worse.

Specs boot the real modules and their real dependencies rather than stand-ins.
A module that cannot be booted cleanly, because it writes to stdout or touches
process-wide state, carries an option that turns that behaviour off, the way
`logging` does with `WithWriter` and `WithoutDefault`.
