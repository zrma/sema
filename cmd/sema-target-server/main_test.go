//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	oidcauth "github.com/zrma/sema/internal/authn/oidc"
	"github.com/zrma/sema/internal/repository"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/targetapi"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

func TestRunPrintsVersionWithoutRuntimeEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-version"}, emptyEnvironment, &stdout, &stderr, dependencies{})
	if code != 0 || stdout.String() != "sema-target-server dev\n" {
		t.Fatalf("exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestParseConfigurationRequiresExplicitSecurityInputs(t *testing.T) {
	var stderr bytes.Buffer
	if _, _, err := parseConfiguration(nil, emptyEnvironment, &stderr); err == nil {
		t.Fatal("missing security configuration was accepted")
	}
	if !strings.Contains(stderr.String(), "required runtime environment is incomplete") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseConfigurationLoadsProviderNeutralEnvironment(t *testing.T) {
	environment := validEnvironment()
	environment[oidcTenantClaimEnvironment] = "tenant_id"
	environment[oidcAlgorithmsEnvironment] = "RS256,PS256"
	var stderr bytes.Buffer
	config, showVersion, err := parseConfiguration(
		[]string{"-listen", "127.0.0.1:0", "-max-in-flight", "64"}, mapEnvironment(environment), &stderr,
	)
	if err != nil {
		t.Fatalf("parse configuration: %v, stderr=%q", err, stderr.String())
	}
	if showVersion || config.listen != "127.0.0.1:0" || config.maxInFlight != 64 ||
		config.oidcTenantClaim != "tenant_id" || len(config.oidcAlgorithms) != 2 ||
		config.oidcAlgorithms[0] != "RS256" || config.oidcAlgorithms[1] != "PS256" ||
		len(config.cursorKey) != 32 {
		t.Fatalf("configuration = %#v", config)
	}
}

func TestParseConfigurationRejectsUnsafeTLSAndSecretWithoutEcho(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"implicit TLS": func(environment map[string]string) {
			delete(environment, tlsTerminationEnvironment)
		},
		"unsupported TLS": func(environment map[string]string) {
			environment[tlsTerminationEnvironment] = "disabled"
		},
		"short cursor key": func(environment map[string]string) {
			environment[cursorKeyEnvironment] = base64.StdEncoding.EncodeToString([]byte("do-not-print-this-secret"))
		},
		"unbounded admission": func(environment map[string]string) {
			environment[oidcAudienceEnvironment] = "sema"
		},
	} {
		t.Run(name, func(t *testing.T) {
			environment := validEnvironment()
			mutate(environment)
			args := []string{}
			if name == "unbounded admission" {
				args = []string{"-max-in-flight", "4097"}
			}
			var stderr bytes.Buffer
			if _, _, err := parseConfiguration(args, mapEnvironment(environment), &stderr); err == nil {
				t.Fatal("unsafe configuration was accepted")
			}
			if strings.Contains(stderr.String(), "do-not-print-this-secret") || strings.Contains(stderr.String(), environment[postgresDSNEnvironment]) {
				t.Fatalf("stderr exposed secret material: %q", stderr.String())
			}
		})
	}
}

func TestRunComposesAndGracefullyStopsRemoteRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	owner := &closingRepository{Repository: repository.NewMemory()}
	deps := dependencies{
		openRepository: func(context.Context, string) (repositoryOwner, error) { return owner, nil },
		newAuthenticator: func(context.Context, oidcauth.Config) (targetapi.Authenticator, error) {
			return targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
				return targetapi.Principal{}, targetapi.ErrUnauthenticated
			}), nil
		},
		listen: func(_, _ string) (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") },
	}
	var stdout, stderr bytes.Buffer
	code := run(ctx, []string{"-listen", "127.0.0.1:0"}, mapEnvironment(validEnvironment()), &stdout, &stderr, deps)
	if code != 0 || !strings.Contains(stdout.String(), "behind external TLS termination") || !owner.closed {
		t.Fatalf("exit code = %d, closed=%t, stdout=%q, stderr=%q", code, owner.closed, stdout.String(), stderr.String())
	}
}

func TestRunFailsBeforeListeningWhenOIDCConfigurationFails(t *testing.T) {
	owner := &closingRepository{Repository: repository.NewMemory()}
	listenCalled := false
	deps := dependencies{
		openRepository: func(context.Context, string) (repositoryOwner, error) { return owner, nil },
		newAuthenticator: func(context.Context, oidcauth.Config) (targetapi.Authenticator, error) {
			return nil, errors.New("provider configuration rejected")
		},
		listen: func(_, _ string) (net.Listener, error) {
			listenCalled = true
			return nil, errors.New("must not listen")
		},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, mapEnvironment(validEnvironment()), &stdout, &stderr, deps)
	if code != 1 || listenCalled || !owner.closed {
		t.Fatalf("exit code = %d, listen=%t, closed=%t, stderr=%q", code, listenCalled, owner.closed, stderr.String())
	}
}

