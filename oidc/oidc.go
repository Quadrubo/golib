package oidc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/samber/do/v2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
)

const configKey = "modules.oidc"

type Config struct {
	Issuer       string `config:"issuer" validate:"required,url"`
	ClientID     string `config:"client_id" validate:"required"`
	ClientSecret string `config:"client_secret" validate:"required"`
	PublicURL    string `config:"public_url" validate:"required,url"`

	RequestTimeout time.Duration `config:"request_timeout" validate:"gt=0"`

	DangerouslyAllowInsecureHTTP bool `config:"dangerously_allow_insecure_http"`
}

// Paths are the paths under public_url that the client uses.
type Paths struct {
	Redirect string
}

func Module(paths Paths) app.Module { return &module{paths: paths} }

type module struct {
	paths Paths
}

var _ app.Provider = (*module)(nil)

func (m *module) Name() string { return "oidc" }

func (m *module) Provide(_ context.Context, i do.Injector) error {
	if m.paths.Redirect == "" {
		return errors.New("oidc: the paths hold no redirect")
	}

	cfg, err := config.Load(i, m.Name(), configKey, Config{RequestTimeout: 5 * time.Second})
	if err != nil {
		return err
	}

	if err := checkIssuer(cfg.Issuer); err != nil {
		return err
	}

	publicURL, err := origin(cfg.PublicURL)
	if err != nil {
		return err
	}

	for _, raw := range []string{cfg.Issuer, publicURL} {
		if err := checkURL(raw, cfg.DangerouslyAllowInsecureHTTP); err != nil {
			return err
		}
	}

	redirectURI, err := url.JoinPath(publicURL, m.paths.Redirect)
	if err != nil {
		return fmt.Errorf("oidc: failed to join %q to public_url: %w", m.paths.Redirect, err)
	}

	do.ProvideValue(i, newClient(cfg, redirectURI))

	return nil
}

// checkIssuer applies OpenID Connect Discovery 1.0 section 2.
func checkIssuer(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("oidc: failed to parse %q: %w", raw, err)
	}

	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("oidc: issuer %q must carry no userinfo, query or fragment", raw)
	}

	return nil
}

// origin serializes public_url as the WHATWG URL standard serializes an origin.
func origin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("oidc: failed to parse %q: %w", raw, err)
	}

	if parsed.User != nil || strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", fmt.Errorf("oidc: public_url %q must be a plain origin without userinfo, path, query or fragment", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())

	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	if port := parsed.Port(); port != "" && defaultPorts[scheme] != port {
		host += ":" + port
	}

	return (&url.URL{Scheme: scheme, Host: host}).String(), nil
}

var defaultPorts = map[string]string{"http": "80", "https": "443"}

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
