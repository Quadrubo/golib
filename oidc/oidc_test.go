package oidc_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"golang.org/x/oauth2"

	"github.com/quadrubo/golib/app"
	"github.com/quadrubo/golib/config"
	"github.com/quadrubo/golib/logging"
	"github.com/quadrubo/golib/oidc"
	tkoidc "github.com/quadrubo/golib/testkit/oidc"
)

func TestOIDC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OIDC Suite")
}

const (
	publicURL     = "http://localhost:3000"
	redirectURI   = publicURL + "/authentication/callback"
	elsewhereURI  = "http://localhost:4000/authentication/callback"
	otherRedirect = publicURL + "/other/callback"
)

var (
	paths        = oidc.Paths{Redirect: "authentication/callback"}
	codeVerifier = strings.Repeat("v", 43)
)

// discover runs the discovery through a sign in, whose code exchange follows only a discovery that passed.
func discover(ctx context.Context, c *oidc.Client) error {
	_, err := c.SignIn(ctx, oidc.SignInRequest{Code: "code", CodeVerifier: codeVerifier, Nonce: "nonce-123"})

	return err
}

func boot(settings map[string]any) (*oidc.Client, error) {
	a, err := app.New(context.Background(), []app.Module{
		config.StaticModule(settings),
		logging.Module(logging.WithWriter(io.Discard), logging.WithoutDefault()),
		oidc.Module(paths),
	})
	if err != nil {
		return nil, err
	}

	return do.MustInvoke[*oidc.Client](a.Injector()), nil
}

func flat(nested map[string]any) map[string]any {
	out := map[string]any{"modules.oidc.public_url": publicURL}
	for key, value := range nested["modules"].(map[string]any)["oidc"].(map[string]any) {
		out["modules.oidc."+key] = value
	}

	return out
}

func standalone(issuer string, extra map[string]any) map[string]any {
	settings := map[string]any{
		"modules.oidc.issuer":        issuer,
		"modules.oidc.client_id":     "client",
		"modules.oidc.client_secret": "s3cr+t/%:&",
		"modules.oidc.public_url":    publicURL,
	}
	maps.Copy(settings, extra)

	return settings
}

func refuseGrant(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
}

func discoveryDocument(base string) map[string]any {
	return map[string]any{
		"issuer":                 base,
		"authorization_endpoint": base + "/authorize",
		"token_endpoint":         base + "/token",
		"userinfo_endpoint":      base + "/userinfo",
		"jwks_uri":               base + "/jwks",
	}
}

