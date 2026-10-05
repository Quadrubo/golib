package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

type provider struct {
	oidc             *gooidc.Provider
	verifier         *gooidc.IDTokenVerifier
	userInfoURL      string
	userInfoVerifier *gooidc.IDTokenVerifier
}

type metadata struct {
	TokenURL             string   `json:"token_endpoint"`
	UserInfoURL          string   `json:"userinfo_endpoint"`
	JWKSURL              string   `json:"jwks_uri"`
	Algorithms           []string `json:"id_token_signing_alg_values_supported"`
	UserInfoAlgorithms   []string `json:"userinfo_signing_alg_values_supported"`
	CodeChallengeMethods []string `json:"code_challenge_methods_supported"`
}

func (c *Client) discover(ctx context.Context) (*provider, error) {
	if p := c.discovered.Load(); p != nil {
		return p, nil
	}

	done := c.discovery.DoChan("discover", func() (any, error) {
		// The deadline of the first call does not end the discovery that other calls share.
		p, err := c.newProvider(context.WithoutCancel(ctx))
		if err == nil {
			c.discovered.Store(p)
		}

		return p, err
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		if result.Err != nil {
			return nil, result.Err
		}

		return result.Val.(*provider), nil
	}
}

func (c *Client) newProvider(ctx context.Context) (*provider, error) {
	discovered, err := gooidc.NewProvider(gooidc.ClientContext(ctx, c.http), c.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	var meta metadata
	if err := discovered.Claims(&meta); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	// The client calls the token endpoint, userinfo and the key set on every sign in.
	for _, required := range []struct{ name, url string }{
		{"token_endpoint", meta.TokenURL}, {"userinfo_endpoint", meta.UserInfoURL}, {"jwks_uri", meta.JWKSURL},
	} {
		if required.url == "" {
			return nil, fmt.Errorf("%w: the discovery document lists no %s", ErrProvider, required.name)
		}

		if err := checkURL(required.url, c.cfg.DangerouslyAllowInsecureHTTP); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
		}
	}

	algorithms := supported(meta.Algorithms)

	// go-oidc falls back to RS256 for an empty list, which a provider that lists other algorithms does not use.
	if len(algorithms) == 0 && len(meta.Algorithms) > 0 {
		return nil, fmt.Errorf("%w: the discovery document lists no supported signing algorithm in %q",
			ErrProvider, meta.Algorithms)
	}

	if len(meta.CodeChallengeMethods) > 0 && !slices.Contains(meta.CodeChallengeMethods, "S256") {
		return nil, fmt.Errorf("%w: the discovery document lists no S256 in %q",
			ErrProvider, meta.CodeChallengeMethods)
	}

	p := &provider{
		oidc: discovered,
		verifier: discovered.Verifier(&gooidc.Config{
			ClientID:             c.cfg.ClientID,
			SupportedSigningAlgs: algorithms,
		}),
		userInfoURL: meta.UserInfoURL,
	}

	userInfoAlgorithms := algorithms
	if len(meta.UserInfoAlgorithms) > 0 {
		userInfoAlgorithms = supported(meta.UserInfoAlgorithms)
	}

	if len(meta.UserInfoAlgorithms) == 0 || len(userInfoAlgorithms) > 0 {
		p.userInfoVerifier = discovered.Verifier(&gooidc.Config{
			ClientID:             c.cfg.ClientID,
			SupportedSigningAlgs: userInfoAlgorithms,
			// OpenID Connect Core section 5.3.2 requires no exp in a signed userinfo answer.
			SkipExpiryCheck: true,
		})
	}

	return p, nil
}

var signatureAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.PS256, jose.PS384, jose.PS512,
	jose.EdDSA,
}

func supported(listed []string) []string {
	var algorithms []string
	for _, algorithm := range signatureAlgorithms {
		if slices.Contains(listed, string(algorithm)) {
			algorithms = append(algorithms, string(algorithm))
		}
	}

	return algorithms
}

func (p *provider) verify(ctx context.Context, raw, clientID string) (*gooidc.IDToken, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: the response holds no ID token", ErrTokensRejected)
	}

	idToken, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokensRejected, err.Error())
	}

	// go-oidc leaves the authorized party to the client.
	var claims struct {
		AuthorizedParty string `json:"azp"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	if claims.AuthorizedParty == "" && len(idToken.Audience) > 1 {
		return nil, fmt.Errorf("%w: the ID token names several audiences and no authorized party", ErrTokensRejected)
	}

	if claims.AuthorizedParty != "" && claims.AuthorizedParty != clientID {
		return nil, fmt.Errorf("%w: the ID token names the authorized party %q", ErrTokensRejected, claims.AuthorizedParty)
	}

	return idToken, nil
}

func (p *provider) userInfo(ctx context.Context, client *http.Client, token *oauth2.Token) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to build the userinfo request: %s", ErrProvider, err.Error())
	}

	token.SetAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err == nil && resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("oidc: userinfo answered %s", resp.Status)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	if !signed(resp.Header.Get("Content-Type")) {
		return body, nil
	}

	if p.userInfoVerifier == nil {
		return nil, fmt.Errorf("%w: the discovery document lists no supported algorithm for a signed userinfo answer",
			ErrTokensRejected)
	}

	verified, err := p.userInfoVerifier.Verify(ctx, string(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokensRejected, err.Error())
	}

	var claims json.RawMessage
	if err := verified.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	return claims, nil
}

func signed(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)

	return err == nil && mediaType == "application/jwt"
}

const maxAnswerSize = 1 << 20

var errOversized = errors.New("oidc: the answer exceeds 1 MiB")

type limited struct{ base http.RoundTripper }

func (l limited) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.base.RoundTrip(req)
	if err == nil {
		resp.Body = &limitedBody{ReadCloser: resp.Body}
	}

	return resp, err
}

type limitedBody struct {
	io.ReadCloser
	read int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p[:min(int64(len(p)), maxAnswerSize+1-b.read)])
	b.read += int64(n)

	if b.read > maxAnswerSize {
		return n, errOversized
	}

	return n, err
}

func checkURL(raw string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("oidc: failed to parse %q: %w", raw, err)
	}

	switch {
	case parsed.Scheme == "https":
		return nil
	case parsed.Scheme != "http":
		return fmt.Errorf("oidc: unsupported scheme %q", parsed.Scheme)
	case isLocal(parsed.Hostname()) || allowInsecureHTTP:
		return nil
	default:
		return fmt.Errorf("oidc: %q must use https outside localhost", raw)
	}
}
