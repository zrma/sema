package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	oidcauth "github.com/zrma/sema/internal/authn/oidc"
	"github.com/zrma/sema/internal/repository"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/targetruntime"
)

const (
	postgresDSNEnvironment       = "SEMA_POSTGRES_DSN"
	cursorKeyEnvironment         = "SEMA_CURSOR_KEY_BASE64"
	oidcIssuerEnvironment        = "SEMA_OIDC_ISSUER"
	oidcAudienceEnvironment      = "SEMA_OIDC_AUDIENCE"
	oidcTenantClaimEnvironment   = "SEMA_OIDC_TENANT_CLAIM"
	oidcAlgorithmsEnvironment    = "SEMA_OIDC_SIGNING_ALGORITHMS"
	tlsTerminationEnvironment    = "SEMA_TLS_TERMINATION"
	externalTLSTermination       = "external"
	maximumConfiguredConcurrency = 4096
)

var version = "dev"

type configuration struct {
	listen             string
	postgresDSN        string
	cursorKey          []byte
	oidcIssuer         string
	oidcAudience       string
	oidcTenantClaim    string
	oidcAlgorithms     []string
	reservationTTL     time.Duration
	maxInFlight        int
	readinessTimeout   time.Duration
	startupTimeout     time.Duration
	shutdownTimeout    time.Duration
	tlsTerminationMode string
}

type repositoryOwner interface {
	repository.Repository
	Ready(context.Context) error
	Close()
}

type dependencies struct {
	openRepository   func(context.Context, string) (repositoryOwner, error)
	newAuthenticator func(context.Context, oidcauth.Config) (targetapi.Authenticator, error)
	listen           func(string, string) (net.Listener, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr, defaultDependencies()))
}

func defaultDependencies() dependencies {
	return dependencies{
		openRepository: func(ctx context.Context, dsn string) (repositoryOwner, error) {
			return postgresrepository.Open(ctx, dsn)
		},
		newAuthenticator: func(ctx context.Context, config oidcauth.Config) (targetapi.Authenticator, error) {
			return oidcauth.New(ctx, config)
		},
		listen: net.Listen,
	}
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	config, showVersion, err := parseConfiguration(args, lookupEnvironment, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-target-server %s\n", version)
		return 0
	}
	if deps.openRepository == nil || deps.newAuthenticator == nil || deps.listen == nil {
		fmt.Fprintln(stderr, "sema-target-server: runtime dependencies are incomplete")
		return 1
	}

	startupContext, cancelStartup := context.WithTimeout(ctx, config.startupTimeout)
	defer cancelStartup()
	owner, err := deps.openRepository(startupContext, config.postgresDSN)
	if err != nil {
		fmt.Fprintln(stderr, "sema-target-server: PostgreSQL repository initialization failed")
		return 1
	}
	defer owner.Close()
	authenticator, err := deps.newAuthenticator(startupContext, oidcauth.Config{
		Issuer: config.oidcIssuer, Audience: config.oidcAudience,
		TenantClaim: config.oidcTenantClaim, SigningAlgorithms: config.oidcAlgorithms,
	})
	if err != nil {
		fmt.Fprintln(stderr, "sema-target-server: OIDC authentication initialization failed")
		return 1
	}
	handler, err := targetruntime.New(owner, authenticator, targetruntime.Options{
		CursorKey: config.cursorKey, ReservationTTL: config.reservationTTL,
		MaxInFlight: config.maxInFlight, ReadinessTimeout: config.readinessTimeout,
		ReadinessCheck: owner.Ready,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sema-target-server: configure target runtime: %v\n", err)
		return 1
	}

	listener, err := deps.listen("tcp", config.listen)
	if err != nil {
		fmt.Fprintf(stderr, "sema-target-server: listen: %v\n", err)
		return 1
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	fmt.Fprintf(stdout, "sema-target-server listening on %s behind external TLS termination\n", listener.Addr())

	select {
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "sema-target-server: serve: %v\n", serveErr)
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), config.shutdownTimeout)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownContext); err != nil {
			fmt.Fprintf(stderr, "sema-target-server: shutdown: %v\n", err)
			return 1
		}
		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "sema-target-server: serve: %v\n", serveErr)
			return 1
		}
		return 0
	}
}

