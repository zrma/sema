package targetruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
)

func TestHandlerExposesHealthWithoutWeakeningTargetAuthentication(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())

	for _, path := range []string{"/livez", "/readyz"} {
		request := httptest.NewRequest(http.MethodGet, "https://sema.example"+path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("target API status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessFailsClosedWithoutRepositoryDetails(t *testing.T) {
	handler, err := New(repository.NewMemory(), targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}), Options{
		CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
		MaxInFlight: 2, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		ReadinessCheck: func(context.Context) error {
			return errors.New("private database endpoint is unavailable")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://sema.example/readyz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("readiness status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestAdmissionRejectsExcessWithoutBlockingHealth(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		once.Do(func() { close(entered) })
		<-release
	})
	handler := bounded(next, 1)

	finished := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
		close(finished)
	}()
	<-entered

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
	if recorder.Code != http.StatusServiceUnavailable ||
		recorder.Header().Get("Retry-After") != "1" ||
		recorder.Header().Get("X-Sema-Error-Code") != "ResourceExhausted" {
		t.Fatalf(
			"overload status = %d, retry-after = %q, code = %q",
			recorder.Code,
			recorder.Header().Get("Retry-After"),
			recorder.Header().Get("X-Sema-Error-Code"),
		)
	}
	close(release)
	<-finished
}

func TestRequestTimeoutCancelsTargetOperation(t *testing.T) {
	cancelled := make(chan struct{})
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		close(cancelled)
		writer.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := withTimeout(next, 10*time.Millisecond)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request context was not cancelled")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout status = %d", recorder.Code)
	}
}

func TestNewRejectsUnsafeOptions(t *testing.T) {
	owner := repository.NewMemory()
	authenticator := targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	})
	for name, options := range map[string]Options{
		"missing cursor key": {
			ReservationTTL: time.Minute, MaxInFlight: 1, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero admission": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero request timeout": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
			MaxInFlight: 1, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero readiness": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute, MaxInFlight: 1, RequestTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"missing readiness check": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
			MaxInFlight: 1, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(owner, authenticator, options); err == nil {
				t.Fatal("unsafe runtime options were accepted")
			}
		})
	}
}

func newTestHandler(t *testing.T, owner repository.Repository) http.Handler {
	t.Helper()
	handler, err := New(owner, targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}), Options{
		CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
		MaxInFlight: 2, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		ReadinessCheck: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
