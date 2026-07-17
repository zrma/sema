package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var version = "dev"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sema-healthcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("url", "http://127.0.0.1:8080/readyz", "readiness endpoint URL")
	timeout := flags.Duration("timeout", 2*time.Second, "request timeout")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-healthcheck [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *endpoint == "" || *timeout <= 0 {
		flags.Usage()
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "sema-healthcheck %s\n", version)
		return 0
	}

	requestContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, *endpoint, nil)
	if err != nil {
		fmt.Fprintf(stderr, "sema-healthcheck: create request: %v\n", err)
		return 1
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Fprintf(stderr, "sema-healthcheck: readiness request failed: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10)); err != nil {
		fmt.Fprintf(stderr, "sema-healthcheck: read response: %v\n", err)
		return 1
	}
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "sema-healthcheck: readiness status %d\n", response.StatusCode)
		return 1
	}
	return 0
}
