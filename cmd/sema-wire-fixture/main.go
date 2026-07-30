package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zrma/sema/internal/wirefixture"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(wirefixture.Run(
		ctx,
		os.Args[1:],
		os.LookupEnv,
		os.Stdout,
		os.Stderr,
		wirefixture.Identity{ProgramName: "sema-wire-fixture", Version: version},
	))
}
