package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
)

func TestAuthenticatorMapsVerifiedOIDCClaims(t *testing.T) {
	provider := newTestProvider(t)
	authenticator := provider.authenticator(t)
	token := provider.token(t, tokenClaims{
		Subject: "game-backend", Tenant: "tenant-a",
		Scope: "match_tickets.write assignments.read unknown.scope",
	})
	request := httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/assignments", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	principal, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "game-backend" || principal.Tenant != "tenant-a" ||
		!principal.Allows(targetapi.PermissionMatchTicketsWrite) ||
		!principal.Allows(targetapi.PermissionAssignmentsRead) ||
		principal.Allows(targetapi.PermissionPoliciesWrite) || len(principal.Permissions) != 2 {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestAuthenticatorRejectsInvalidCredentials(t *testing.T) {
	provider := newTestProvider(t)
	authenticator := provider.authenticator(t)
	valid := tokenClaims{Subject: "game-backend", Tenant: "tenant-a", Scope: "match_tickets.read"}
	tests := map[string]func(*http.Request){
		"missing header": func(*http.Request) {},
		"multiple headers": func(request *http.Request) {
			request.Header.Add("Authorization", "Bearer "+provider.token(t, valid))
			request.Header.Add("Authorization", "Bearer "+provider.token(t, valid))
		},
		"wrong scheme": func(request *http.Request) {
			request.Header.Set("Authorization", "Basic "+provider.token(t, valid))
		},
		"wrong audience": func(request *http.Request) {
			claims := valid
			claims.Audience = "another-service"
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
		"wrong issuer": func(request *http.Request) {
			claims := valid
			claims.Issuer = "https://another-issuer.example"
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
		"expired": func(request *http.Request) {
			claims := valid
			claims.Expiry = time.Now().Add(-time.Minute)
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
		"not yet valid": func(request *http.Request) {
			claims := valid
			claims.NotBefore = time.Now().Add(10 * time.Minute)
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
		"missing tenant": func(request *http.Request) {
			claims := valid
			claims.Tenant = ""
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
		"non-string scope": func(request *http.Request) {
			claims := valid
			claims.RawScope = []string{"match_tickets.read"}
			request.Header.Set("Authorization", "Bearer "+provider.token(t, claims))
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
			prepare(request)
			if _, err := authenticator.Authenticate(request); !errors.Is(err, targetapi.ErrUnauthenticated) {
				t.Fatalf("Authenticate error = %v; want unauthenticated", err)
			}
		})
	}
}

func TestAuthenticatorRefreshesRotatedJWKS(t *testing.T) {
	provider := newTestProvider(t)
	authenticator := provider.authenticator(t)
	first := provider.token(t, tokenClaims{Subject: "first", Tenant: "tenant-a"})
	request := httptest.NewRequest(http.MethodGet, "https://sema.example", nil)
	request.Header.Set("Authorization", "Bearer "+first)
	if _, err := authenticator.Authenticate(request); err != nil {
		t.Fatal(err)
	}
	provider.rotate(t)
	second := provider.token(t, tokenClaims{Subject: "second", Tenant: "tenant-a"})
	request = httptest.NewRequest(http.MethodGet, "https://sema.example", nil)
	request.Header.Set("Authorization", "Bearer "+second)
	principal, err := authenticator.Authenticate(request)
	if err != nil || principal.Subject != "second" {
		t.Fatalf("rotated principal = %#v, error = %v", principal, err)
	}
	if provider.jwksRequests() < 2 {
		t.Fatalf("JWKS requests = %d; want refresh after key rotation", provider.jwksRequests())
	}
}

func TestAuthenticatorReportsProviderUnavailableDuringKeyRefresh(t *testing.T) {
	provider := newTestProvider(t)
	authenticator := provider.authenticator(t)
	request := httptest.NewRequest(http.MethodGet, "https://sema.example", nil)
	request.Header.Set("Authorization", "Bearer "+provider.token(t, tokenClaims{Subject: "cached", Tenant: "tenant-a"}))
	if _, err := authenticator.Authenticate(request); err != nil {
		t.Fatal(err)
	}
	provider.rotate(t)
	provider.setUnavailable(true)
	request = httptest.NewRequest(http.MethodGet, "https://sema.example", nil)
	request.Header.Set("Authorization", "Bearer "+provider.token(t, tokenClaims{Subject: "rotated", Tenant: "tenant-a"}))
	if _, err := authenticator.Authenticate(request); err == nil || errors.Is(err, targetapi.ErrUnauthenticated) {
		t.Fatalf("Authenticate error = %v; want provider unavailable", err)
	}
}

func TestTargetAPIFailClosedOIDCBoundary(t *testing.T) {
	provider := newTestProvider(t)
	authenticator := provider.authenticator(t)
	handler, err := targetapi.New(repository.NewMemory(), authenticator, targetapi.Options{
		CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
	request.Header.Set("Authorization", "Bearer "+provider.token(t, tokenClaims{
		Subject: "reader", Tenant: "tenant-a", Scope: "match_tickets.read",
	}))
	if recorder := serve(handler, request); recorder.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", recorder.Code, recorder.Body)
	}

	request = httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
	if recorder := serve(handler, request); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, body = %s", recorder.Code, recorder.Body)
	}

	request = httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
	request.Header.Set("Authorization", "Bearer "+provider.token(t, tokenClaims{
		Subject: "no-permission", Tenant: "tenant-a",
	}))
	if recorder := serve(handler, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("permission denied status = %d, body = %s", recorder.Code, recorder.Body)
	}

	provider.rotate(t)
	provider.setUnavailable(true)
	request = httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
	request.Header.Set("Authorization", "Bearer "+provider.token(t, tokenClaims{
		Subject: "new-key", Tenant: "tenant-a", Scope: "match_tickets.read",
	}))
	if recorder := serve(handler, request); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("provider unavailable status = %d, body = %s", recorder.Code, recorder.Body)
	}
}

func serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	tests := map[string]Config{
		"HTTP issuer":    {Issuer: "http://issuer.example", Audience: "sema"},
		"empty audience": {Issuer: "https://issuer.example", Audience: ""},
		"bad claim":      {Issuer: "https://issuer.example", Audience: "sema", TenantClaim: "tenant claim"},
		"symmetric algorithm": {
			Issuer: "https://issuer.example", Audience: "sema", SigningAlgorithms: []string{"HS256"},
		},
	}
	for name, configuration := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(context.Background(), configuration); err == nil {
				t.Fatal("unsafe OIDC configuration was accepted")
			}
		})
	}
}

type testProvider struct {
	testing *testing.T
	server  *httptest.Server

	mu          sync.Mutex
	key         *rsa.PrivateKey
	kid         string
	requests    int
	unavailable bool
}

type tokenClaims struct {
	Subject   string
	Tenant    string
	Scope     string
	RawScope  any
	Issuer    string
	Audience  string
	Expiry    time.Time
	NotBefore time.Time
}

func newTestProvider(t *testing.T) *testProvider {
	t.Helper()
	provider := &testProvider{testing: t}
	provider.rotate(t)
	provider.server = httptest.NewTLSServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (provider *testProvider) authenticator(t *testing.T) *Authenticator {
	t.Helper()
	client := provider.server.Client()
	client.Timeout = time.Second
	authenticator, err := New(context.Background(), Config{
		Issuer: provider.server.URL, Audience: "sema", HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

func (provider *testProvider) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"issuer": provider.server.URL, "jwks_uri": provider.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/jwks":
		provider.requests++
		if provider.unavailable {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []any{publicJWK(&provider.key.PublicKey, provider.kid)}})
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (provider *testProvider) rotate(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.key = key
	provider.kid = fmt.Sprintf("key-%d", time.Now().UnixNano())
}

func (provider *testProvider) setUnavailable(unavailable bool) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.unavailable = unavailable
}

func (provider *testProvider) jwksRequests() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.requests
}

func (provider *testProvider) token(t *testing.T, requested tokenClaims) string {
	t.Helper()
	provider.mu.Lock()
	key, kid := provider.key, provider.kid
	provider.mu.Unlock()
	audience := requested.Audience
	if audience == "" {
		audience = "sema"
	}
	expiry := requested.Expiry
	if expiry.IsZero() {
		expiry = time.Now().Add(time.Minute)
	}
	issuer := requested.Issuer
	if issuer == "" {
		issuer = provider.server.URL
	}
	claims := map[string]any{
		"iss": issuer, "aud": audience, "sub": requested.Subject,
		"iat": time.Now().Add(-time.Second).Unix(), "exp": expiry.Unix(),
	}
	if !requested.NotBefore.IsZero() {
		claims["nbf"] = requested.NotBefore.Unix()
	}
	if requested.Tenant != "" {
		claims[DefaultTenantClaim] = requested.Tenant
	}
	if requested.RawScope != nil {
		claims["scope"] = requested.RawScope
	} else if requested.Scope != "" {
		claims["scope"] = requested.Scope
	}
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(encoded))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func publicJWK(key *rsa.PublicKey, kid string) map[string]any {
	return map[string]any{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}
