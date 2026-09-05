package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
)

const (
	delim = "."
	tag   = "config"

	// envDelim is a double underscore, so a single underscore stays part of a
	// key, as in read_timeout.
	envDelim = "__"

	defaultFile = "config.yaml"
	localSuffix = ".local"

	// fileVar is read before any config exists so it cannot itself be a config key.
	fileVar = "CONFIG_FILE"
)

type Options struct {
	// EnvPrefix has to end in __, which keeps unrelated environment variables
	// such as SERVICE_HOST out of the settings.
	EnvPrefix string

	// File defaults to config.yaml and is overridden by <EnvPrefix>CONFIG_FILE.
	// A path either of them names has to exist, where config.yaml may be
	// missing.
	File string
}

type Config struct {
	k        *koanf.Koanf
	validate *validator.Validate
}

// Module provides a Config layered from config.yaml, config.local.yaml and the
// environment.
func Module(opts Options) app.Module {
	return &module{opts: opts}
}

type module struct {
	opts Options
}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "config" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	k, err := newKoanf(m.opts)
	if err != nil {
		return err
	}

	do.ProvideValue(i, &Config{k: k, validate: newValidator()})
	return nil
}

// StaticModule provides a Config from dotted keys instead of files on disk,
// which is how a spec boots a service with settings.
func StaticModule(settings map[string]any) app.Module {
	return &staticModule{settings: settings}
}

type staticModule struct {
	settings map[string]any
}

var _ app.Provider = (*staticModule)(nil)

func (m *staticModule) Name() string { return "config" }

func (m *staticModule) Provide(_ context.Context, i do.Injector) error {
	k := koanf.New(delim)
	if err := k.Load(confmap.Provider(m.settings, delim), nil); err != nil {
		return fmt.Errorf("config: failed to load static settings: %w", err)
	}

	do.ProvideValue(i, &Config{k: k, validate: newValidator()})
	return nil
}

// Unmarshal decodes the settings under key into out and leaves fields that no
// file or variable sets at their existing value.
func (c *Config) Unmarshal(key string, out any) error {
	conf := koanf.UnmarshalConf{
		Tag: tag,
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.TextUnmarshallerHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
			WeaklyTypedInput: true,
			// Without ErrorUnused a misspelled key silently keeps the default.
			ErrorUnused: true,
		},
	}

	if err := c.k.UnmarshalWithConf(key, out, conf); err != nil {
		return fmt.Errorf("config: failed to decode %q: %w", key, err)
	}

	if err := c.validate.Struct(out); err != nil {
		return validationError(key, err)
	}

	return nil
}

// Load decodes the settings under key onto cfg, which carries the defaults a
// key no source sets keeps. component names the module in the error an absent
// config produces.
func Load[T any](i do.Injector, component, key string, cfg T) (T, error) {
	conf, err := do.Invoke[*Config](i)
	if err != nil {
		return cfg, fmt.Errorf("%s: failed to invoke the config: %w", component, err)
	}

	if err := conf.Unmarshal(key, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func newKoanf(opts Options) (*koanf.Koanf, error) {
	if !strings.HasSuffix(opts.EnvPrefix, envDelim) {
		return nil, fmt.Errorf("config: the environment prefix %q has to end in %q", opts.EnvPrefix, envDelim)
	}

	path, named := opts.File, opts.File != ""
	if override := strings.TrimSpace(os.Getenv(opts.EnvPrefix + fileVar)); override != "" {
		path, named = override, true
	}
	if path == "" {
		path = defaultFile
	}

	k := koanf.New(delim)

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		// Naming a path claims the file exists, so a missing path is a mistake.
		if named || !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config: failed to read %s: %w", path, err)
		}
	}

	local := localPath(path)
	if err := k.Load(file.Provider(local), yaml.Parser()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config: failed to read %s: %w", local, err)
	}

	provider := env.Provider(delim, env.Opt{Prefix: opts.EnvPrefix, TransformFunc: transform(opts.EnvPrefix)})
	if err := k.Load(provider, nil); err != nil {
		return nil, fmt.Errorf("config: failed to read the environment: %w", err)
	}

	return k, nil
}

func localPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + localSuffix + ext
}

func transform(prefix string) func(string, string) (string, any) {
	return func(key, value string) (string, any) {
		key = strings.TrimPrefix(key, prefix)
		if key == fileVar {
			return "", nil
		}

		return strings.ToLower(strings.ReplaceAll(key, envDelim, delim)), value
	}
}

func newValidator() *validator.Validate {
	validate := validator.New(validator.WithRequiredStructEnabled())

	// Reports fields by their config key instead of their Go field name.
	validate.RegisterTagNameFunc(func(f reflect.StructField) string {
		return strings.SplitN(f.Tag.Get(tag), ",", 2)[0]
	})

	return validate
}

func validationError(key string, err error) error {
	var failures validator.ValidationErrors
	if !errors.As(err, &failures) {
		return fmt.Errorf("config: failed to validate %q: %w", key, err)
	}

	errs := make([]error, len(failures))
	for i, failure := range failures {
		rule := failure.Tag()
		if failure.Param() != "" {
			rule += "=" + failure.Param()
		}

		errs[i] = fmt.Errorf("config: %s failed rule %q", key+delim+fieldPath(failure), rule)
	}

	return errors.Join(errs...)
}

// fieldPath drops the Go type name validator puts in front of the path.
func fieldPath(failure validator.FieldError) string {
	if _, rest, ok := strings.Cut(failure.Namespace(), delim); ok {
		return rest
	}

	return failure.Field()
}
