package oidc

import (
	"cmp"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
)

var (
	ErrCodeRefused    = errors.New("oidc: the provider refused the code")
	ErrTokensRejected = errors.New("oidc: the tokens failed a check")
	ErrProvider       = errors.New("oidc: the provider failed")
)

type Identity struct {
	Issuer            string
	Subject           string
	Email             string
	Name              string
	PreferredUsername string
}

type SignInRequest struct {
	Code         string
	CodeVerifier string
	Nonce        string
}

type SignInResult struct {
	Identity Identity
	IDToken  string
}

type Client struct {
	cfg         Config
	redirectURI string
	http        *http.Client

	discovered atomic.Pointer[provider]
	discovery  singleflight.Group
}

func newClient(cfg Config, redirectURI string) *Client {
	return &Client{
		cfg:         cfg,
		redirectURI: redirectURI,
		http: &http.Client{
			Timeout:       cfg.RequestTimeout,
			Transport:     limited{base: http.DefaultTransport},
			CheckRedirect: refuseRedirect,
		},
	}
}

func refuseRedirect(req *http.Request, via []*http.Request) error {
	return fmt.Errorf("oidc: %s redirected to %s", via[0].URL.Redacted(), req.URL.Redacted())
}

func (c *Client) SignIn(ctx context.Context, req SignInRequest) (SignInResult, error) {
	result, err := c.signIn(ctx, req)
	if err != nil && ctx.Err() != nil {
		return SignInResult{}, ctx.Err()
	}

	if err != nil && len(err.Error()) > maxErrorLength {
		return SignInResult{}, shortened{err}
	}

	return result, err
}

const maxErrorLength = 512

type shortened struct{ error }

func (s shortened) Error() string {
	return strings.ToValidUTF8(s.error.Error()[:maxErrorLength], "") + "…"
}

func (s shortened) Unwrap() error { return s.error }

func (c *Client) signIn(ctx context.Context, req SignInRequest) (SignInResult, error) {
	if req.Nonce == "" || req.CodeVerifier == "" {
		return SignInResult{}, fmt.Errorf("%w: the sign in carries no nonce or no code verifier", ErrTokensRejected)
	}

	p, err := c.discover(ctx)
	if err != nil {
		return SignInResult{}, err
	}

	ctx = c.context(ctx)

	token, err := c.oauth2(p).Exchange(ctx, req.Code, oauth2.VerifierOption(req.CodeVerifier))
	if err != nil {
		var retrieve *oauth2.RetrieveError
		// RFC 6749 section 5.2 answers a code the provider does not honour with invalid_grant.
		if errors.As(err, &retrieve) && retrieve.ErrorCode == "invalid_grant" {
			return SignInResult{}, fmt.Errorf("%w: %s", ErrCodeRefused, err.Error())
		}

		// The text of a RetrieveError without an error code carries the whole body of the answer.
		if retrieve != nil && retrieve.ErrorCode == "" && retrieve.Response != nil {
			return SignInResult{}, fmt.Errorf("%w: the token endpoint answered %s", ErrProvider, retrieve.Response.Status)
		}

		return SignInResult{}, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	raw, _ := token.Extra("id_token").(string)
	idToken, err := p.verify(ctx, raw, c.cfg.ClientID)
	if err != nil {
		return SignInResult{}, err
	}

	// OpenID Connect Core section 2 requires sub, and go-oidc leaves the check to the client.
	if idToken.Subject == "" {
		return SignInResult{}, fmt.Errorf("%w: the ID token names no subject", ErrTokensRejected)
	}

	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(req.Nonce)) != 1 {
		return SignInResult{}, fmt.Errorf("%w: the nonce differs", ErrTokensRejected)
	}

	var fromIDToken profile
	if err := idToken.Claims(&fromIDToken); err != nil {
		return SignInResult{}, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	claims, err := p.userInfo(ctx, c.http, token)
	if err != nil {
		return SignInResult{}, err
	}

	var fromUserInfo struct {
		Subject string `json:"sub"`
		profile
	}
	if err := json.Unmarshal(claims, &fromUserInfo); err != nil {
		return SignInResult{}, fmt.Errorf("%w: %s", ErrProvider, err.Error())
	}

	if fromUserInfo.Subject != idToken.Subject {
		return SignInResult{}, fmt.Errorf("%w: userinfo names subject %q instead of %q",
			ErrTokensRejected, fromUserInfo.Subject, idToken.Subject)
	}

	identity := Identity{
		Issuer:            c.cfg.Issuer,
		Subject:           idToken.Subject,
		Email:             cmp.Or(fromUserInfo.Email, fromIDToken.Email),
		Name:              cmp.Or(fromUserInfo.Name, fromIDToken.Name),
		PreferredUsername: cmp.Or(fromUserInfo.PreferredUsername, fromIDToken.PreferredUsername),
	}

	return SignInResult{Identity: identity, IDToken: raw}, nil
}

func (c *Client) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.http)
}

func (c *Client) oauth2(p *provider) *oauth2.Config {
	endpoint := p.oidc.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInHeader

	return &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     endpoint,
		RedirectURL:  c.redirectURI,
	}
}

type profile struct {
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}
