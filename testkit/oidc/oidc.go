package oidc

import (
	"cmp"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/samber/do/v2"
	"golang.org/x/oauth2"

	"github.com/quadrubo/golib/testkit"
)

const (
	clientID     = "testkit-client"
	clientSecret = "s3cr+t/%:&"
)

type User struct {
	Subject           string
	Email             string
	Name              string
	PreferredUsername string
}

type Grant struct {
	User          User
	Nonce         string
	CodeChallenge string
	RedirectURI   string

	// A nil value removes the claim.
	IDTokenClaims  map[string]any
	UserInfoClaims map[string]any

	OmitIDToken       bool
	IDTokenForeignKey bool
	UserInfoSubject   string
	UserInfoStatus    int
	UserInfoBody      string

	SignUserInfo       bool
	UserInfoForeignKey bool
	UserInfoAlgorithm  jose.SignatureAlgorithm
}

type signingKey struct {
	id  string
	key crypto.Signer
}

type Issuer struct {
	opts    options
	server  *httptest.Server
	foreign signingKey

	mu            sync.Mutex
	keys          []signingKey
	keySetStatus  int
	keySetPadding int
	codes         map[string]*Grant
	access        map[string]*Grant
	failing       map[string]bool
	redirectURIs  map[string]string
	requests      map[string]int
}

var (
	_ testkit.Dependency       = (*Issuer)(nil)
	_ testkit.SettingsProvider = (*Issuer)(nil)
)

var rsaAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.PS256, jose.PS384, jose.PS512,
}

func New(opts ...Option) *Issuer {
	cfg := options{algorithm: jose.RS256}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.algorithm != jose.ES256 && !slices.Contains(rsaAlgorithms, cfg.algorithm) {
		panic(fmt.Sprintf("testkit/oidc: unsupported signing algorithm %q", cfg.algorithm))
	}

	if cfg.listed == nil {
		cfg.listed = []string{string(cfg.algorithm)}
	}

	return &Issuer{
		opts:         cfg,
		codes:        map[string]*Grant{},
		access:       map[string]*Grant{},
		failing:      map[string]bool{},
		redirectURIs: map[string]string{},
		requests:     map[string]int{},
	}
}

func (i *Issuer) Start(_ context.Context, injector do.Injector) error {
	key, err := i.newSigningKey("key-0")
	if err != nil {
		return err
	}
	i.keys = []signingKey{key}

	if i.foreign, err = i.newSigningKey("foreign"); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", i.discovery)
	mux.HandleFunc("GET /jwks", i.jwks)
	mux.HandleFunc("POST /token", i.token)
	mux.HandleFunc("GET /userinfo", i.userinfo)
	i.server = httptest.NewServer(i.count(mux))

	do.ProvideValue(injector, i)

	return nil
}

func (i *Issuer) newSigningKey(id string) (signingKey, error) {
	var (
		key crypto.Signer
		err error
	)

	if i.opts.algorithm == jose.ES256 {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	}

	if err != nil {
		return signingKey{}, fmt.Errorf("testkit/oidc: failed to generate the key %q: %w", id, err)
	}

	return signingKey{id: id, key: key}, nil
}

func (i *Issuer) count(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.mu.Lock()
		i.requests[r.URL.Path]++
		i.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

// Requests returns the number of requests the issuer received on the path,
// such as "/jwks".
func (i *Issuer) Requests(path string) int {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.requests[path]
}

func (i *Issuer) Stop(context.Context) error {
	if i.server != nil {
		i.server.Close()
	}

	return nil
}

func (i *Issuer) Settings() map[string]any {
	return map[string]any{
		"modules": map[string]any{
			"oidc": map[string]any{
				"issuer":        i.URL(),
				"client_id":     clientID,
				"client_secret": clientSecret,
			},
		},
	}
}

func (i *Issuer) URL() string { return i.server.URL }

func (i *Issuer) ClientID() string { return clientID }

func (i *Issuer) Code(grant Grant) string {
	i.mu.Lock()
	defer i.mu.Unlock()

	code := rand.Text()
	i.codes[code] = &grant

	return code
}

// FailExchange answers the code exchanges of the subject with status 503 and
// temporarily_unavailable.
func (i *Issuer) FailExchange(subject string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.failing[subject] = true
}

// RotateKey signs every later token with a new key, which the JWKS serves
// beside the previous ones.
func (i *Issuer) RotateKey() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	key, err := i.newSigningKey("key-" + strconv.Itoa(len(i.keys)))
	if err != nil {
		return err
	}

	i.keys = append(i.keys, key)

	return nil
}

// FailKeySet answers the JWKS with the status. Status 0 serves the keys again.
func (i *Issuer) FailKeySet(status int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.keySetStatus = status
}

// PadKeySet adds a member of the size in bytes to the JWKS. Size 0 removes it.
func (i *Issuer) PadKeySet(size int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.keySetPadding = size
}

// RedirectURI returns the redirect_uri of the last code exchange of the
// subject.
func (i *Issuer) RedirectURI(subject string) string {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.redirectURIs[subject]
}

