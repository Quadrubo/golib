package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

const envPrefix = "CONFIGTEST__"

type serverConfig struct {
	Addr        string        `config:"addr"`
	ReadTimeout time.Duration `config:"read_timeout"`
}

type tlsConfig struct {
	Cert string `config:"cert" validate:"required"`
}

type securedConfig struct {
	TLS tlsConfig `config:"tls"`
}

type modulesConfig struct {
	Server serverConfig `config:"server"`
}

type level struct {
	name string
}

func (l *level) UnmarshalText(text []byte) error {
	if string(text) == "loud" {
		return errors.New("unknown level")
	}

	l.name = strings.ToUpper(string(text))
	return nil
}

type loggerConfig struct {
	Level level `config:"level"`
}

type bodyConfig struct {
	MaxBody config.Bytes `config:"max_body"`
}

type rootConfig struct {
	Modules modulesConfig `config:"modules"`
}

var _ = Describe("Config", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		Expect(os.WriteFile(path, []byte(body), 0o600)).To(Succeed())
		return path
	}

	setenv := func(key, value string) {
		Expect(os.Setenv(key, value)).To(Succeed())
		DeferCleanup(os.Unsetenv, key)
	}

	chdir := func(path string) {
		GinkgoHelper()
		prev, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		Expect(os.Chdir(path)).To(Succeed())
		DeferCleanup(os.Chdir, prev)
	}

	load := func(modules ...app.Module) (*config.Config, error) {
		a, err := app.New(context.Background(), modules)
		if err != nil {
			return nil, err
		}
		return do.Invoke[*config.Config](a.Injector())
	}

	fromFile := func(body string) *config.Config {
		GinkgoHelper()
		conf, err := load(config.Module(config.Options{
			EnvPrefix: envPrefix,
			File:      write("config.yaml", body),
		}))
		Expect(err).ToNot(HaveOccurred())
		return conf
	}

	Describe("files", func() {
		It("reads the base file", func() {
			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":50051"))
		})

		It("overlays the local file on top of the base", func() {
			write("config.local.yaml", "modules:\n  server:\n    addr: \":9999\"\n")
			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n    read_timeout: 5s\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":9999"))
			Expect(out.ReadTimeout).To(Equal(5 * time.Second))
		})

		It("ignores a missing local file", func() {
			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
		})

		It("fails when a named file is missing", func() {
			missing := filepath.Join(dir, "absent.yaml")

			_, err := load(config.Module(config.Options{EnvPrefix: envPrefix, File: missing}))

			Expect(err).To(MatchError(os.ErrNotExist))
			Expect(err.Error()).To(ContainSubstring(missing))
		})

		It("fails when the environment names a missing file", func() {
			missing := filepath.Join(dir, "absent.yaml")
			setenv(envPrefix+"CONFIG_FILE", missing)

			_, err := load(config.Module(config.Options{EnvPrefix: envPrefix}))

			Expect(err).To(MatchError(os.ErrNotExist))
			Expect(err.Error()).To(ContainSubstring(missing))
		})

		It("runs on the environment alone when no file is named", func() {
			chdir(dir)
			setenv(envPrefix+"MODULES__SERVER__ADDR", ":8080")

			conf, err := load(config.Module(config.Options{EnvPrefix: envPrefix}))
			Expect(err).ToNot(HaveOccurred())

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":8080"))
		})

		It("still fails when the default file cannot be parsed", func() {
			chdir(dir)
			write("config.yaml", "modules:\n\tserver: oops\n")

			_, err := load(config.Module(config.Options{EnvPrefix: envPrefix}))

			Expect(err).To(HaveOccurred())
			Expect(err).ToNot(MatchError(os.ErrNotExist))
		})

		It("takes the config file from the environment", func() {
			elsewhere := write("elsewhere.yaml", "modules:\n  server:\n    addr: \":7000\"\n")
			setenv(envPrefix+"CONFIG_FILE", elsewhere)

			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":7000"))
		})
	})

	Describe("environment", func() {
		It("overrides the files", func() {
			setenv(envPrefix+"MODULES__SERVER__ADDR", ":8080")

			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":8080"))
		})

		It("provides a setting that no file mentions", func() {
			setenv(envPrefix+"MODULES__SERVER__ADDR", ":8080")

			conf := fromFile("modules:\n  other:\n    unrelated: true\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":8080"))
		})

		It("addresses a key that contains an underscore", func() {
			setenv(envPrefix+"MODULES__SERVER__READ_TIMEOUT", "90s")

			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.ReadTimeout).To(Equal(90 * time.Second))
		})

		It("leaves variables outside the prefix alone", func() {
			setenv("OTHER_MODULES__SERVER__ADDR", ":8080")

			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":50051"))
		})

		It("leaves the variables Kubernetes injects for a like-named service alone", func() {
			setenv("CONFIGTEST_SERVICE_HOST", "10.0.0.1")
			setenv("CONFIGTEST_PORT_9001_TCP_ADDR", "10.0.0.1")

			conf := fromFile("modules:\n  server:\n    addr: \":50051\"\n")

			var out rootConfig
			Expect(conf.Unmarshal("", &out)).To(Succeed())
			Expect(out.Modules.Server.Addr).To(Equal(":50051"))
		})

		It("keeps the config file variable out of the settings", func() {
			base := write("config.yaml", "modules:\n  server:\n    addr: \":50051\"\n")
			setenv(envPrefix+"CONFIG_FILE", base)

			conf, err := load(config.Module(config.Options{EnvPrefix: envPrefix, File: base}))
			Expect(err).ToNot(HaveOccurred())

			var out rootConfig
			Expect(conf.Unmarshal("", &out)).To(Succeed())
			Expect(out.Modules.Server.Addr).To(Equal(":50051"))
		})

		It("refuses a prefix that stops short of the delimiter", func() {
			_, err := load(config.Module(config.Options{
				EnvPrefix: "CONFIGTEST_",
				File:      write("config.yaml", "{}\n"),
			}))

			Expect(err).To(MatchError(ContainSubstring(`prefix "CONFIGTEST_" has to end in "__"`)))
		})

		It("refuses an empty prefix", func() {
			_, err := load(config.Module(config.Options{File: write("config.yaml", "{}\n")}))

			Expect(err).To(MatchError(ContainSubstring(`has to end in "__"`)))
		})
	})

	Describe("decoding", func() {
		It("leaves defaults in place when nothing sets the key", func() {
			conf := fromFile("modules:\n  other:\n    unrelated: true\n")

			out := serverConfig{Addr: ":50051", ReadTimeout: time.Minute}
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":50051"))
			Expect(out.ReadTimeout).To(Equal(time.Minute))
		})

		It("overrides only the settings that are present", func() {
			conf := fromFile("modules:\n  server:\n    read_timeout: 5s\n")

			out := serverConfig{Addr: ":50051", ReadTimeout: time.Minute}
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":50051"))
			Expect(out.ReadTimeout).To(Equal(5 * time.Second))
		})

		It("rejects a key the struct does not have", func() {
			conf := fromFile("modules:\n  server:\n    adrr: \":50051\"\n")

			var out serverConfig
			err := conf.Unmarshal("modules.server", &out)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("adrr"))
		})

		It("hands a value to the type that knows how to parse it", func() {
			conf := fromFile("modules:\n  logger:\n    level: debug\n")

			var out loggerConfig
			Expect(conf.Unmarshal("modules.logger", &out)).To(Succeed())
			Expect(out.Level.name).To(Equal("DEBUG"))
		})

		It("reports what that type rejected", func() {
			conf := fromFile("modules:\n  logger:\n    level: loud\n")

			var out loggerConfig
			err := conf.Unmarshal("modules.logger", &out)

			Expect(err).To(MatchError(ContainSubstring("unknown level")))
		})

		It("parses a byte count with its binary unit", func() {
			conf := fromFile("modules:\n  server:\n    max_body: 128MiB\n")

			var out bodyConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.MaxBody).To(BeEquivalentTo(128 * 1024 * 1024))
		})

		It("takes a bare number as bytes", func() {
			conf := fromFile("modules:\n  server:\n    max_body: 4096\n")

			var out bodyConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.MaxBody).To(BeEquivalentTo(4096))
		})

		It("takes a quoted number as bytes, which the environment sends", func() {
			conf := fromFile("modules:\n  server:\n    max_body: \"4096\"\n")

			var out bodyConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.MaxBody).To(BeEquivalentTo(4096))
		})

		It("reports a byte unit it does not know", func() {
			conf := fromFile("modules:\n  server:\n    max_body: 128MB\n")

			var out bodyConfig
			err := conf.Unmarshal("modules.server", &out)

			Expect(err).To(MatchError(ContainSubstring(`unsupported byte unit "MB"`)))
		})

		It("reports a byte count that does not parse", func() {
			conf := fromFile("modules:\n  server:\n    max_body: lots\n")

			var out bodyConfig
			err := conf.Unmarshal("modules.server", &out)

			Expect(err).To(MatchError(ContainSubstring(`unsupported byte count "lots"`)))
		})

		It("names a failing setting by its full config path", func() {
			conf := fromFile("modules:\n  server:\n    tls:\n      cert: \"\"\n")

			var out securedConfig
			err := conf.Unmarshal("modules.server", &out)

			Expect(err).To(MatchError(ContainSubstring("modules.server.tls.cert")))
			Expect(err).To(MatchError(ContainSubstring(`failed rule "required"`)))
		})

		It("names a failing setting when the struct has no type name", func() {
			conf := fromFile("modules:\n  server:\n    addr: \"\"\n")

			out := struct {
				Addr string `config:"addr" validate:"required"`
			}{}
			err := conf.Unmarshal("modules.server", &out)

			Expect(err).To(MatchError(ContainSubstring("modules.server.addr")))
		})
	})

	Describe("static settings", func() {
		It("builds a config without touching disk", func() {
			conf, err := load(config.StaticModule(map[string]any{
				"modules.server.addr":         ":1234",
				"modules.server.read_timeout": "30s",
			}))
			Expect(err).ToNot(HaveOccurred())

			var out serverConfig
			Expect(conf.Unmarshal("modules.server", &out)).To(Succeed())
			Expect(out.Addr).To(Equal(":1234"))
			Expect(out.ReadTimeout).To(Equal(30 * time.Second))
		})
	})
})
