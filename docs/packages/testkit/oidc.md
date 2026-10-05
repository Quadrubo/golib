# testkit/oidc

The `testkit/oidc` package runs an OpenID provider for a suite and points the
service at it.

## Usage

`oidc.New` returns a `testkit.Dependency`, which a suite declares in one line.
`Start` puts the `*oidc.Issuer` into the suite injector. `WithRedirectURIs`
registers the redirect URIs of the client. `WithSigningAlgorithm` signs the
tokens with another algorithm than RS256, and `WithListedAlgorithms` replaces
the `id_token_signing_alg_values_supported` of the discovery document, which
lists the signing algorithm by default. `WithListedUserInfoAlgorithms` sets
`userinfo_signing_alg_values_supported`, which the document leaves out by
default.

```go
Dependencies: []testkit.Dependency{tkoidc.New(
    tkoidc.WithRedirectURIs("http://localhost:3000/authentication/callback"),
)},

issuer := do.MustInvoke[*tkoidc.Issuer](suite.Injector())
code := issuer.Code(tkoidc.Grant{
    User:          user,
    Nonce:         nonce,
    CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier),
    RedirectURI:   "http://localhost:3000/authentication/callback",
})
```

A spec describes a sign in as a `Grant` and trades it for a code through
`Code`, which stands in for the authorization request a browser runs. The
grant carries the S256 code challenge and the redirect URI of that request.
The service then exchanges the code like any other.

A grant shapes the tokens. `IDTokenClaims` and `UserInfoClaims` replace claims
of the ID token and of userinfo, and a nil value removes a claim.
`OmitIDToken` leaves the ID token out of the token response,
`IDTokenForeignKey` signs the ID token with a key the JWKS does not hold,
`SignUserInfo` makes userinfo answer a signed JWT with `iss` and `aud`,
`UserInfoForeignKey` signs that JWT with a key the JWKS does not hold,
`UserInfoAlgorithm` signs it with another algorithm than the issuer's,
`UserInfoSubject` makes userinfo name another subject, `UserInfoStatus` makes
userinfo answer the usual claims under that status, and `UserInfoBody` makes
userinfo answer status 200 with that body. `UserInfoStatus` has no effect
together with `UserInfoBody` or `SignUserInfo`. With `UserInfoBody`, the
issuer ignores `UserInfoClaims`, `UserInfoSubject`, `SignUserInfo`,
`UserInfoForeignKey` and `UserInfoAlgorithm`.

The fault of a subject: `FailExchange` answers its code exchanges with status
503 and `temporarily_unavailable`. The faults of the issuer: `RotateKey` signs
every later token with a new key, `FailKeySet` answers the JWKS with the
status it takes, until a call with status 0 serves the keys again, and
`PadKeySet` adds a member of the size it takes to the JWKS, until a call with
size 0 removes it.

`RedirectURI` returns the `redirect_uri` that the last code exchange of a
subject sent. `Requests` returns how many requests the issuer received on a
path, such as `/jwks`.

## Mechanics

The issuer serves discovery, the JWKS, the token endpoint and userinfo on an
`httptest` server. It signs with keys that it generates, an EC P-256 key for
ES256 and an RSA key for each RS and PS algorithm. `New` panics for any other
algorithm, since the issuer has no key that matches it. `Settings` hands the
service `modules.oidc.issuer`, `client_id` and `client_secret`.

The token endpoint takes `client_secret_basic` and the grant
`authorization_code` alone, and it issues no refresh token. It form-decodes
the client ID and the client secret of the Basic credentials, as RFC 6749
section 2.3.1 requires. The client secret holds characters that the form
encoding escapes, so a client that sends the secret unencoded fails. A code
works once. The exchange must carry a verifier of 43 to 128 characters (RFC
7636 section 4.1), or it fails with `invalid_request`. The S256 challenge of
the verifier must match the grant, the exchange must carry exactly the
redirect URI of the grant, and that URI must be registered.

## Decisions

The faults a spec needs belong to a subject where they can, so parallel specs
never see the fault of another one. The key set belongs to the whole issuer,
so a spec that fails it restores it before it ends.
