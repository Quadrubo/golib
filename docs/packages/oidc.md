# oidc

The `oidc` package runs the calls a confidential OpenID Connect client makes
to its provider: the code exchange of a sign in. The authorization request,
the sign out at the provider and the session of the service belong to the
consumer.

## Usage

`oidc.Module` reads `modules.oidc` and provides `*oidc.Client`. It takes
`issuer`, `client_id`, `client_secret` and `public_url`, all required, and
`request_timeout` and `dangerously_allow_insecure_http`. The module takes the
path of the redirect URI, which it joins to `public_url`. `public_url` is a
plain origin, and the module refuses to start when it carries userinfo, a
path, a query or a fragment. The module serializes it as the WHATWG URL
standard serializes an origin, with a lower-case scheme and host and without
the default port, so the web server and the module build the same redirect
URI. The module also refuses an issuer with userinfo, a query or a fragment
(OpenID Connect Discovery 1.0 section 2).

```go
oidc.Module(oidc.Paths{Redirect: "authentication/callback"})

result, err := client.SignIn(ctx, oidc.SignInRequest{
    Code: code, CodeVerifier: verifier, Nonce: nonce,
})
```

The consumer sends `code_challenge_method=S256` and a nonce in the
authorization request, and `SignIn` rejects a request without a nonce or a
code verifier. `SignIn` returns a `SignInResult` with the `Identity` of the
user and the raw `IDToken`, which the consumer sends as `id_token_hint` when
it signs out at the provider.

`SignIn` returns the error of the caller's context, unwrapped, when that
context has ended. Every other error falls into one of two groups:

- The sign in is rejected. `oidc.ErrCodeRefused` is a code that the provider
  answers with `invalid_grant`. `oidc.ErrTokensRejected` is a sign in whose
  tokens fail a check, such as a missing ID token, a failed signature check,
  or another issuer, audience, authorized party, nonce or userinfo subject. A
  signature check fails with `ErrTokensRejected` for any cause, also for a key
  set that fails to load. The error text keeps the cause.
- The provider failed. `oidc.ErrProvider` is a failed request, a request that
  ran out of `request_timeout`, any other error answer, a body that is no
  JSON or larger than 1 MiB, and a discovery document the package cannot use.
  It keeps only the text of its cause and never matches an error such as
  `context.DeadlineExceeded`.

`SignIn` cuts the text of an error longer than 512 bytes, such as one that
quotes an HTML page of the provider, and ends it with "…". The error still
matches its sentinel and the error of the context.

The `Issuer` of the identity is the configured issuer, which OpenID Connect
Core section 3.1.3.7 requires the `iss` of the ID token to match.

## Mechanics

The libraries do the protocol. `github.com/coreos/go-oidc/v3` runs the
discovery, fetches the JWKS, fetches it again after any failed signature
check, and checks the signature, the issuer, the audience and the expiry of an
ID token. `golang.org/x/oauth2` runs the code exchange with the PKCE verifier,
authenticates the client with `client_secret_basic` and parses the error of
the token endpoint.

The package adds what the libraries leave to a client. `SignIn` requires a
subject (OpenID Connect Core section 2), compares the nonce in constant time,
checks the authorized party, reads userinfo and rejects a subject that differs
from the ID token. The package fetches userinfo itself, since the userinfo
call of go-oidc checks no `iss` or `aud` of a signed answer. Both verifiers
come from the discovered go-oidc provider and share its key set. A userinfo
answer of type `application/jwt` passes the userinfo verifier, which checks
the signature, `iss` and `aud` (OpenID Connect Core section 5.3.2) but no
expiry. Any other userinfo answer is read as plain JSON. The profile claims
come from userinfo and fall back to the ID token.

Both verifiers accept only the signing algorithms that the discovery lists and
the package supports. The ID token verifier takes
`id_token_signing_alg_values_supported`, and a document whose list holds no
supported algorithm fails the discovery with `ErrProvider`. A document that
lists no algorithm leaves the list empty, and go-oidc then accepts RS256
alone. The userinfo verifier takes the supported algorithms of
`userinfo_signing_alg_values_supported` when the discovery lists any there,
and the algorithms of the ID token otherwise. When that list holds no
supported algorithm, such as a list of `none` alone (OpenID Connect Discovery
1.0 section 3), every signed userinfo answer fails with `ErrTokensRejected`.

A token endpoint answer without an OAuth error code fails with `ErrProvider`
and its HTTP status alone, since its body can be a large HTML page.

The discovery runs on the first call that needs it, and the provider it finds
is kept. Parallel first calls share one discovery, and each call stops waiting
at its own deadline. The shared discovery ignores the deadline of the call that
started it, so a short deadline never fails the calls that joined. Every
request to the provider runs under `request_timeout`, 5 seconds by default.

The transport of the package fails the read of any body after 1 MiB. The
go-oidc verifier returns a failed key set fetch and a bad signature as the
same plain error, so the package does not tell them apart.

## Decisions

The issuer and `public_url` have to be https, and so do the token, userinfo
and JWKS endpoints the discovery lists. `localhost` and the loopback addresses
may use http. Any other http URL needs `dangerously_allow_insecure_http`,
since http sends the client secret and the tokens in plain text. A subdomain
of `localhost` is no exception. A resolver can map it to another host, such
as a Docker network alias.

A discovery document that lists `code_challenge_methods_supported` without
S256 fails with `ErrProvider`, since the consumer sends the S256 challenge of
RFC 7636 section 4.2. A document without the list passes, since OpenID
Connect Discovery 1.0 does not define the list.

The client follows no redirect of the provider. The discovery names every
endpoint, so a redirect is no part of the protocol. A redirect of a request
the server makes, such as the code exchange, could send the client
credentials to another host. Checking the https rule on every hop instead
would still send the credentials to a host the discovery never named.

The redirect URI comes from `public_url` rather than from each call, so a
caller cannot send the provider a URI the operator did not configure. The
path belongs to the consumer's routes and stays in its code.

The provider failures form one group. An unavailable provider and a
misconfigured one differ only by a guess from the status and the body of an
answer. A consumer shows one answer for both, and the log carries the cause.

The ID token passes with several audiences only when `azp` names the client.
Pocket ID sends the issuer as a second audience.

The package takes no scopes. The consumer builds the authorization request,
which alone carries them, and the code exchange does not send them.