func parseConfiguration(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
) (configuration, bool, error) {
	config := configuration{}
	flags := flag.NewFlagSet("sema-target-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&config.listen, "listen", "0.0.0.0:8080", "private TCP listen address behind external TLS termination")
	flags.DurationVar(&config.reservationTTL, "reservation-ttl", 30*time.Second, "fixed reservation TTL")
	flags.IntVar(&config.maxInFlight, "max-in-flight", 128, "maximum concurrent target API requests")
	flags.DurationVar(&config.readinessTimeout, "readiness-timeout", 2*time.Second, "PostgreSQL readiness probe timeout")
	flags.DurationVar(&config.startupTimeout, "startup-timeout", 15*time.Second, "PostgreSQL and OIDC startup timeout")
	flags.DurationVar(&config.shutdownTimeout, "shutdown-timeout", 10*time.Second, "graceful shutdown timeout")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-target-server [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return configuration{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return configuration{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		return config, true, nil
	}
	if lookupEnvironment == nil {
		return configuration{}, false, fmt.Errorf("environment lookup is required")
	}

	config.postgresDSN = requiredEnvironment(lookupEnvironment, postgresDSNEnvironment)
	rawCursorKey := requiredEnvironment(lookupEnvironment, cursorKeyEnvironment)
	config.oidcIssuer = requiredEnvironment(lookupEnvironment, oidcIssuerEnvironment)
	config.oidcAudience = requiredEnvironment(lookupEnvironment, oidcAudienceEnvironment)
	config.oidcTenantClaim = optionalEnvironment(lookupEnvironment, oidcTenantClaimEnvironment)
	config.tlsTerminationMode = requiredEnvironment(lookupEnvironment, tlsTerminationEnvironment)
	rawAlgorithms := optionalEnvironment(lookupEnvironment, oidcAlgorithmsEnvironment)

	if config.postgresDSN == "" || rawCursorKey == "" || config.oidcIssuer == "" ||
		config.oidcAudience == "" || config.tlsTerminationMode == "" {
		fmt.Fprintln(stderr, "sema-target-server: required runtime environment is incomplete")
		return configuration{}, false, fmt.Errorf("missing required runtime environment")
	}
	if config.oidcTenantClaim != strings.TrimSpace(config.oidcTenantClaim) {
		fmt.Fprintln(stderr, "sema-target-server: SEMA_OIDC_TENANT_CLAIM must not contain surrounding whitespace")
		return configuration{}, false, fmt.Errorf("invalid OIDC tenant claim")
	}
	if config.tlsTerminationMode != externalTLSTermination {
		fmt.Fprintln(stderr, "sema-target-server: SEMA_TLS_TERMINATION must be external")
		return configuration{}, false, fmt.Errorf("unsupported TLS termination mode")
	}
	decodedCursorKey, err := base64.StdEncoding.Strict().DecodeString(rawCursorKey)
	if err != nil || len(decodedCursorKey) < 32 {
		fmt.Fprintln(stderr, "sema-target-server: SEMA_CURSOR_KEY_BASE64 must encode at least 32 bytes")
		return configuration{}, false, fmt.Errorf("invalid cursor authentication key")
	}
	config.cursorKey = decodedCursorKey
	if rawAlgorithms != "" {
		for _, value := range strings.Split(rawAlgorithms, ",") {
			if value == "" || value != strings.TrimSpace(value) {
				fmt.Fprintln(stderr, "sema-target-server: SEMA_OIDC_SIGNING_ALGORITHMS must be a comma-separated list without whitespace")
				return configuration{}, false, fmt.Errorf("invalid OIDC signing algorithms")
			}
			config.oidcAlgorithms = append(config.oidcAlgorithms, value)
		}
	}
	if _, _, err := net.SplitHostPort(config.listen); err != nil {
		fmt.Fprintln(stderr, "sema-target-server: listen address must include a valid host and port")
		return configuration{}, false, fmt.Errorf("invalid listen address")
	}
	if config.reservationTTL <= 0 || config.maxInFlight <= 0 || config.maxInFlight > maximumConfiguredConcurrency ||
		config.readinessTimeout <= 0 || config.startupTimeout <= 0 || config.shutdownTimeout <= 0 {
		fmt.Fprintln(stderr, "sema-target-server: durations and bounded request concurrency must be positive and safe")
		return configuration{}, false, fmt.Errorf("invalid runtime bounds")
	}
	return config, false, nil
}

func requiredEnvironment(lookup func(string) (string, bool), name string) string {
	value, exists := lookup(name)
	if !exists || value == "" || value != strings.TrimSpace(value) {
		return ""
	}
	return value
}

func optionalEnvironment(lookup func(string) (string, bool), name string) string {
	value, exists := lookup(name)
	if !exists || value == "" {
		return ""
	}
	return value
}
