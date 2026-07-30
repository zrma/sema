package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zrma/sema/internal/serviceapp"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(serviceapp.Run(
		ctx,
		os.Args[1:],
		os.LookupEnv,
		os.Stdout,
		os.Stderr,
		serviceapp.Identity{ProgramName: "sema-service", Version: version},
	))
}
