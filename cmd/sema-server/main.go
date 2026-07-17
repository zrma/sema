package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
	"github.com/zrma/sema/internal/observability"
)

var version = "dev"

type config struct {
	listen                     string
	journal                    string
	reservationTTL             time.Duration
	allowUnauthenticatedRemote bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) (exitCode int) {
	configuration, showVersion, err := parseConfig(args, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-server %s\n", version)
		return 0
	}
	if !configuration.allowUnauthenticatedRemote && !isLoopbackAddress(configuration.listen) {
		fmt.Fprintln(stderr, "sema-server: non-loopback listen requires -allow-unauthenticated-remote")
		return 2
	}

	runtime, err := durable.Open(configuration.journal, configuration.reservationTTL)
	if err != nil {
		fmt.Fprintf(stderr, "sema-server: open durable runtime: %v\n", err)
		return 1
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			fmt.Fprintf(stderr, "sema-server: close durable runtime: %v\n", err)
			exitCode = 1
		}
	}()

	listener, err := net.Listen("tcp", configuration.listen)
	if err != nil {
		fmt.Fprintf(stderr, "sema-server: listen: %v\n", err)
		return 1
	}
	server := &http.Server{
		Handler: httpapi.NewWithOptions(runtime, httpapi.Options{
			Observer: observability.New(stderr, time.Now),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	fmt.Fprintf(stdout, "sema-server listening on %s\n", listener.Addr())

	select {
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "sema-server: serve: %v\n", serveErr)
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			fmt.Fprintf(stderr, "sema-server: shutdown: %v\n", err)
			return 1
		}
		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "sema-server: serve: %v\n", serveErr)
			return 1
		}
		return 0
	}
}

func parseConfig(args []string, stderr io.Writer) (config, bool, error) {
	configuration := config{}
	flags := flag.NewFlagSet("sema-server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configuration.listen, "listen", "127.0.0.1:8080", "TCP listen address")
	flags.StringVar(&configuration.journal, "journal", "var/sema.journal", "durable journal path")
	flags.DurationVar(&configuration.reservationTTL, "reservation-ttl", 30*time.Second, "fixed reservation TTL")
	flags.BoolVar(
		&configuration.allowUnauthenticatedRemote,
		"allow-unauthenticated-remote",
		false,
		"allow a non-loopback listener without authentication",
	)
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-server [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return config{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if configuration.journal == "" || configuration.reservationTTL <= 0 {
		fmt.Fprintln(stderr, "sema-server: journal and positive reservation TTL are required")
		return config{}, false, fmt.Errorf("invalid runtime configuration")
	}
	return configuration, *showVersion, nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
