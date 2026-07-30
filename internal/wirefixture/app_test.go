package wirefixture

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/wireconformance"
)

func TestRunServesWireConformanceOnLoopback(t *testing.T) {
	environment := fixtureEnvironment()
	address := make(chan string, 1)
	deps := dependencies{listen: func(network, requested string) (net.Listener, error) {
		listener, err := net.Listen(network, requested)
		if err == nil {
			address <- listener.Addr().String()
		}
		return listener, err
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- run(
			ctx,
			[]string{"-listen", "127.0.0.1:0"},
			mapEnvironment(environment),
			&stdout,
			&stderr,
			Identity{ProgramName: "sema-wire-fixture", Version: "test"},
			deps,
		)
	}()

	var fixtureAddress string
	select {
	case fixtureAddress = <-address:
	case code := <-exitCode:
		t.Fatalf("fixture exited before listening: code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not open its listener")
	}

	var clientStdout, clientStderr bytes.Buffer
	code := wireconformance.Run(
		context.Background(),
		[]string{"-base-url", "http://" + fixtureAddress, "-allow-http", "-timeout", "5s"},
		mapEnvironment(environment),
		&clientStdout,
		&clientStderr,
		wireconformance.Identity{
			ProgramName:  "sema-conformance",
			Version:      "test",
			ReportSchema: "sema.wire-conformance.v1",
		},
	)
	if code != 0 ||
		!strings.Contains(clientStdout.String(), `"schema":"sema.wire-conformance.v1"`) ||
		!strings.Contains(clientStdout.String(), `"lifecycle_complete":true`) {
		t.Fatalf(
			"conformance: code=%d, stdout=%q, stderr=%q",
			code,
			clientStdout.String(),
			clientStderr.String(),
		)
	}

	cancel()
	if code := <-exitCode; code != 0 {
		t.Fatalf("fixture exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsNonLoopbackAndUnsafeTokens(t *testing.T) {
	tests := map[string]struct {
		args        []string
		environment map[string]string
		want        string
	}{
		"non-loopback": {
			args:        []string{"-listen", "0.0.0.0:0"},
			environment: fixtureEnvironment(),
			want:        "loopback",
		},
		"missing token": {
			args: []string{"-listen", "127.0.0.1:0"},
			environment: map[string]string{
				wireconformance.WriteTokenEnvironment: "write",
				wireconformance.ReadTokenEnvironment:  "read",
			},
			want: wireconformance.OtherTenantTokenEnvironment,
		},
		"duplicate token": {
			args: []string{"-listen", "127.0.0.1:0"},
			environment: map[string]string{
				wireconformance.WriteTokenEnvironment:       "same",
				wireconformance.ReadTokenEnvironment:        "same",
				wireconformance.OtherTenantTokenEnvironment: "other",
			},
			want: "must be distinct",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				test.args,
				mapEnvironment(test.environment),
				&stdout,
				&stderr,
				Identity{},
			)
			if code != 2 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("code=%d, stdout=%q, stderr=%q; want %q", code, stdout.String(), stderr.String(), test.want)
			}
		})
	}
}

func TestRunPrintsVersionWithoutConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"-version"},
		func(string) (string, bool) { return "", false },
		&stdout,
		&stderr,
		Identity{ProgramName: "sema-wire-fixture", Version: "v0.3.0"},
	)
	if code != 0 || stdout.String() != "sema-wire-fixture v0.3.0\n" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAuthenticatorRequiresBearerPrefix(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", fixtureEnvironment()[wireconformance.WriteTokenEnvironment])
	if _, err := authenticator(configuration{
		writeToken:       fixtureEnvironment()[wireconformance.WriteTokenEnvironment],
		readToken:        fixtureEnvironment()[wireconformance.ReadTokenEnvironment],
		otherTenantToken: fixtureEnvironment()[wireconformance.OtherTenantTokenEnvironment],
	}).Authenticate(request); err != targetapi.ErrUnauthenticated {
		t.Fatalf("Authenticate error = %v; want %v", err, targetapi.ErrUnauthenticated)
	}
}

func fixtureEnvironment() map[string]string {
	return map[string]string{
		wireconformance.WriteTokenEnvironment:       "wire-fixture-write-token",
		wireconformance.ReadTokenEnvironment:        "wire-fixture-read-token",
		wireconformance.OtherTenantTokenEnvironment: "wire-fixture-other-token",
	}
}

func mapEnvironment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}
