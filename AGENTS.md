# golib

Generic Go service infrastructure, shared across projects. The
[README](README.md) covers the setup and the commands.

## Boundaries

A package here must not know what it is being used to build. It carries no
project names, no domain concepts, and no defaults that only make sense for one
consumer. Anything specific belongs to the consuming service or its config.

## Working here

The `justfile` carries every command. Use those instead of writing your own.

Commits are conventional. Every commit must pass `just check`.

## Code

- A package name is singular. Plural is for a consumer's collections, not for
  infrastructure.
- Receivers take the conventional short name derived from their type, one or
  two letters, the same one on every method of that type.
- Errors are wrapped with the component that failed and what it was doing, as
  in `fmt.Errorf("app: failed to provide %q: %w", ...)`.

## Comments

- Never write a comment above a package.
- Never restate the code. A comment carries the reason a line exists, or the
  constraint that makes it look wrong, not what the line plainly does.
- Keep a comment to one sentence. A doc comment on an exported identifier
  runs as long as the contract a consumer reads needs it to.
- A doc comment opens with the name it documents, optionally after an article.
- Name what you are referring to instead of pointing back at it.
- Keep it mechanical and technical.
- Never mention the history of the code unless it guards a regression.

Spec names follow the same rules.

## Tests

A test that still passes when you break the code it covers is not a test. Break
the implementation once and confirm the spec fails.

[docs/tests.md](docs/tests.md) carries the conventions.

## Documentation

Keep `docs/` up to date with the code, in the same commit.

`docs/packages/` mirrors the import path, so `grpcinterceptor/recovery` is
documented in `docs/packages/grpcinterceptor/recovery.md`. Each page documents
one package under a fixed structure:

- `## Usage` is the shape a consumer works with
- `## Mechanics` is the mechanism, for anyone changing the package
- `## Decisions` is what was chosen, what was rejected, and why
- `## Failure modes` is what goes wrong silently, and appears only where there
  is something

Anything that does not describe a single package stays at the root of `docs/`.

Write prose that describes what the module does rather than instructing the
reader. Leave out anything a config file already states, and anything that only
argues for a rule stated beside it. No em dashes and no semicolons. Code goes
in a fenced block with a language tag.
