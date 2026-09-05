# config

The `config` package layers a service's settings from files and the
environment, and hands each module the subtree it reads.

## Usage

`config.Module` provides a `*config.Config` and goes first in the module list,
since every module that reads settings resolves it.

A module declares a struct tagged with `config` keys and optional `validate`
rules, then calls `config.Load` with its own name, its key, and the struct
holding its defaults.

```go
cfg, err := config.Load(i, m.Name(), "modules.server", Config{
    Addr: ":50051",
})
```

Fields that no source sets keep the value they came in with, so a module
carries its own defaults instead of a file having to list every setting.

`config.StaticModule` builds the same `Config` from a map of dotted keys, which
is how a spec boots a service with settings and no file on disk.

## Custom types

A type implementing `encoding.TextUnmarshaler` decodes from a config value, so
a module takes a richer setting than a string or a number. `time.Duration` is
handled the same way, which is what lets a setting be written `5s`.

### Bytes

`config.Bytes` is a byte count that takes a plain number or a binary unit, so a
module writes `max_receive_bytes: 128MiB` rather than `134217728`. The units
are `B`, `KiB`, `MiB` and `GiB`. The decimal `MB` family is rejected, since
half the tools that use it mean binary anyway.

## Mechanics

Three sources layer in order, each overriding the one before it. The base file
is `config.yaml`, then `config.local.yaml` beside it, then the environment. The
local file is gitignored, so a developer overrides a setting without touching a
tracked file.

The base path comes from `Options.File`, or from `<EnvPrefix>CONFIG_FILE` which
overrides it, or defaults to `config.yaml`. The local overlay follows whichever
won, so `/etc/app/config.yaml` picks up `/etc/app/config.local.yaml`.
`CONFIG_FILE` is read before any config exists and never becomes a key itself.

A setting is addressed in the environment as `<PREFIX>MODULES__SERVER__ADDR`.
The double underscore separates segments and a single underscore stays part of
a key, which lets `read_timeout` be written `READ_TIMEOUT`.

`Unmarshal` decodes a subtree and then runs the `validate` tags, reporting a
failure by config key rather than by Go field name.

## Decisions

The environment prefix has to end in `__`, and the module refuses to boot
otherwise. Kubernetes injects `APP_SERVICE_HOST`, `APP_PORT_9001_TCP_ADDR` and
the rest of the link-style variables into every pod in the namespace of a
service named `app`, and under an `APP_` prefix every one of them becomes a
config key. None carries a double underscore, so requiring one keeps them out.

`config.yaml` may be missing, which lets a container run on the environment
alone. Naming a path in `Options.File` or `CONFIG_FILE` claims the file exists,
so a missing one fails the boot instead of falling back to defaults.

`CONFIG_FILE` wins over `Options.File` because the environment gets the last
word, and whoever has to point a built binary at another file cannot recompile
it.

Unknown keys fail the boot. Without that, `adrr` would quietly keep `:50051`.

## Failure modes

Rejecting unknown keys applies to whatever subtree is read, so a struct has to
cover its whole subtree. Unmarshalling the root fails on every key belonging to
another module.

Decoding is weakly typed, so `"5"` reaches an int field as `5`. A value meant
to stay a string that happens to look like a number is converted rather than
refused.
