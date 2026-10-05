package oidc

import "github.com/go-jose/go-jose/v4"

type options struct {
	redirectURIs   []string
	algorithm      jose.SignatureAlgorithm
	listed         []string
	userInfoListed []string
}

type Option func(*options)

func WithRedirectURIs(uris ...string) Option {
	return func(o *options) { o.redirectURIs = append(o.redirectURIs, uris...) }
}

// WithSigningAlgorithm signs the tokens with the algorithm, RS256 by default.
// ES256 signs with an EC P-256 key, and RS* and PS* sign with an RSA key. New
// panics for any other algorithm.
func WithSigningAlgorithm(algorithm jose.SignatureAlgorithm) Option {
	return func(o *options) { o.algorithm = algorithm }
}

// WithListedAlgorithms replaces id_token_signing_alg_values_supported of the
// discovery document, which lists the signing algorithm by default.
func WithListedAlgorithms(algorithms ...string) Option {
	return func(o *options) { o.listed = algorithms }
}

// WithListedUserInfoAlgorithms sets userinfo_signing_alg_values_supported of
// the discovery document, which leaves it out by default.
func WithListedUserInfoAlgorithms(algorithms ...string) Option {
	return func(o *options) { o.userInfoListed = algorithms }
}
