# golib

Generic Go service infrastructure, shared across projects. Nothing here knows
what it is being used to build.

## Installation

```shell
go get github.com/quadrubo/golib@v0.1.0
```

## Documentation

[docs/](docs/) explains how each part of the module works and why. It records
what this module guarantees. A consumer's own API contract is not documented
here.

## Contributing

The repository uses a Nix flake together with direnv. `direnv allow` sets it up
once, after which every tool is on the PATH.

The `justfile` carries the commands. The most important ones are:

- `just install-hooks` enables the git hook
- `just check` runs everything a commit has to pass
- `just fmt` formats the code

Commits are conventional. Every commit passes `just check` on its own.

## License

MIT. See [LICENSE](LICENSE).
