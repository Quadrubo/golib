package oidc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/do/v2"
	"golang.org/x/oauth2"

	tkoidc "github.com/quadrubo/golib/testkit/oidc"
)

func TestOIDC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testkit OIDC Suite")
}

var _ = Describe("Issuer", func() {
	var (
		issuer *tkoidc.Issuer
		secret string
	)

	const redirectURI = "http://localhost:3000/authentication/callback"

	BeforeEach(func() {
		issuer = tkoidc.New(tkoidc.WithRedirectURIs(redirectURI))
		Expect(issuer.Start(context.Background(), do.New())).To(Succeed())
		DeferCleanup(issuer.Stop, context.Background())

		secret = issuer.Settings()["modules"].(map[string]any)["oidc"].(map[string]any)["client_secret"].(string)
	})

	post := func(ctx context.Context, id, secret string, form url.Values) string {
		GinkgoHelper()

		form.Set("grant_type", "authorization_code")

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer.URL()+"/token",
			strings.NewReader(form.Encode()))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(id, secret)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		var body struct {
			Error string `json:"error"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

		return body.Error
	}

	exchangeWith := func(ctx context.Context, id, secret, verifier string) string {
		GinkgoHelper()

		return post(ctx, id, secret, url.Values{"code": {"unknown"}, "code_verifier": {verifier}})
	}

	exchange := func(ctx context.Context, id, secret string) string {
		GinkgoHelper()

		return exchangeWith(ctx, id, secret, strings.Repeat("v", 43))
	}

	DescribeTable("answers a code verifier by its length",
		func(ctx SpecContext, length int, answer string) {
			Expect(exchangeWith(ctx, url.QueryEscape(issuer.ClientID()), url.QueryEscape(secret),
				strings.Repeat("v", length))).To(Equal(answer))
		},
		Entry("42 characters are invalid_request", 42, "invalid_request"),
		Entry("43 characters reach the code", 43, "invalid_grant"),
		Entry("128 characters reach the code", 128, "invalid_grant"),
		Entry("129 characters are invalid_request", 129, "invalid_request"),
	)

	It("carries a client secret with characters that the form encoding escapes", func() {
		Expect(url.QueryEscape(secret)).ToNot(Equal(secret))
	})

	It("authenticates a client that form-encodes its credentials", func(ctx SpecContext) {
		Expect(exchange(ctx, url.QueryEscape(issuer.ClientID()), url.QueryEscape(secret))).To(Equal("invalid_grant"))
	})

	It("refuses a client that does not form-encode its client secret", func(ctx SpecContext) {
		Expect(exchange(ctx, issuer.ClientID(), secret)).To(Equal("invalid_client"))
	})

	It("exchanges a code once", func(ctx SpecContext) {
		verifier := strings.Repeat("v", 43)
		code := issuer.Code(tkoidc.Grant{
			User:          tkoidc.User{Subject: "alice-1"},
			Nonce:         "nonce-123",
			CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier),
			RedirectURI:   redirectURI,
		})
		form := func() url.Values {
			return url.Values{"code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirectURI}}
		}

		Expect(post(ctx, url.QueryEscape(issuer.ClientID()), url.QueryEscape(secret), form())).To(BeEmpty())
		Expect(post(ctx, url.QueryEscape(issuer.ClientID()), url.QueryEscape(secret), form())).
			To(Equal("invalid_grant"))
	})
})

var _ = Describe("New", func() {
	DescribeTable("panics for a signing algorithm without a matching key",
		func(algorithm jose.SignatureAlgorithm) {
			Expect(func() { tkoidc.New(tkoidc.WithSigningAlgorithm(algorithm)) }).
				To(PanicWith(ContainSubstring("unsupported signing algorithm")))
		},
		Entry("ES384", jose.ES384),
		Entry("ES512", jose.ES512),
		Entry("EdDSA", jose.EdDSA),
	)
})
