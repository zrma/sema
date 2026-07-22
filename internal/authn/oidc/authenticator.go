// Package oidc provides a provider-neutral OpenID Connect authenticator for
// the target API. Token acquisition and identity-provider configuration remain
// deployment responsibilities.
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/zrma/sema/internal/targetapi"
)

const (
	DefaultTenantClaim  = "sema_tenant"
	maxBearerTokenBytes = 16 << 10
	defaultHTTPTimeout  = 5 * time.Second
)

var claimNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,63}$`)

// Config contains only provider-neutral token validation inputs. Client
// credentials are intentionally absent because Sema validates tokens but does
// not acquire them on behalf of callers.
type Config struct {
	Issuer            string
	Audience          string
	TenantClaim       string
	SigningAlgorithms []string
	HTTPClient        *http.Client
	Now               func() time.Time
}

// Authenticator verifies signed access tokens and maps their claims to the
// provider-opaque target API principal.
type Authenticator struct {
	verifier    tokenVerifier
	tenantClaim string
}

type tokenVerifier interface {
	Verify(context.Context, string) (*coreoidc.IDToken, error)
}

type providerMetadata struct {
	JWKSURL string `json:"jwks_uri"`
}

type verificationState struct {
	providerUnavailable bool
}

type verificationStateKey struct{}

// New performs strict OIDC discovery and constructs a verifier with bounded
// provider I/O. Issuer and JWKS endpoints must use HTTPS.
func New(ctx context.Context, config Config) (*Authenticator, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if config.TenantClaim == "" {
		config.TenantClaim = DefaultTenantClaim
	}
	if len(config.SigningAlgorithms) == 0 {
		config.SigningAlgorithms = []string{"RS256"}
	}

	client := providerHTTPClient(config.HTTPClient)
	discoveryContext := coreoidc.ClientContext(ctx, client)
	provider, err := coreoidc.NewProvider(discoveryContext, config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	var metadata providerMetadata
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("decode OIDC provider metadata: %w", err)
	}
	if err := validateHTTPSURL("OIDC JWKS URL", metadata.JWKSURL); err != nil {
		return nil, err
	}

	keyContext := coreoidc.ClientContext(context.Background(), client)
	remoteKeys := coreoidc.NewRemoteKeySet(keyContext, metadata.JWKSURL)
	keys := availabilityKeySet{delegate: remoteKeys}
	verifier := coreoidc.NewVerifier(config.Issuer, keys, &coreoidc.Config{
		ClientID:             config.Audience,
		SupportedSigningAlgs: append([]string(nil), config.SigningAlgorithms...),
		Now:                  config.Now,
	})
	return &Authenticator{verifier: verifier, tenantClaim: config.TenantClaim}, nil
}

func validateConfig(config Config) error {
	if err := validateHTTPSURL("OIDC issuer", config.Issuer); err != nil {
		return err
	}
	if strings.TrimSpace(config.Audience) == "" || config.Audience != strings.TrimSpace(config.Audience) {
		return fmt.Errorf("OIDC audience is required without surrounding whitespace")
	}
	tenantClaim := config.TenantClaim
	if tenantClaim == "" {
		tenantClaim = DefaultTenantClaim
	}
	if !claimNamePattern.MatchString(tenantClaim) {
		return fmt.Errorf("OIDC tenant claim name is invalid")
	}
	for _, algorithm := range config.SigningAlgorithms {
		if algorithm != "RS256" && algorithm != "RS384" && algorithm != "RS512" &&
			algorithm != "PS256" && algorithm != "PS384" && algorithm != "PS512" &&
			algorithm != "ES256" && algorithm != "ES384" && algorithm != "ES512" &&
			algorithm != "EdDSA" {
			return fmt.Errorf("OIDC signing algorithm %q is not an allowed asymmetric algorithm", algorithm)
		}
	}
	return nil
}

func validateHTTPSURL(label, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials, query, or fragment", label)
	}
	return nil
}

func providerHTTPClient(configured *http.Client) *http.Client {
	client := http.Client{Timeout: defaultHTTPTimeout}
	if configured != nil {
		client = *configured
		if client.Timeout <= 0 {
			client.Timeout = defaultHTTPTimeout
		}
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = availabilityTransport{delegate: transport}
	return &client
}

// Authenticate accepts exactly one Authorization header and no alternative
// credential location. Unknown scopes are ignored; only the fixed Sema
// permission vocabulary can enter the principal.
func (authenticator *Authenticator) Authenticate(request *http.Request) (targetapi.Principal, error) {
	if authenticator == nil || authenticator.verifier == nil || request == nil {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}
	rawToken, err := bearerToken(request.Header.Values("Authorization"))
	if err != nil {
		return targetapi.Principal{}, err
	}
	state := &verificationState{}
	verificationContext := context.WithValue(request.Context(), verificationStateKey{}, state)
	token, err := authenticator.verifier.Verify(verificationContext, rawToken)
	if err != nil {
		if state.providerUnavailable {
			return targetapi.Principal{}, fmt.Errorf("OIDC key provider unavailable")
		}
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}
	principal, err := authenticator.principal(token)
	if err != nil {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}
	return principal, nil
}

func bearerToken(values []string) (string, error) {
	if len(values) != 1 {
		return "", targetapi.ErrUnauthenticated
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" ||
		len(parts[1]) > maxBearerTokenBytes {
		return "", targetapi.ErrUnauthenticated
	}
	return parts[1], nil
}

func (authenticator *Authenticator) principal(token *coreoidc.IDToken) (targetapi.Principal, error) {
	if token == nil || token.Subject == "" {
		return targetapi.Principal{}, fmt.Errorf("OIDC subject is required")
	}
	claims := make(map[string]json.RawMessage)
	if err := token.Claims(&claims); err != nil {
		return targetapi.Principal{}, fmt.Errorf("decode OIDC claims: %w", err)
	}
	var tenant string
	if raw, exists := claims[authenticator.tenantClaim]; !exists || json.Unmarshal(raw, &tenant) != nil || tenant == "" {
		return targetapi.Principal{}, fmt.Errorf("OIDC tenant claim is required")
	}
	permissions, err := permissionsFromScope(claims["scope"])
	if err != nil {
		return targetapi.Principal{}, err
	}
	return targetapi.Principal{
		Subject: token.Subject, Tenant: tenant, Permissions: permissions,
	}, nil
}

func permissionsFromScope(raw json.RawMessage) (map[targetapi.Permission]bool, error) {
	permissions := make(map[targetapi.Permission]bool)
	if len(raw) == 0 {
		return permissions, nil
	}
	var scope string
	if err := json.Unmarshal(raw, &scope); err != nil {
		return nil, fmt.Errorf("OIDC scope claim must be a string")
	}
	for _, value := range strings.Fields(scope) {
		if permission, exists := permissionScopes[value]; exists {
			permissions[permission] = true
		}
	}
	return permissions, nil
}

var permissionScopes = map[string]targetapi.Permission{
	string(targetapi.PermissionMatchTicketsRead):     targetapi.PermissionMatchTicketsRead,
	string(targetapi.PermissionMatchTicketsWrite):    targetapi.PermissionMatchTicketsWrite,
	string(targetapi.PermissionBackfillTicketsRead):  targetapi.PermissionBackfillTicketsRead,
	string(targetapi.PermissionBackfillTicketsWrite): targetapi.PermissionBackfillTicketsWrite,
	string(targetapi.PermissionPoliciesRead):         targetapi.PermissionPoliciesRead,
	string(targetapi.PermissionPoliciesWrite):        targetapi.PermissionPoliciesWrite,
	string(targetapi.PermissionPlanningRunsRead):     targetapi.PermissionPlanningRunsRead,
	string(targetapi.PermissionPlanningRunsWrite):    targetapi.PermissionPlanningRunsWrite,
	string(targetapi.PermissionReservationsRead):     targetapi.PermissionReservationsRead,
	string(targetapi.PermissionReservationsWrite):    targetapi.PermissionReservationsWrite,
	string(targetapi.PermissionAssignmentsRead):      targetapi.PermissionAssignmentsRead,
	string(targetapi.PermissionAssignmentsWrite):     targetapi.PermissionAssignmentsWrite,
}

type availabilityKeySet struct {
	delegate coreoidc.KeySet
}

func (keySet availabilityKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	payload, err := keySet.delegate.VerifySignature(ctx, jwt)
	if err == nil {
		return payload, nil
	}
	var unavailable *providerUnavailableError
	if errors.As(err, &unavailable) {
		if state, ok := ctx.Value(verificationStateKey{}).(*verificationState); ok {
			state.providerUnavailable = true
		}
	}
	return nil, err
}

type availabilityTransport struct {
	delegate http.RoundTripper
}

func (transport availabilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.delegate.RoundTrip(request)
	if err != nil {
		return nil, &providerUnavailableError{cause: err}
	}
	if response.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		_ = response.Body.Close()
		return nil, &providerUnavailableError{status: response.StatusCode}
	}
	return response, nil
}

type providerUnavailableError struct {
	cause  error
	status int
}

func (err *providerUnavailableError) Error() string {
	if err.status != 0 {
		return fmt.Sprintf("OIDC provider returned HTTP %d", err.status)
	}
	return "OIDC provider request failed"
}

func (err *providerUnavailableError) Unwrap() error {
	return err.cause
}
