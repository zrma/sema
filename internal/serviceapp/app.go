package serviceapp

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	maximumConfiguredPoolSize    = 4096
)

const compatibilityProgramName = "sema-target-server"

// Identity controls the executable name and build version exposed by a service
// command while all commands share the same runtime implementation.
type Identity struct {
	ProgramName string
	Version     string
}

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
	requestTimeout     time.Duration
	readinessTimeout   time.Duration
	startupTimeout     time.Duration
	shutdownTimeout    time.Duration
	postgresPool       postgresrepository.PoolOptions
	tlsTerminationMode string
}

type repositoryOwner interface {
	repository.Repository
	Ready(context.Context) error
	Close()
}

type dependencies struct {
	openRepository   func(context.Context, string, postgresrepository.PoolOptions) (repositoryOwner, error)
	newAuthenticator func(context.Context, oidcauth.Config) (targetapi.Authenticator, error)
	listen           func(string, string) (net.Listener, error)
}

// Run starts the PostgreSQL/OIDC service runtime for a command identity.
func Run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	identity Identity,
) int {
	return runWithIdentity(ctx, args, lookupEnvironment, stdout, stderr, defaultDependencies(), identity)
}

func defaultDependencies() dependencies {
	return dependencies{
		openRepository: func(
			ctx context.Context,
			dsn string,
			options postgresrepository.PoolOptions,
		) (repositoryOwner, error) {
			return postgresrepository.OpenWithOptions(ctx, dsn, options)
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
	return runWithIdentity(ctx, args, lookupEnvironment, stdout, stderr, deps, Identity{
		ProgramName: compatibilityProgramName,
		Version:     "dev",
	})
}

func runWithIdentity(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
	identity Identity,
) int {
	if identity.ProgramName == "" {
		identity.ProgramName = compatibilityProgramName
	}
	if identity.Version == "" {
		identity.Version = "dev"
	}
	config, showVersion, err := parseConfigurationWithName(args, lookupEnvironment, stderr, identity.ProgramName)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "%s %s\n", identity.ProgramName, identity.Version)
		return 0
	}
	if deps.openRepository == nil || deps.newAuthenticator == nil || deps.listen == nil {
		fmt.Fprintf(stderr, "%s: runtime dependencies are incomplete\n", identity.ProgramName)
		return 1
	}

	startupContext, cancelStartup := context.WithTimeout(ctx, config.startupTimeout)
	defer cancelStartup()
	owner, err := deps.openRepository(startupContext, config.postgresDSN, config.postgresPool)
	if err != nil {
		fmt.Fprintf(stderr, "%s: PostgreSQL repository initialization failed\n", identity.ProgramName)
		return 1
	}
	defer owner.Close()
	authenticator, err := deps.newAuthenticator(startupContext, oidcauth.Config{
		Issuer: config.oidcIssuer, Audience: config.oidcAudience,
		TenantClaim: config.oidcTenantClaim, SigningAlgorithms: config.oidcAlgorithms,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s: OIDC authentication initialization failed\n", identity.ProgramName)
		return 1
	}
	handler, err := targetruntime.New(owner, authenticator, targetruntime.Options{
		CursorKey: config.cursorKey, ReservationTTL: config.reservationTTL,
		MaxInFlight: config.maxInFlight, RequestTimeout: config.requestTimeout,
		ReadinessTimeout: config.readinessTimeout,
		ReadinessCheck:   owner.Ready,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s: configure target runtime: %v\n", identity.ProgramName, err)
		return 1
	}

	listener, err := deps.listen("tcp", config.listen)
	if err != nil {
		fmt.Fprintf(stderr, "%s: listen: %v\n", identity.ProgramName, err)
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
	fmt.Fprintf(stdout, "%s listening on %s behind external TLS termination\n", identity.ProgramName, listener.Addr())

	select {
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "%s: serve: %v\n", identity.ProgramName, serveErr)
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), config.shutdownTimeout)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownContext); err != nil {
			fmt.Fprintf(stderr, "%s: shutdown: %v\n", identity.ProgramName, err)
			return 1
		}
		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "%s: serve: %v\n", identity.ProgramName, serveErr)
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
	return parseConfigurationWithName(args, lookupEnvironment, stderr, compatibilityProgramName)
}

