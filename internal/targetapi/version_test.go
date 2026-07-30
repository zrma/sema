package targetapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestStableAndLegacyRoutesShareStateWithDistinctMarkers(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	ticket := matchTicket("ticket-versioned", 1)

	created := performRequest(
		t,
		handler,
		"tenant-a",
		"create-versioned",
		http.MethodPut,
		"/v1/match-tickets/ticket-versioned",
		ticket,
		"application/json",
	)
	assertVersionedSuccess(t, created, StableAPIVersion)

	current := performRequest(
		t,
		handler,
		"tenant-a",
		"",
		http.MethodGet,
		"/v0alpha2/match-tickets/ticket-versioned",
		nil,
		"application/json",
	)
	assertVersionedSuccess(t, current, api.Version)

	unauthenticated := performRequest(
		t,
		handler,
		"",
		"",
		http.MethodGet,
		"/v1/match-tickets/ticket-versioned",
		nil,
		"application/json",
	)
	assertVersionedFailure(t, unauthenticated, http.StatusUnauthorized, "Unauthenticated", StableAPIVersion)

	notFound := performRequest(
		t,
		handler,
		"tenant-a",
		"",
		http.MethodGet,
		"/v1/unknown",
		nil,
		"application/json",
	)
	assertVersionedFailure(t, notFound, http.StatusNotFound, "NotFound", StableAPIVersion)
}

func assertVersionedSuccess(
	t *testing.T,
	response *httptest.ResponseRecorder,
	expectedVersion string,
) {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", result.StatusCode)
	}
	var envelope api.Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.APIVersion != expectedVersion || envelope.Error != nil {
		t.Fatalf("response envelope = %#v; want version %s", envelope, expectedVersion)
	}
}

func assertVersionedFailure(
	t *testing.T,
	response *httptest.ResponseRecorder,
	expectedStatus int,
	expectedCode string,
	expectedVersion string,
) {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != expectedStatus {
		t.Fatalf("status = %d; want %d", result.StatusCode, expectedStatus)
	}
	var envelope api.Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.APIVersion != expectedVersion ||
		envelope.Error == nil ||
		envelope.Error.Code != expectedCode {
		t.Fatalf(
			"response envelope = %#v; want version %s code %s",
			envelope,
			expectedVersion,
			expectedCode,
		)
	}
}
