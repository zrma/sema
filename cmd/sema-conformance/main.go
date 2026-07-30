package main

import (
	"context"
	"os"

	"github.com/zrma/sema/internal/wireconformance"
)

var version = "dev"

func main() {
	os.Exit(wireconformance.Run(
		context.Background(),
		os.Args[1:],
		os.LookupEnv,
		os.Stdout,
		os.Stderr,
		wireconformance.Identity{
			ProgramName:  "sema-conformance",
			Version:      version,
			ReportSchema: "sema.wire-conformance.v1",
		},
	))
}