// discoveryServer serves the discovery document after edit and every other
// path through handle, or with invalid_grant when handle is nil.
func discoveryServer(edit func(doc map[string]any), handle http.HandlerFunc) *httptest.Server {
	if handle == nil {
		handle = refuseGrant
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			handle(w, r)

			return
		}

		doc := discoveryDocument(server.URL)
		if edit != nil {
			edit(doc)
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	DeferCleanup(server.Close)

	return server
}

var _ = Describe("Client", func() {
	var (
		issuer *tkoidc.Issuer
		client *oidc.Client
		user   tkoidc.User
	)

	BeforeEach(func() {
		issuer = tkoidc.New(tkoidc.WithRedirectURIs(redirectURI, otherRedirect))
		Expect(issuer.Start(context.Background(), do.New())).To(Succeed())
		DeferCleanup(issuer.Stop, context.Background())

		var err error
		client, err = boot(flat(issuer.Settings()))
		Expect(err).ToNot(HaveOccurred())

		user = tkoidc.User{
			Subject: "alice-1", Email: "alice@example.test", Name: "Alice", PreferredUsername: "alice",
		}
	})

	grant := func(edit func(*tkoidc.Grant)) tkoidc.Grant {
		g := tkoidc.Grant{
			User:          user,
			Nonce:         "nonce-123",
			CodeChallenge: oauth2.S256ChallengeFromVerifier(codeVerifier),
			RedirectURI:   redirectURI,
		}
		if edit != nil {
			edit(&g)
		}

		return g
	}

	signInWith := func(c *oidc.Client, g tkoidc.Grant) (oidc.SignInResult, error) {
		return c.SignIn(context.Background(), oidc.SignInRequest{
			Code: issuer.Code(g), CodeVerifier: codeVerifier, Nonce: "nonce-123",
		})
	}

	signIn := func(g tkoidc.Grant) (oidc.SignInResult, error) { return signInWith(client, g) }

	rebooted := func(extra map[string]any) *oidc.Client {
		GinkgoHelper()

		settings := flat(issuer.Settings())
		maps.Copy(settings, extra)

		c, err := boot(settings)
		Expect(err).ToNot(HaveOccurred())

		return c
	}

	Describe("SignIn", func() {
		It("returns the identity and the ID token of the grant", func() {
			result, err := signIn(grant(nil))

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Identity).To(Equal(oidc.Identity{
				Issuer: issuer.URL(), Subject: "alice-1",
				Email: "alice@example.test", Name: "Alice", PreferredUsername: "alice",
			}))
			Expect(result.IDToken).ToNot(BeEmpty())
		})

		It("takes each profile claim from userinfo and from the ID token where userinfo lacks it", func() {
			result, err := signIn(grant(func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"name": "Alice of the ID token", "preferred_username": "alice-id-token"}
				g.UserInfoClaims = map[string]any{"preferred_username": nil}
			}))

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Identity.Name).To(Equal("Alice"))
			Expect(result.Identity.PreferredUsername).To(Equal("alice-id-token"))
		})

		It("rejects a token response without an ID token", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) { g.OmitIDToken = true }))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
			Expect(err).To(MatchError(ContainSubstring("the response holds no ID token")))
		})

		It("rejects an ID token without a subject", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) { g.User.Subject = "" }))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		DescribeTable("fails with ErrProvider for a failure of the provider",
			func(failingSignIn func() error) {
				Expect(failingSignIn()).To(MatchError(oidc.ErrProvider))
			},
			Entry("a 503 of the token endpoint", func() error {
				issuer.FailExchange(user.Subject)

				_, err := signIn(grant(nil))

				return err
			}),
			Entry("a 404 of the discovery", func() error {
				server := httptest.NewServer(http.NotFoundHandler())
				DeferCleanup(server.Close)
				failing, err := boot(standalone(server.URL, nil))
				Expect(err).ToNot(HaveOccurred())

				return discover(context.Background(), failing)
			}),
			Entry("a userinfo answer that is no JSON", func() error {
				_, err := signIn(grant(func(g *tkoidc.Grant) { g.UserInfoBody = "<html>Welcome</html>" }))

				return err
			}),
			Entry("a userinfo answer over 1 MiB", func() error {
				_, err := signIn(grant(func(g *tkoidc.Grant) {
					g.UserInfoBody = `{"sub":"alice-1","padding":"` + strings.Repeat("a", 1<<20) + `"}`
				}))

				return err
			}),
			Entry("a discovery answer over 1 MiB", func() error {
				server := discoveryServer(func(doc map[string]any) { doc["padding"] = strings.Repeat("a", 1<<20) }, nil)
				oversized, err := boot(standalone(server.URL, nil))
				Expect(err).ToNot(HaveOccurred())

				return discover(context.Background(), oversized)
			}),
			Entry("a userinfo answer with status 503 and the claims", func() error {
				_, err := signIn(grant(func(g *tkoidc.Grant) { g.UserInfoStatus = http.StatusServiceUnavailable }))

				return err
			}),
		)

		It("leaves the body of a token endpoint answer without an error code out of the error", func() {
			page := "<html>" + strings.Repeat("gateway trouble ", 1000) + "</html>"
			server := discoveryServer(nil, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(page))
			})

			failing, err := boot(standalone(server.URL, nil))
			Expect(err).ToNot(HaveOccurred())

			err = discover(context.Background(), failing)

			Expect(err).To(MatchError(oidc.ErrProvider))
			Expect(err).To(MatchError(ContainSubstring("502")))
			Expect(err.Error()).ToNot(ContainSubstring("gateway trouble"))
		})

		It("keeps the error of a discovery that answers a large HTML page short", func() {
			page := "<html>" + strings.Repeat("gateway trouble ", 1000) + "</html>"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(page))
			}))
			DeferCleanup(server.Close)

			failing, err := boot(standalone(server.URL, nil))
			Expect(err).ToNot(HaveOccurred())

			err = discover(context.Background(), failing)

			Expect(err).To(MatchError(oidc.ErrProvider))
			Expect(len(err.Error())).To(BeNumerically("<", 1024))
		})

		It("fails with ErrProvider and no deadline error when the token endpoint outlasts request_timeout", func() {
			release := make(chan struct{})
			server := discoveryServer(nil, func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-release:
				}
			})
			DeferCleanup(func() { close(release) })

			slow, err := boot(standalone(server.URL, map[string]any{"modules.oidc.request_timeout": "100ms"}))
			Expect(err).ToNot(HaveOccurred())

			err = discover(context.Background(), slow)

			Expect(err).To(MatchError(oidc.ErrProvider))
			Expect(err).ToNot(MatchError(context.DeadlineExceeded))
		})

		DescribeTable("signs in with ES256",
			func(signUserInfo bool) {
				ec := tkoidc.New(tkoidc.WithRedirectURIs(redirectURI), tkoidc.WithSigningAlgorithm(jose.ES256))
				Expect(ec.Start(context.Background(), do.New())).To(Succeed())
				DeferCleanup(ec.Stop, context.Background())

				c, err := boot(flat(ec.Settings()))
				Expect(err).ToNot(HaveOccurred())

				_, err = c.SignIn(context.Background(), oidc.SignInRequest{
					Code:         ec.Code(grant(func(g *tkoidc.Grant) { g.SignUserInfo = signUserInfo })),
					CodeVerifier: codeVerifier,
					Nonce:        "nonce-123",
				})

				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with a plain userinfo answer", false),
			Entry("with a signed userinfo answer", true),
		)

		It("verifies an ID token of a rotated key after it fetches the key set again", func() {
			_, err := signIn(grant(nil))
			Expect(err).ToNot(HaveOccurred())
			Expect(issuer.RotateKey()).To(Succeed())

			_, err = signIn(grant(nil))

			Expect(err).ToNot(HaveOccurred())
			Expect(issuer.Requests("/jwks")).To(Equal(2))
		})

		DescribeTable("refuses a code whose exchange differs from the authorization request",
			func(edit func(*tkoidc.Grant), verifier string) {
				_, err := client.SignIn(context.Background(), oidc.SignInRequest{
					Code: issuer.Code(grant(edit)), CodeVerifier: verifier, Nonce: "nonce-123",
				})

				Expect(err).To(MatchError(oidc.ErrCodeRefused))
				Expect(issuer.RedirectURI(user.Subject)).To(Equal(redirectURI))
			},
			Entry("another code verifier", nil, strings.Repeat("o", 43)),
			Entry("another registered redirect URI", func(g *tkoidc.Grant) { g.RedirectURI = otherRedirect }, codeVerifier),
		)

		It("refuses a code when the provider has not registered the redirect URI under public_url", func() {
			elsewhere := rebooted(map[string]any{"modules.oidc.public_url": "http://localhost:4000"})

			_, err := signInWith(elsewhere, grant(func(g *tkoidc.Grant) { g.RedirectURI = elsewhereURI }))

			Expect(err).To(MatchError(oidc.ErrCodeRefused))
			Expect(issuer.RedirectURI(user.Subject)).To(Equal(elsewhereURI))
		})

		DescribeTable("sends the redirect URI under the origin of public_url",
			func(raw, expected string) {
				_, err := signInWith(rebooted(map[string]any{"modules.oidc.public_url": raw}), grant(nil))
				Expect(err).To(HaveOccurred())

				Expect(issuer.RedirectURI(user.Subject)).To(Equal(expected))
			},
			Entry("without the default port of https", "https://localhost:443",
				"https://localhost/authentication/callback"),
			Entry("without the default port of http", "http://localhost:80",
				"http://localhost/authentication/callback"),
			Entry("with a lower-case host", "http://LOCALHOST:4000", elsewhereURI),
		)

		DescribeTable("takes the profile from a signed userinfo answer",
			func(audience func() any) {
				result, err := signIn(grant(func(g *tkoidc.Grant) {
					g.SignUserInfo = true
					g.UserInfoClaims = map[string]any{"aud": audience()}
					g.IDTokenClaims = map[string]any{"name": nil}
				}))

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Identity.Name).To(Equal("Alice"))
			},
			Entry("with the client as the audience", func() any { return issuer.ClientID() }),
			Entry("with the client among several audiences", func() any {
				return []string{"other-client", issuer.ClientID()}
			}),
		)

		DescribeTable("rejects a signed userinfo answer that fails a check",
			func(claims map[string]any) {
				_, err := signIn(grant(func(g *tkoidc.Grant) {
					g.SignUserInfo = true
					g.UserInfoClaims = claims
				}))

				Expect(err).To(MatchError(oidc.ErrTokensRejected))
			},
			Entry("another issuer", map[string]any{"iss": "https://other.example.test"}),
			Entry("no issuer", map[string]any{"iss": nil}),
			Entry("another audience", map[string]any{"aud": "other-client"}),
			Entry("no audience", map[string]any{"aud": nil}),
		)

		It("rejects a signed userinfo answer of an algorithm outside the discovery document", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) {
				g.SignUserInfo = true
				g.UserInfoAlgorithm = jose.RS384
			}))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		DescribeTable("checks a userinfo answer against the userinfo algorithms of the discovery document",
			func(listed string, signed jose.SignatureAlgorithm, matcher OmegaMatcher) {
				listing := tkoidc.New(tkoidc.WithRedirectURIs(redirectURI), tkoidc.WithListedUserInfoAlgorithms(listed))
				Expect(listing.Start(context.Background(), do.New())).To(Succeed())
				DeferCleanup(listing.Stop, context.Background())

				c, err := boot(flat(listing.Settings()))
				Expect(err).ToNot(HaveOccurred())

				_, err = c.SignIn(context.Background(), oidc.SignInRequest{
					Code: listing.Code(grant(func(g *tkoidc.Grant) {
						g.SignUserInfo = signed != ""
						g.UserInfoAlgorithm = signed
					})),
					CodeVerifier: codeVerifier,
					Nonce:        "nonce-123",
				})

				Expect(err).To(matcher)
			},
			Entry("accepts a listed algorithm the ID token does not use", "RS384", jose.RS384, Succeed()),
			Entry("rejects the algorithm of the ID token when another is listed", "RS384", jose.RS256,
				MatchError(oidc.ErrTokensRejected)),
			Entry("reads a plain answer when none is listed", "none", jose.SignatureAlgorithm(""), Succeed()),
			Entry("rejects a signed answer when none is listed", "none", jose.RS256,
				MatchError(oidc.ErrTokensRejected)),
			Entry("reads a plain answer when no supported algorithm is listed", "HS256", jose.SignatureAlgorithm(""),
				Succeed()),
			Entry("rejects a signed answer when no supported algorithm is listed", "HS256", jose.RS256,
				MatchError(oidc.ErrTokensRejected)),
		)

		It("rejects an ID token of an algorithm the discovery document does not list", func() {
			listing := tkoidc.New(tkoidc.WithRedirectURIs(redirectURI), tkoidc.WithListedAlgorithms("RS384"))
			Expect(listing.Start(context.Background(), do.New())).To(Succeed())
			DeferCleanup(listing.Stop, context.Background())

			c, err := boot(flat(listing.Settings()))
			Expect(err).ToNot(HaveOccurred())

			_, err = c.SignIn(context.Background(), oidc.SignInRequest{
				Code: listing.Code(grant(nil)), CodeVerifier: codeVerifier, Nonce: "nonce-123",
			})

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		It("rejects an ID token while the key set answers 503", func() {
			issuer.FailKeySet(http.StatusServiceUnavailable)
			DeferCleanup(issuer.FailKeySet, 0)

			_, err := signIn(grant(nil))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		It("rejects an ID token while the key set answers over 1 MiB", func() {
			issuer.PadKeySet(1 << 20)
			DeferCleanup(issuer.PadKeySet, 0)

			_, err := signIn(grant(nil))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		It("rejects a signed userinfo answer of a key outside the key set", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) {
				g.SignUserInfo = true
				g.UserInfoForeignKey = true
			}))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		DescribeTable("rejects a sign in without a nonce or a code verifier",
			func(nonce, verifier string, edit func(*tkoidc.Grant)) {
				_, err := client.SignIn(context.Background(), oidc.SignInRequest{
					Code: issuer.Code(grant(edit)), CodeVerifier: verifier, Nonce: nonce,
				})

				Expect(err).To(MatchError(oidc.ErrTokensRejected))
			},
			Entry("no nonce, for an ID token without a nonce", "", codeVerifier, func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"nonce": nil}
			}),
			Entry("no code verifier", "nonce-123", "", nil),
		)

		It("rejects an ID token of another nonce", func() {
			_, err := client.SignIn(context.Background(), oidc.SignInRequest{
				Code: issuer.Code(grant(nil)), CodeVerifier: codeVerifier, Nonce: "nonce-456",
			})

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		DescribeTable("rejects an ID token that fails a check",
			func(edit func(*tkoidc.Grant)) {
				_, err := signIn(grant(edit))

				Expect(err).To(MatchError(oidc.ErrTokensRejected))
			},
			Entry("another issuer", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"iss": "https://other.example.test"}
			}),
			Entry("another audience", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"aud": "other-client"}
			}),
			Entry("an expiry in the past", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}
			}),
			Entry("several audiences without an authorized party", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"aud": []string{issuer.ClientID(), "other-client"}}
			}),
			Entry("another authorized party", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"aud": []string{issuer.ClientID(), "other-client"}, "azp": "other-client"}
			}),
			Entry("another authorized party with a single audience", func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"azp": "other-client"}
			}),
			Entry("a signature of a key outside the key set", func(g *tkoidc.Grant) { g.IDTokenForeignKey = true }),
		)

		It("rejects a userinfo answer of another subject than the ID token", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) { g.UserInfoSubject = "mallory-1" }))

			Expect(err).To(MatchError(oidc.ErrTokensRejected))
		})

		DescribeTable("fails with ErrProvider for a profile claim of the wrong type",
			func(edit func(*tkoidc.Grant)) {
				_, err := signIn(grant(edit))

				Expect(err).To(MatchError(oidc.ErrProvider))
			},
			Entry("in the ID token", func(g *tkoidc.Grant) { g.IDTokenClaims = map[string]any{"name": 42} }),
			Entry("in userinfo", func(g *tkoidc.Grant) { g.UserInfoClaims = map[string]any{"name": 42} }),
		)

		It("accepts several audiences when the authorized party is the client", func() {
			_, err := signIn(grant(func(g *tkoidc.Grant) {
				g.IDTokenClaims = map[string]any{"aud": []string{issuer.ClientID(), issuer.URL()}, "azp": issuer.ClientID()}
			}))

			Expect(err).ToNot(HaveOccurred())
		})

		It("stops waiting for the discovery at the deadline of the call", func() {
			release := make(chan struct{})
			hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			DeferCleanup(hanging.Close)
			DeferCleanup(func() { close(release) })

			slow, err := boot(standalone(hanging.URL, map[string]any{"modules.oidc.request_timeout": "30s"}))
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			started := time.Now()
			_, err = slow.SignIn(ctx, oidc.SignInRequest{Code: "code", CodeVerifier: codeVerifier, Nonce: "nonce-123"})

			Expect(err).To(Equal(context.DeadlineExceeded))
			Expect(time.Since(started)).To(BeNumerically("<", 2*time.Second))
		})

		It("returns the error of the context when the deadline of the call passes during the code exchange", func() {
			release := make(chan struct{})
			server := discoveryServer(nil, func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-release:
				}
			})
			DeferCleanup(func() { close(release) })

			slow, err := boot(standalone(server.URL, map[string]any{"modules.oidc.request_timeout": "30s"}))
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err = slow.SignIn(ctx, oidc.SignInRequest{Code: "code", CodeVerifier: codeVerifier, Nonce: "nonce-123"})

			Expect(err).To(Equal(context.DeadlineExceeded))
		})

		DescribeTable("refuses an http endpoint outside localhost that discovery lists",
			func(endpoint string) {
				server := discoveryServer(func(doc map[string]any) {
					doc[endpoint] = "http://id.example.test/" + endpoint
				}, nil)

				insecure, err := boot(standalone(server.URL, nil))
				Expect(err).ToNot(HaveOccurred())

				err = discover(context.Background(), insecure)

				Expect(err).To(MatchError(oidc.ErrProvider))
				Expect(err).To(MatchError(ContainSubstring("must use https outside localhost")))
			},
			Entry("the token endpoint", "token_endpoint"),
			Entry("the userinfo endpoint", "userinfo_endpoint"),
			Entry("the key set", "jwks_uri"),
		)
	})

	Describe("discovery", func() {
		It("runs once and fetches the key set once for two sign ins", func() {
			for range 2 {
				_, err := signIn(grant(nil))
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(issuer.Requests("/.well-known/openid-configuration")).To(Equal(1))
			Expect(issuer.Requests("/jwks")).To(Equal(1))
		})

		It("runs a failed discovery again at the next call", func() {
			var (
				server      *httptest.Server
				discoveries atomic.Int32
			)
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/openid-configuration" {
					refuseGrant(w, r)

					return
				}

				if discoveries.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)

					return
				}

				_ = json.NewEncoder(w).Encode(discoveryDocument(server.URL))
			}))
			DeferCleanup(server.Close)

			flaky, err := boot(standalone(server.URL, nil))
			Expect(err).ToNot(HaveOccurred())

			Expect(discover(context.Background(), flaky)).To(MatchError(oidc.ErrProvider))
			Expect(discover(context.Background(), flaky)).To(MatchError(oidc.ErrCodeRefused))
			Expect(discoveries.Load()).To(BeEquivalentTo(2))
		})

		It("finishes for a joined call after the deadline of the first call passed", func() {
			var (
				server      *httptest.Server
				discoveries atomic.Int32
			)
			arrived, release := make(chan struct{}), make(chan struct{})
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/openid-configuration" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))

					return
				}

				if discoveries.Add(1) == 1 {
					close(arrived)
				}

				select {
				case <-r.Context().Done():
					return
				case <-release:
				}

				_ = json.NewEncoder(w).Encode(discoveryDocument(server.URL))
			}))
			DeferCleanup(server.Close)

			slow, err := boot(standalone(server.URL, nil))
			Expect(err).ToNot(HaveOccurred())

			short, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			first, joined := make(chan error, 1), make(chan error, 1)
			go func() { first <- discover(short, slow) }()
			<-arrived
			go func() { joined <- discover(context.Background(), slow) }()

			Expect(<-first).To(MatchError(context.DeadlineExceeded))
			close(release)

			// The token endpoint refuses the code only after the discovery passed.
			Expect(<-joined).To(MatchError(oidc.ErrCodeRefused))
			Expect(discoveries.Load()).To(BeEquivalentTo(1))
		})

		DescribeTable("fails with ErrProvider for a discovery document the client cannot use",
			func(edit func(doc map[string]any)) {
				broken, err := boot(standalone(discoveryServer(edit, nil).URL, nil))
				Expect(err).ToNot(HaveOccurred())

				Expect(discover(context.Background(), broken)).To(MatchError(oidc.ErrProvider))
			},
			Entry("another issuer", func(doc map[string]any) { doc["issuer"] = "https://other.example.test" }),
			Entry("no token endpoint", func(doc map[string]any) { delete(doc, "token_endpoint") }),
			Entry("no userinfo endpoint", func(doc map[string]any) { delete(doc, "userinfo_endpoint") }),
			Entry("no key set", func(doc map[string]any) { delete(doc, "jwks_uri") }),
			Entry("no supported signing algorithm", func(doc map[string]any) {
				doc["id_token_signing_alg_values_supported"] = []string{"HS256"}
			}),
			Entry("PKCE methods without S256", func(doc map[string]any) {
				doc["code_challenge_methods_supported"] = []string{"plain"}
			}),
		)

		DescribeTable("accepts a discovery document the client can use",
			func(edit func(doc map[string]any)) {
				usable, err := boot(standalone(discoveryServer(edit, nil).URL, nil))
				Expect(err).ToNot(HaveOccurred())

				Expect(discover(context.Background(), usable)).To(MatchError(oidc.ErrCodeRefused))
			},
			Entry("no PKCE methods", nil),
			Entry("PKCE methods with S256", func(doc map[string]any) {
				doc["code_challenge_methods_supported"] = []string{"plain", "S256"}
			}),
			Entry("a supported signing algorithm among others", func(doc map[string]any) {
				doc["id_token_signing_alg_values_supported"] = []string{"HS256", "RS256"}
			}),
			Entry("no supported userinfo signing algorithm", func(doc map[string]any) {
				doc["userinfo_signing_alg_values_supported"] = []string{"HS256"}
			}),
		)
	})

	Describe("redirects", func() {
		var (
			server     *httptest.Server
			redirected atomic.Int32
		)

		BeforeEach(func() {
			redirected.Store(0)
			server = discoveryServer(nil, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/elsewhere" {
					redirected.Add(1)
					return
				}

				http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			})
		})

		It("sends the client credentials to no redirect target of the token endpoint", func() {
			redirecting, err := boot(standalone(server.URL, nil))
			Expect(err).ToNot(HaveOccurred())

			_, err = redirecting.SignIn(context.Background(), oidc.SignInRequest{
				Code: "code", CodeVerifier: codeVerifier, Nonce: "nonce-123",
			})

			Expect(err).To(MatchError(oidc.ErrProvider))
			Expect(redirected.Load()).To(BeZero())
		})
	})
})

