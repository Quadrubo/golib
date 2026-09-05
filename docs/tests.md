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

## Spec protos

This module serves no API, so the only protos it carries are the messages the
specs validate against, under `grpcinterceptor/validate/testdata`. `just
buf-gen` regenerates them. The generated code is committed, so a fresh clone
builds and tests without a codegen step.

There is no proto lint recipe, because one of those messages carries a CEL rule
that fails to compile on purpose.

A module path rename never rewrites a generated `.pb.go` in place. The
`go_package` option is embedded in the serialized descriptor, which is
length-prefixed, so a shorter path corrupts it and every message in the file
panics on init. Change the `.proto` and run `just buf-gen`.