func (i *Issuer) discovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                                i.URL(),
		"authorization_endpoint":                i.URL() + "/authorize",
		"token_endpoint":                        i.URL() + "/token",
		"userinfo_endpoint":                     i.URL() + "/userinfo",
		"jwks_uri":                              i.URL() + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": i.opts.listed,
		"code_challenge_methods_supported":      []string{"S256"},
	}

	if i.opts.userInfoListed != nil {
		doc["userinfo_signing_alg_values_supported"] = i.opts.userInfoListed
	}

	writeJSON(w, http.StatusOK, doc)
}

func (i *Issuer) jwks(w http.ResponseWriter, _ *http.Request) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.keySetStatus != 0 {
		w.WriteHeader(i.keySetStatus)

		return
	}

	var set struct {
		Keys    []jose.JSONWebKey `json:"keys"`
		Padding string            `json:"padding,omitempty"`
	}

	for _, key := range i.keys {
		set.Keys = append(set.Keys, jose.JSONWebKey{
			Key: key.key.Public(), KeyID: key.id, Use: "sig",
		})
	}

	set.Padding = strings.Repeat("a", i.keySetPadding)

	writeJSON(w, http.StatusOK, set)
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	if !authenticated(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	if r.PostForm.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}

	// RFC 7636 section 4.1 bounds the code verifier at 43 to 128 characters.
	if verifier := r.PostForm.Get("code_verifier"); len(verifier) < 43 || len(verifier) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	i.exchange(w, r.PostForm)
}

// authenticated decodes both credentials as RFC 6749 section 2.3.1 requires.
func authenticated(r *http.Request) bool {
	encodedID, encodedSecret, ok := r.BasicAuth()
	if !ok {
		return false
	}

	id, idErr := url.QueryUnescape(encodedID)
	secret, secretErr := url.QueryUnescape(encodedSecret)

	return idErr == nil && secretErr == nil && id == clientID && secret == clientSecret
}

func (i *Issuer) exchange(w http.ResponseWriter, form url.Values) {
	i.mu.Lock()
	defer i.mu.Unlock()

	grant, ok := i.codes[form.Get("code")]
	delete(i.codes, form.Get("code"))

	if ok {
		i.redirectURIs[grant.User.Subject] = form.Get("redirect_uri")

		if i.failing[grant.User.Subject] {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
			return
		}
	}

	if !ok || oauth2.S256ChallengeFromVerifier(form.Get("code_verifier")) != grant.CodeChallenge ||
		!slices.Contains(i.opts.redirectURIs, grant.RedirectURI) || form.Get("redirect_uri") != grant.RedirectURI {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}

	claims := i.claims(grant)
	claims["nonce"] = grant.Nonce
	override(claims, grant.IDTokenClaims)

	key := i.keys[len(i.keys)-1]
	if grant.IDTokenForeignKey {
		key = i.foreign
	}

	idToken, err := sign(key, i.opts.algorithm, claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}

	access := rand.Text()
	i.access[access] = grant

	response := map[string]any{
		"access_token": access,
		"token_type":   "Bearer",
		"id_token":     idToken,
		"expires_in":   int(time.Hour.Seconds()),
	}
	if grant.OmitIDToken {
		delete(response, "id_token")
	}

	writeJSON(w, http.StatusOK, response)
}

func override(claims, overrides map[string]any) {
	for name, value := range overrides {
		if value == nil {
			delete(claims, name)
			continue
		}
		claims[name] = value
	}
}

func (i *Issuer) claims(grant *Grant) map[string]any {
	now := time.Now()

	return map[string]any{
		"iss":                i.URL(),
		"sub":                grant.User.Subject,
		"aud":                clientID,
		"iat":                now.Unix(),
		"exp":                now.Add(time.Hour).Unix(),
		"email":              grant.User.Email,
		"name":               grant.User.Name,
		"preferred_username": grant.User.PreferredUsername,
	}
}

func (i *Issuer) userinfo(w http.ResponseWriter, r *http.Request) {
	i.mu.Lock()
	defer i.mu.Unlock()

	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	grant, known := i.access[token]

	if !ok || !known {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}

	if grant.UserInfoBody != "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(grant.UserInfoBody))

		return
	}

	subject := grant.User.Subject
	if grant.UserInfoSubject != "" {
		subject = grant.UserInfoSubject
	}

	claims := map[string]any{
		"sub":                subject,
		"email":              grant.User.Email,
		"name":               grant.User.Name,
		"preferred_username": grant.User.PreferredUsername,
	}

	if !grant.SignUserInfo {
		override(claims, grant.UserInfoClaims)
		writeJSON(w, cmp.Or(grant.UserInfoStatus, http.StatusOK), claims)

		return
	}

	claims["iss"] = i.URL()
	claims["aud"] = clientID
	override(claims, grant.UserInfoClaims)

	key := i.keys[len(i.keys)-1]
	if grant.UserInfoForeignKey {
		key = i.foreign
	}

	algorithm := grant.UserInfoAlgorithm
	if algorithm == "" {
		algorithm = i.opts.algorithm
	}

	signed, err := sign(key, algorithm, claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}

	w.Header().Set("Content-Type", "application/jwt")
	_, _ = w.Write([]byte(signed))
}

func sign(key signingKey, algorithm jose.SignatureAlgorithm, claims map[string]any) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: algorithm, Key: key.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", key.id),
	)
	if err != nil {
		return "", err
	}

	return jwt.Signed(signer).Claims(maps.Clone(claims)).Serialize()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