func TestRunRedactsRepositoryInitializationFailure(t *testing.T) {
	const sensitiveDetail = "deployment-only database detail"
	deps := dependencies{
		openRepository: func(context.Context, string) (repositoryOwner, error) {
			return nil, errors.New(sensitiveDetail)
		},
		newAuthenticator: func(context.Context, oidcauth.Config) (targetapi.Authenticator, error) {
			return nil, errors.New("must not authenticate")
		},
		listen: net.Listen,
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, mapEnvironment(validEnvironment()), &stdout, &stderr, deps)
	if code != 1 || strings.Contains(stderr.String(), sensitiveDetail) ||
		!strings.Contains(stderr.String(), "PostgreSQL repository initialization failed") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestPostgreSQLAndOIDCEndToEndRuntime(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip(postgresTestDSNEnvironment + " is not set")
	}
	contextWithTimeout, cancelSetup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelSetup()
	schema := fmt.Sprintf("sema_target_runtime_%d", time.Now().UnixNano())
	admin, err := pgxpool.New(contextWithTimeout, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(contextWithTimeout, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupContext, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	runtimeDSN := withSearchPath(t, dsn, schema)
	migrationPool, err := pgxpool.New(contextWithTimeout, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresrepository.Migrate(contextWithTimeout, migrationPool); err != nil {
		migrationPool.Close()
		t.Fatal(err)
	}
	migrationPool.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
		PublicKey: &key.PublicKey, KeyID: "runtime-key", Algorithm: coreoidc.RS256,
	}}}
	providerServer := httptest.NewTLSServer(provider)
	defer providerServer.Close()
	provider.SetIssuer(providerServer.URL)

	address := make(chan string, 1)
	deps := defaultDependencies()
	deps.newAuthenticator = func(ctx context.Context, config oidcauth.Config) (targetapi.Authenticator, error) {
		config.HTTPClient = providerServer.Client()
		return oidcauth.New(ctx, config)
	}
	deps.listen = func(network, requested string) (net.Listener, error) {
		listener, listenErr := net.Listen(network, requested)
		if listenErr == nil {
			address <- listener.Addr().String()
		}
		return listener, listenErr
	}
	environment := validEnvironment()
	environment[postgresDSNEnvironment] = runtimeDSN
	environment[oidcIssuerEnvironment] = providerServer.URL
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	var stdout, stderr bytes.Buffer
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- run(
			runtimeContext, []string{"-listen", "127.0.0.1:0"}, mapEnvironment(environment),
			&stdout, &stderr, deps,
		)
	}()

	var runtimeAddress string
	select {
	case runtimeAddress = <-address:
	case code := <-exitCode:
		t.Fatalf("runtime exited before listening: code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	case <-time.After(20 * time.Second):
		t.Fatal("runtime did not open its listener")
	}
	readyURL := "http://" + runtimeAddress + "/readyz"
	waitForReady(t, readyURL)
	assertStatus(t, http.MethodGet, "http://"+runtimeAddress+"/v0alpha2/match-tickets", "", http.StatusUnauthorized)

	claims, err := json.Marshal(map[string]any{
		"iss": providerServer.URL, "aud": "sema", "sub": "runtime-reader",
		"exp": time.Now().Add(time.Minute).Unix(), "sema_tenant": "tenant-a", "scope": "match_tickets.read",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := oidctest.SignIDToken(key, "runtime-key", coreoidc.RS256, string(claims))
	assertStatus(t, http.MethodGet, "http://"+runtimeAddress+"/v0alpha2/match-tickets", token, http.StatusOK)

	cancelRuntime()
	if code := <-exitCode; code != 0 || strings.Contains(stdout.String()+stderr.String(), runtimeDSN) {
		t.Fatalf("exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}

func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func waitForReady(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertStatus(t *testing.T, method, endpoint, bearer string, want int) {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s status = %d; want %d", endpoint, response.StatusCode, want)
	}
}

type closingRepository struct {
	repository.Repository
	closed bool
}

func (*closingRepository) Ready(context.Context) error {
	return nil
}

func (owner *closingRepository) Close() {
	owner.closed = true
}

func validEnvironment() map[string]string {
	return map[string]string{
		postgresDSNEnvironment:    "postgres://service@database.example/sema",
		cursorKeyEnvironment:      base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		oidcIssuerEnvironment:     "https://identity.example/application/o/sema/",
		oidcAudienceEnvironment:   "sema",
		tlsTerminationEnvironment: externalTLSTermination,
	}
}

func mapEnvironment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}

func emptyEnvironment(string) (string, bool) {
	return "", false
}
