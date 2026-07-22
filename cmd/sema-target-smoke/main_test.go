package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/targetruntime"
)

func TestRunValidatesAuthenticatedLifecycle(t *testing.T) {
	handler, err := targetruntime.New(
		repository.NewMemory(), fixtureAuthenticator(),
		targetruntime.Options{
			CursorKey: bytes.Repeat([]byte{7}, 32), ReservationTTL: time.Minute,
			MaxInFlight: 8, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	environment := map[string]string{
		writeTokenEnvironment: "write-token", readTokenEnvironment: "read-token",
		otherTenantTokenEnvironment: "other-token",
	}
	lookup := func(name string) (string, bool) {
		value, exists := environment[name]
		return value, exists
	}
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(), []string{"-base-url", server.URL, "-allow-http", "-timeout", "5s"},
		lookup, bytes.NewReader(bytes.Repeat([]byte{1}, 8)), &stdout, &stderr,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var result report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != "sema.target-smoke.v1" || result.RunID == "" ||
		!result.Health || !result.Unauthenticated || !result.PermissionDenied ||
		!result.TenantIsolation || !result.LifecycleComplete {
		t.Fatalf("report = %#v", result)
	}
}

func TestRunRequiresHTTPSAndThreeDistinctTokens(t *testing.T) {
	tests := map[string]struct {
		args        []string
		environment map[string]string
		want        string
	}{
		"https": {
			args: []string{"-base-url", "http://target.example"},
			environment: map[string]string{
				writeTokenEnvironment: "write", readTokenEnvironment: "read",
				otherTenantTokenEnvironment: "other",
			},
			want: "must use HTTPS",
		},
		"missing token": {
			args: []string{"-base-url", "https://target.example"},
			environment: map[string]string{
				writeTokenEnvironment: "write", readTokenEnvironment: "read",
			},
			want: otherTenantTokenEnvironment,
		},
		"duplicate token": {
			args: []string{"-base-url", "https://target.example"},
			environment: map[string]string{
				writeTokenEnvironment: "same", readTokenEnvironment: "same",
				otherTenantTokenEnvironment: "other",
			},
			want: "must be distinct",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				value, exists := test.environment[key]
				return value, exists
			}
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), test.args, lookup, strings.NewReader(""), &stdout, &stderr); code != 2 {
				t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q; want %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunDoesNotPrintTokenWhenRemoteRejectsIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"api_version":"v0alpha2","error":{"code":"Unauthenticated"}}`))
	}))
	defer server.Close()
	secretToken := "do-not-print-this-token"
	environment := map[string]string{
		writeTokenEnvironment: secretToken, readTokenEnvironment: "read-token",
		otherTenantTokenEnvironment: "other-token",
	}
	lookup := func(name string) (string, bool) {
		value, exists := environment[name]
		return value, exists
	}
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(), []string{"-base-url", server.URL, "-allow-http"},
		lookup, bytes.NewReader(bytes.Repeat([]byte{2}, 8)), &stdout, &stderr,
	)
	if code != 1 || strings.Contains(stderr.String(), secretToken) {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestRunDoesNotForwardTokensAcrossRedirects(t *testing.T) {
	redirected := make(chan struct{}, 1)
	sink := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected <- struct{}{}
	}))
	defer sink.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/livez" || request.URL.Path == "/readyz" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(writer, request, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	environment := map[string]string{
		writeTokenEnvironment: "write-token", readTokenEnvironment: "read-token",
		otherTenantTokenEnvironment: "other-token",
	}
	lookup := func(name string) (string, bool) {
		value, exists := environment[name]
		return value, exists
	}
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(), []string{"-base-url", server.URL, "-allow-http"},
		lookup, bytes.NewReader(bytes.Repeat([]byte{3}, 8)), &stdout, &stderr,
	)
	if code != 1 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	select {
	case <-redirected:
		t.Fatal("target smoke followed a redirect")
	default:
	}
}

func TestRunPrintsVersionWithoutConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(
		context.Background(), []string{"-version"}, func(string) (string, bool) { return "", false },
		strings.NewReader(""), &stdout, &stderr,
	); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-target-smoke dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func fixtureAuthenticator() targetapi.Authenticator {
	all := map[targetapi.Permission]bool{
		targetapi.PermissionMatchTicketsRead: true, targetapi.PermissionMatchTicketsWrite: true,
		targetapi.PermissionBackfillTicketsRead: true, targetapi.PermissionBackfillTicketsWrite: true,
		targetapi.PermissionPoliciesRead: true, targetapi.PermissionPoliciesWrite: true,
		targetapi.PermissionPlanningRunsRead: true, targetapi.PermissionPlanningRunsWrite: true,
		targetapi.PermissionReservationsRead: true, targetapi.PermissionReservationsWrite: true,
		targetapi.PermissionAssignmentsRead: true, targetapi.PermissionAssignmentsWrite: true,
	}
	return targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
		switch request.Header.Get("Authorization") {
		case "Bearer write-token":
			return targetapi.Principal{Subject: "writer", Tenant: "tenant-a", Permissions: all}, nil
		case "Bearer read-token":
			return targetapi.Principal{
				Subject: "reader", Tenant: "tenant-a",
				Permissions: map[targetapi.Permission]bool{targetapi.PermissionMatchTicketsRead: true},
			}, nil
		case "Bearer other-token":
			return targetapi.Principal{
				Subject: "other", Tenant: "tenant-b",
				Permissions: map[targetapi.Permission]bool{targetapi.PermissionMatchTicketsRead: true},
			}, nil
		default:
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
	})
}
