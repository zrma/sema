// Package wirefixture provides a loopback-only service process for tagged
// client-service compatibility checks. It exercises the real target wire
// handler without replacing the PostgreSQL/OIDC operational gates.
package wirefixture

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/targetruntime"
	"github.com/zrma/sema/internal/wireconformance"
)

const maximumTokenBytes = 16 << 10

// Identity controls the executable name and version.
type Identity struct {
	ProgramName string
	Version     string
}

type configuration struct {
	listen           string
	writeToken       string
	readToken        string
	otherTenantToken string
}

type dependencies struct {
	listen func(string, string) (net.Listener, error)
}

// Run starts the loopback-only wire fixture.
func Run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	identity Identity,
) int {
	return run(ctx, args, lookupEnvironment, stdout, stderr, identity, dependencies{listen: net.Listen})
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	identity Identity,
	deps dependencies,
) int {
	if identity.ProgramName == "" {
		identity.ProgramName = "sema-wire-fixture"
	}
	if identity.Version == "" {
		identity.Version = "dev"
	}
	config, showVersion, err := parseConfiguration(args, lookupEnvironment, stderr, identity.ProgramName)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "%s %s\n", identity.ProgramName, identity.Version)
		return 0
	}
	if deps.listen == nil {
		fmt.Fprintf(stderr, "%s: listener dependency is unavailable\n", identity.ProgramName)
		return 1
	}

	handler, err := targetruntime.New(
		repository.NewMemory(),
		authenticator(config),
		targetruntime.Options{
			CursorKey:        []byte("wire-fixture-cursor-key-32-bytes"),
			ReservationTTL:   time.Minute,
			MaxInFlight:      16,
			RequestTimeout:   5 * time.Second,
			ReadinessTimeout: time.Second,
			ReadinessCheck:   func(context.Context) error { return nil },
		},
	)
	if err != nil {
		fmt.Fprintf(stderr, "%s: configure target runtime: %v\n", identity.ProgramName, err)
		return 1
	}
	listener, err := deps.listen("tcp", config.listen)
	if err != nil {
		fmt.Fprintf(stderr, "%s: listen: %v\n", identity.ProgramName, err)
		return 1
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	fmt.Fprintf(stdout, "%s listening on %s\n", identity.ProgramName, listener.Addr())

	select {
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "%s: serve: %v\n", identity.ProgramName, serveErr)
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
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
	programName string,
) (configuration, bool, error) {
	config := configuration{listen: "127.0.0.1:0"}
	flags := flag.NewFlagSet(programName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&config.listen, "listen", config.listen, "loopback TCP listen address")
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
	host, _, err := net.SplitHostPort(config.listen)
	if err != nil {
		fmt.Fprintf(stderr, "%s: listen address must include a valid host and port\n", programName)
		return configuration{}, false, fmt.Errorf("invalid listen address")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		fmt.Fprintf(stderr, "%s: listen address must use an IP loopback host\n", programName)
		return configuration{}, false, fmt.Errorf("non-loopback listen address")
	}

	config.writeToken, _ = lookupEnvironment(wireconformance.WriteTokenEnvironment)
	config.readToken, _ = lookupEnvironment(wireconformance.ReadTokenEnvironment)
	config.otherTenantToken, _ = lookupEnvironment(wireconformance.OtherTenantTokenEnvironment)
	if err := validateTokens(config); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return configuration{}, false, err
	}
	return config, false, nil
}

func validateTokens(config configuration) error {
	for name, token := range map[string]string{
		wireconformance.WriteTokenEnvironment:       config.writeToken,
		wireconformance.ReadTokenEnvironment:        config.readToken,
		wireconformance.OtherTenantTokenEnvironment: config.otherTenantToken,
	} {
		if token == "" || token != strings.TrimSpace(token) || len(token) > maximumTokenBytes {
			return fmt.Errorf("%s must contain one bounded token without surrounding whitespace", name)
		}
	}
	if config.writeToken == config.readToken ||
		config.writeToken == config.otherTenantToken ||
		config.readToken == config.otherTenantToken {
		return fmt.Errorf("write, read-only, and other-tenant tokens must be distinct")
	}
	return nil
}

func authenticator(config configuration) targetapi.Authenticator {
	all := map[targetapi.Permission]bool{
		targetapi.PermissionMatchTicketsRead: true, targetapi.PermissionMatchTicketsWrite: true,
		targetapi.PermissionBackfillTicketsRead: true, targetapi.PermissionBackfillTicketsWrite: true,
		targetapi.PermissionPoliciesRead: true, targetapi.PermissionPoliciesWrite: true,
		targetapi.PermissionPlanningRunsRead: true, targetapi.PermissionPlanningRunsWrite: true,
		targetapi.PermissionReservationsRead: true, targetapi.PermissionReservationsWrite: true,
		targetapi.PermissionAssignmentsRead: true, targetapi.PermissionAssignmentsWrite: true,
	}
	return targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
		authorization := request.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
		switch strings.TrimPrefix(authorization, "Bearer ") {
		case config.writeToken:
			return targetapi.Principal{
				Subject:     "wire-fixture-writer",
				Tenant:      "wire-fixture-tenant",
				Permissions: all,
			}, nil
		case config.readToken:
			return targetapi.Principal{
				Subject: "wire-fixture-reader",
				Tenant:  "wire-fixture-tenant",
				Permissions: map[targetapi.Permission]bool{
					targetapi.PermissionMatchTicketsRead: true,
				},
			}, nil
		case config.otherTenantToken:
			return targetapi.Principal{
				Subject: "wire-fixture-other-reader",
				Tenant:  "wire-fixture-other-tenant",
				Permissions: map[targetapi.Permission]bool{
					targetapi.PermissionMatchTicketsRead: true,
				},
			}, nil
		default:
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
	})
}