func parseConfigurationWithName(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
	programName string,
) (configuration, bool, error) {
	config := configuration{postgresPool: postgresrepository.DefaultPoolOptions()}
	flags := flag.NewFlagSet(programName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&config.listen, "listen", "0.0.0.0:8080", "private TCP listen address behind external TLS termination")
	flags.DurationVar(&config.reservationTTL, "reservation-ttl", 30*time.Second, "fixed reservation TTL")
	flags.IntVar(&config.maxInFlight, "max-in-flight", 64, "maximum concurrent target API requests")
	flags.DurationVar(&config.requestTimeout, "request-timeout", 5*time.Second, "maximum target API operation duration")
	flags.DurationVar(&config.readinessTimeout, "readiness-timeout", 2*time.Second, "PostgreSQL readiness probe timeout")
	flags.DurationVar(&config.startupTimeout, "startup-timeout", 15*time.Second, "PostgreSQL and OIDC startup timeout")
	flags.DurationVar(&config.shutdownTimeout, "shutdown-timeout", 10*time.Second, "graceful shutdown timeout")
	flags.Var(
		(*int32Flag)(&config.postgresPool.MaxConnections),
		"postgres-max-conns",
		"maximum PostgreSQL connections per service replica",
	)
	flags.Var(
		(*int32Flag)(&config.postgresPool.MinIdleConnections),
		"postgres-min-idle-conns",
		"minimum idle PostgreSQL connections per service replica",
	)
	flags.DurationVar(
		&config.postgresPool.MaxConnectionAge,
		"postgres-max-conn-age",
		postgresrepository.DefaultMaxConnectionAge,
		"maximum PostgreSQL connection lifetime",
	)
	flags.DurationVar(
		&config.postgresPool.MaxConnectionIdle,
		"postgres-max-conn-idle",
		postgresrepository.DefaultMaxConnectionIdle,
		"maximum PostgreSQL idle connection duration",
	)
	flags.DurationVar(
		&config.postgresPool.HealthCheckPeriod,
		"postgres-health-check-period",
		postgresrepository.DefaultPoolHealthCheck,
		"PostgreSQL idle connection health-check period",
	)
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s [flags]\n", programName)
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
		fmt.Fprintf(stderr, "%s: required runtime environment is incomplete\n", programName)
		return configuration{}, false, fmt.Errorf("missing required runtime environment")
	}
	if config.oidcTenantClaim != strings.TrimSpace(config.oidcTenantClaim) {
		fmt.Fprintf(stderr, "%s: SEMA_OIDC_TENANT_CLAIM must not contain surrounding whitespace\n", programName)
		return configuration{}, false, fmt.Errorf("invalid OIDC tenant claim")
	}
	if config.tlsTerminationMode != externalTLSTermination {
		fmt.Fprintf(stderr, "%s: SEMA_TLS_TERMINATION must be external\n", programName)
		return configuration{}, false, fmt.Errorf("unsupported TLS termination mode")
	}
	decodedCursorKey, err := base64.StdEncoding.Strict().DecodeString(rawCursorKey)
	if err != nil || len(decodedCursorKey) < 32 {
		fmt.Fprintf(stderr, "%s: SEMA_CURSOR_KEY_BASE64 must encode at least 32 bytes\n", programName)
		return configuration{}, false, fmt.Errorf("invalid cursor authentication key")
	}
	config.cursorKey = decodedCursorKey
	if rawAlgorithms != "" {
		for _, value := range strings.Split(rawAlgorithms, ",") {
			if value == "" || value != strings.TrimSpace(value) {
				fmt.Fprintf(stderr, "%s: SEMA_OIDC_SIGNING_ALGORITHMS must be a comma-separated list without whitespace\n", programName)
				return configuration{}, false, fmt.Errorf("invalid OIDC signing algorithms")
			}
			config.oidcAlgorithms = append(config.oidcAlgorithms, value)
		}
	}
	if _, _, err := net.SplitHostPort(config.listen); err != nil {
		fmt.Fprintf(stderr, "%s: listen address must include a valid host and port\n", programName)
		return configuration{}, false, fmt.Errorf("invalid listen address")
	}
	if config.reservationTTL <= 0 || config.maxInFlight <= 0 || config.maxInFlight > maximumConfiguredConcurrency ||
		config.requestTimeout <= 0 ||
		config.readinessTimeout <= 0 || config.startupTimeout <= 0 || config.shutdownTimeout <= 0 {
		fmt.Fprintf(stderr, "%s: durations and bounded request concurrency must be positive and safe\n", programName)
		return configuration{}, false, fmt.Errorf("invalid runtime bounds")
	}
	if config.postgresPool.MaxConnections <= 0 ||
		config.postgresPool.MaxConnections > maximumConfiguredPoolSize ||
		config.postgresPool.MinIdleConnections < 0 ||
		config.postgresPool.MinIdleConnections > config.postgresPool.MaxConnections ||
		config.postgresPool.MaxConnectionAge <= 0 ||
		config.postgresPool.MaxConnectionIdle <= 0 ||
		config.postgresPool.HealthCheckPeriod <= 0 {
		fmt.Fprintf(stderr, "%s: PostgreSQL pool bounds must be positive and internally consistent\n", programName)
		return configuration{}, false, fmt.Errorf("invalid PostgreSQL pool bounds")
	}
	return config, false, nil
}

type int32Flag int32

func (value *int32Flag) String() string {
	if value == nil {
		return "0"
	}
	return fmt.Sprintf("%d", *value)
}

func (value *int32Flag) Set(raw string) error {
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return err
	}
	*value = int32Flag(parsed)
	return nil
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