var _ = Describe("Module", func() {
	DescribeTable("accepts an issuer that is https or stays on the machine",
		func(issuer string) {
			_, err := boot(standalone(issuer, nil))

			Expect(err).ToNot(HaveOccurred())
		},
		Entry("https", "https://id.example.test"),
		Entry("localhost", "http://localhost:1411"),
		Entry("an IPv4 loopback address", "http://127.0.0.1:1411"),
		Entry("the IPv6 loopback address", "http://[::1]:1411"),
	)

	DescribeTable("refuses an http issuer elsewhere",
		func(issuer string) {
			_, err := boot(standalone(issuer, nil))

			Expect(err).To(MatchError(ContainSubstring("must use https outside localhost")))
		},
		Entry("another host", "http://id.example.test"),
		Entry("a subdomain of localhost", "http://id.localhost:1411"),
	)

	It("accepts a public_url with a trailing slash", func() {
		_, err := boot(standalone("https://id.example.test",
			map[string]any{"modules.oidc.public_url": publicURL + "/"}))

		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable("refuses a public_url that is no origin",
		func(raw string) {
			_, err := boot(standalone("https://id.example.test",
				map[string]any{"modules.oidc.public_url": raw}))

			Expect(err).To(MatchError(ContainSubstring("must be a plain origin")))
		},
		Entry("a path", "http://localhost:3000/app"),
		Entry("a query", "http://localhost:3000/?tab=map"),
		Entry("a fragment", "http://localhost:3000/#map"),
		Entry("userinfo", "http://user@localhost:3000"),
	)

	DescribeTable("refuses an issuer that OpenID Connect Discovery forbids",
		func(issuer string) {
			_, err := boot(standalone(issuer, nil))

			Expect(err).To(MatchError(ContainSubstring("must carry no userinfo, query or fragment")))
		},
		Entry("a query", "https://id.example.test/?tenant=a"),
		Entry("a fragment", "https://id.example.test/#a"),
		Entry("userinfo", "https://user@id.example.test"),
	)

	It("refuses an http public_url elsewhere", func() {
		_, err := boot(standalone("https://id.example.test",
			map[string]any{"modules.oidc.public_url": "http://app.example.test"}))

		Expect(err).To(MatchError(ContainSubstring("must use https outside localhost")))
	})

	It("accepts an http issuer and public_url elsewhere under the explicit opt-in", func() {
		_, err := boot(standalone("http://id.example.test", map[string]any{
			"modules.oidc.public_url":                      "http://app.example.test",
			"modules.oidc.dangerously_allow_insecure_http": true,
		}))

		Expect(err).ToNot(HaveOccurred())
	})
})
