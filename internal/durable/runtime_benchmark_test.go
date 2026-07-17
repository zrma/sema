//go:build darwin || linux

package durable_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/zrma/sema/internal/durable"
)

func BenchmarkOpenReplay(b *testing.B) {
	for _, ticketCount := range []int{100, 1000} {
		b.Run(fmt.Sprintf("events-%d", ticketCount+2), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "sema.journal")
			runtime, err := durable.Open(path, time.Minute)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := runtime.RegisterPolicy(testPolicy()); err != nil {
				b.Fatal(err)
			}
			for _, ticket := range soloTickets(ticketCount) {
				if err := runtime.SubmitMatchTicket(ticket); err != nil {
					b.Fatal(err)
				}
			}
			if err := runtime.Close(); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ReportMetric(float64(ticketCount+2), "events/op")
			b.ResetTimer()
			for range b.N {
				recovered, err := durable.Open(path, time.Minute)
				if err != nil {
					b.Fatal(err)
				}
				if err := recovered.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
