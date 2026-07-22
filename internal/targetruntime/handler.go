// Package targetruntime composes the authenticated target API with the
// provider-neutral health and admission boundaries required by a remote
// service process.
package targetruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
)

type Options struct {
	CursorKey        []byte
	ReservationTTL   time.Duration
	MaxInFlight      int
	ReadinessTimeout time.Duration
	ReadinessCheck   func(context.Context) error
}

// New returns one handler with unauthenticated health probes and an
// authenticated, bounded target API. Readiness uses the supplied infrastructure
// check and never creates or reads a tenant scope.
func New(
	owner repository.Repository,
	authenticator targetapi.Authenticator,
	options Options,
) (http.Handler, error) {
	if owner == nil {
		return nil, fmt.Errorf("target runtime repository is required")
	}
	if options.MaxInFlight <= 0 {
		return nil, fmt.Errorf("target runtime max in-flight requests must be positive")
	}
	if options.ReadinessTimeout <= 0 {
		return nil, fmt.Errorf("target runtime readiness timeout must be positive")
	}
	if options.ReadinessCheck == nil {
		return nil, fmt.Errorf("target runtime readiness check is required")
	}
	apiHandler, err := targetapi.New(owner, authenticator, targetapi.Options{
		CursorKey: options.CursorKey, ReservationTTL: options.ReservationTTL,
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(writer http.ResponseWriter, _ *http.Request) {
		writeHealth(writer, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), options.ReadinessTimeout)
		defer cancel()
		if err := options.ReadinessCheck(ctx); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeHealth(writer, http.StatusOK, "ok")
	})
	mux.HandleFunc("/livez", healthMethodNotAllowed)
	mux.HandleFunc("/readyz", healthMethodNotAllowed)
	mux.Handle("/", bounded(apiHandler, options.MaxInFlight))
	return mux, nil
}

func bounded(next http.Handler, maximum int) http.Handler {
	permits := make(chan struct{}, maximum)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case permits <- struct{}{}:
			defer func() { <-permits }()
			next.ServeHTTP(writer, request)
		default:
			writer.Header().Set("Retry-After", "1")
			writeEnvelope(writer, http.StatusServiceUnavailable, api.Envelope{
				APIVersion: api.Version,
				Error: &api.Failure{
					Code: "ResourceExhausted", Message: "target runtime request capacity is exhausted", Retryable: true,
				},
			})
		}
	})
}

func healthMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writeHealth(writer, http.StatusMethodNotAllowed, "method_not_allowed")
}

func writeHealth(writer http.ResponseWriter, status int, value string) {
	writeEnvelope(writer, status, map[string]string{"status": value})
}

func writeEnvelope(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
