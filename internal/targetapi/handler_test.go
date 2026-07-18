package targetapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

var targetFixtureNow = time.Date(2026, time.July, 18, 3, 0, 0, 0, time.UTC)

func TestMatchTicketLifecycleReplaysHistoricalOperation(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	first := matchTicket("ticket-a", 1)
	created := requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", first, http.StatusOK,
	)
	if created.Replayed || created.Resource.StorageVersion == 0 || created.Resource.Ticket.Revision != 1 {
		t.Fatalf("created ticket = %#v", created)
	}
	second := matchTicket("ticket-a", 2)
	replaced := requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "replace-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", second, http.StatusOK,
	)
	if replaced.Resource.StorageVersion <= created.Resource.StorageVersion {
		t.Fatalf("replaced ticket = %#v; created=%#v", replaced, created)
	}

	replayed := requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", first, http.StatusOK,
	)
	if !replayed.Replayed || replayed.Resource.StorageVersion != created.Resource.StorageVersion ||
		replayed.Resource.Ticket.Revision != 1 {
		t.Fatalf("historical replay = %#v; created=%#v", replayed, created)
	}
	current := requestData[api.MatchTicketResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-a", nil, http.StatusOK,
	)
	if current.Ticket.Revision != 2 || current.StorageVersion != replaced.Resource.StorageVersion {
		t.Fatalf("current ticket = %#v", current)
	}

	conflict := matchTicket("ticket-a", 3)
	requestFailure(
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", conflict,
		http.StatusConflict, "IdempotencyConflict",
	)
	requestFailure(
		t, handler, "tenant-a", "cancel-stale", http.MethodDelete,
		"/v0alpha2/match-tickets/ticket-a?revision=1", nil,
		http.StatusConflict, "InvalidRevision",
	)
	cancelled := requestData[api.MatchTicketCancellation](
		t, handler, "tenant-a", "cancel-a", http.MethodDelete,
		"/v0alpha2/match-tickets/ticket-a?revision=2", nil, http.StatusOK,
	)
	if cancelled.Replayed || cancelled.Revision != 2 {
		t.Fatalf("cancellation = %#v", cancelled)
	}
	retriedCancel := requestData[api.MatchTicketCancellation](
		t, handler, "tenant-a", "cancel-a", http.MethodDelete,
		"/v0alpha2/match-tickets/ticket-a?revision=2", nil, http.StatusOK,
	)
	if !retriedCancel.Replayed || retriedCancel.StorageVersion != cancelled.StorageVersion {
		t.Fatalf("replayed cancellation = %#v", retriedCancel)
	}
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-a", nil, http.StatusNotFound, "NotFound",
	)
	requestFailure(
		t, handler, "tenant-a", "reuse-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", matchTicket("ticket-a", 3),
		http.StatusConflict, "InvalidRevision",
	)
}

func TestTargetAPIAuthenticatesAuthorizesAndIsolatesTenants(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	requestFailure(
		t, handler, "", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-a", nil, http.StatusUnauthorized, "Unauthenticated",
	)
	requestFailure(
		t, handler, "reader-a", "reader-create", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", matchTicket("ticket-a", 1),
		http.StatusForbidden, "PermissionDenied",
	)
	requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", matchTicket("ticket-a", 1), http.StatusOK,
	)
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-a", nil, http.StatusNotFound, "NotFound",
	)
	requestData[api.MatchTicketMutation](
		t, handler, "tenant-b", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", matchTicket("ticket-a", 1), http.StatusOK,
	)
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		resource := requestData[api.MatchTicketResource](
			t, handler, tenant, "", http.MethodGet,
			"/v0alpha2/match-tickets/ticket-a", nil, http.StatusOK,
		)
		if resource.Ticket.ID != "ticket-a" {
			t.Fatalf("%s resource = %#v", tenant, resource)
		}
	}

	unavailable, err := New(
		repository.NewMemory(),
		AuthenticatorFunc(func(*http.Request) (Principal, error) {
			return Principal{}, errors.New("provider unavailable")
		}),
		Options{CursorKey: bytes.Repeat([]byte{9}, 32), ReservationTTL: 30 * time.Second},
	)
	if err != nil {
		t.Fatal(err)
	}
	requestFailure(
		t, unavailable, "ignored", "", http.MethodGet,
		"/v0alpha2/match-tickets", nil,
		http.StatusServiceUnavailable, "AuthenticationUnavailable",
	)
}

func TestMatchTicketPaginationUsesBoundAndVersionedCursor(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	for index, id := range []string{"ticket-a", "ticket-b", "ticket-c"} {
		requestData[api.MatchTicketMutation](
			t, handler, "tenant-a", fmt.Sprintf("create-%d", index), http.MethodPut,
			"/v0alpha2/match-tickets/"+id, matchTicket(id, 1), http.StatusOK,
		)
	}
	first := requestData[api.MatchTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?limit=2", nil, http.StatusOK,
	)
	if len(first.Items) != 2 || first.Items[0].Ticket.ID != "ticket-a" ||
		first.Items[1].Ticket.ID != "ticket-b" || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second := requestData[api.MatchTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?limit=2&cursor="+first.NextCursor, nil, http.StatusOK,
	)
	if len(second.Items) != 1 || second.Items[0].Ticket.ID != "ticket-c" || second.NextCursor != "" ||
		second.RepositoryVersion != first.RepositoryVersion {
		t.Fatalf("second page = %#v; first=%#v", second, first)
	}
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/match-tickets?cursor="+first.NextCursor, nil,
		http.StatusBadRequest, "InvalidInput",
	)
	tampered := first.NextCursor[:len(first.NextCursor)-1] + "A"
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?cursor="+tampered, nil,
		http.StatusBadRequest, "InvalidInput",
	)
	requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-d", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-d", matchTicket("ticket-d", 1), http.StatusOK,
	)
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?cursor="+first.NextCursor, nil,
		http.StatusConflict, "StaleSnapshot",
	)
}

func TestMatchTicketListExcludesTombstones(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	for index, id := range []string{"ticket-a", "ticket-b"} {
		requestData[api.MatchTicketMutation](
			t, handler, "tenant-a", fmt.Sprintf("create-%d", index), http.MethodPut,
			"/v0alpha2/match-tickets/"+id, matchTicket(id, 1), http.StatusOK,
		)
	}
	requestData[api.MatchTicketCancellation](
		t, handler, "tenant-a", "cancel-a", http.MethodDelete,
		"/v0alpha2/match-tickets/ticket-a?revision=1", nil, http.StatusOK,
	)
	page := requestData[api.MatchTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets", nil, http.StatusOK,
	)
	if len(page.Items) != 1 || page.Items[0].Ticket.ID != "ticket-b" {
		t.Fatalf("active page = %#v", page)
	}
}

func TestTargetAPIRejectsAmbiguousInput(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	requestFailure(
		t, handler, "tenant-a", "", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", matchTicket("ticket-a", 1),
		http.StatusBadRequest, "InvalidInput",
	)
	requestFailureRaw(
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", `{"id":"ticket-a"}`,
		"text/plain", http.StatusUnsupportedMediaType, "UnsupportedMediaType",
	)
	requestFailureRaw(
		t, handler, "tenant-a", "create-a", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-a", `{"id":"ticket-a","tenant":"tenant-b"}`,
		"application/json", http.StatusBadRequest, "InvalidInput",
	)
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?tenant=tenant-b", nil,
		http.StatusBadRequest, "InvalidInput",
	)
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?limit=1&limit=2", nil,
		http.StatusBadRequest, "InvalidInput",
	)
}

func TestCursorBindingIncludesFilterAndOrder(t *testing.T) {
	codec, err := newCursorCodec(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	binding := cursorBinding{
		Tenant: "tenant-a", ResourceKind: "match_ticket", Filter: "active", Order: "resource_id.asc",
	}
	token, err := codec.encode(binding, cursorPosition{RepositoryVersion: 9, After: "ticket-a"})
	if err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string]cursorBinding{
		"tenant": {Tenant: "tenant-b", ResourceKind: "match_ticket", Filter: "active", Order: "resource_id.asc"},
		"kind":   {Tenant: "tenant-a", ResourceKind: "backfill_ticket", Filter: "active", Order: "resource_id.asc"},
		"filter": {Tenant: "tenant-a", ResourceKind: "match_ticket", Filter: "all", Order: "resource_id.asc"},
		"order":  {Tenant: "tenant-a", ResourceKind: "match_ticket", Filter: "active", Order: "resource_id.desc"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := codec.decode(token, changed); err == nil {
				t.Fatal("cursor decoded with a changed binding")
			}
		})
	}
}

func TestPrincipalIdentityRemainsProviderOpaque(t *testing.T) {
	for _, value := range []string{"issuer|user@example.com", "tenant/customer-a", "urn:tenant:42"} {
		if !validPrincipalValue(value) {
			t.Fatalf("provider principal value %q was rejected", value)
		}
	}
	for _, value := range []string{"", " tenant-a", "tenant-a\n"} {
		if validPrincipalValue(value) {
			t.Fatalf("invalid principal value %q was accepted", value)
		}
	}
}

func newTestHandler(t *testing.T, owner repository.Repository) http.Handler {
	t.Helper()
	handler, err := New(owner, fixtureAuthenticator(), Options{
		Now:            func() time.Time { return targetFixtureNow },
		CursorKey:      bytes.Repeat([]byte{7}, 32),
		ReservationTTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func fixtureAuthenticator() Authenticator {
	return AuthenticatorFunc(func(request *http.Request) (Principal, error) {
		switch request.Header.Get("Authorization") {
		case "Test tenant-a":
			return fixturePrincipal("tenant-a", true), nil
		case "Test tenant-b":
			return fixturePrincipal("tenant-b", true), nil
		case "Test reader-a":
			return fixturePrincipal("tenant-a", false), nil
		default:
			return Principal{}, ErrUnauthenticated
		}
	})
}

func fixturePrincipal(tenant string, write bool) Principal {
	permissions := map[Permission]bool{
		PermissionMatchTicketsRead: true, PermissionBackfillTicketsRead: true,
		PermissionPoliciesRead: true, PermissionPlanningRunsRead: true,
		PermissionReservationsRead: true,
		PermissionAssignmentsRead:  true,
	}
	if write {
		permissions[PermissionMatchTicketsWrite] = true
		permissions[PermissionBackfillTicketsWrite] = true
		permissions[PermissionPoliciesWrite] = true
		permissions[PermissionPlanningRunsWrite] = true
		permissions[PermissionReservationsWrite] = true
		permissions[PermissionAssignmentsWrite] = true
	}
	return Principal{Subject: "subject-" + tenant, Tenant: tenant, Permissions: permissions}
}

func matchTicket(id string, revision uint64) api.MatchTicket {
	return api.MatchTicket{
		ID: id, Revision: revision, EnqueuedAt: targetFixtureNow,
		Players: []api.Player{{ID: "player-" + id, Skill: 1500, Role: "flex", LatencyMillis: 30}},
	}
}

func requestData[T any](
	t *testing.T,
	handler http.Handler,
	credential string,
	operationID string,
	method string,
	path string,
	body any,
	wantStatus int,
) T {
	t.Helper()
	response := performRequest(t, handler, credential, operationID, method, path, body, "application/json")
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d body=%s; want %d", method, path, response.Code, response.Body.String(), wantStatus)
	}
	var envelope struct {
		APIVersion string          `json:"api_version"`
		Data       json.RawMessage `json:"data"`
		Error      *api.Failure    `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.APIVersion != api.Version || envelope.Error != nil {
		t.Fatalf("response envelope = %#v", envelope)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func requestFailure(
	t *testing.T,
	handler http.Handler,
	credential string,
	operationID string,
	method string,
	path string,
	body any,
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	response := performRequest(t, handler, credential, operationID, method, path, body, "application/json")
	assertFailure(t, response, wantStatus, wantCode)
}

func requestFailureRaw(
	t *testing.T,
	handler http.Handler,
	credential string,
	operationID string,
	method string,
	path string,
	body string,
	contentType string,
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if credential != "" {
		request.Header.Set("Authorization", "Test "+credential)
	}
	if operationID != "" {
		request.Header.Set("Idempotency-Key", operationID)
	}
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertFailure(t, response, wantStatus, wantCode)
}

func performRequest(
	t *testing.T,
	handler http.Handler,
	credential string,
	operationID string,
	method string,
	path string,
	body any,
	contentType string,
) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if credential != "" {
		request.Header.Set("Authorization", "Test "+credential)
	}
	if operationID != "" {
		request.Header.Set("Idempotency-Key", operationID)
	}
	if body != nil {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertFailure(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d body=%s; want %d", response.Code, response.Body.String(), wantStatus)
	}
	var envelope api.Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.APIVersion != api.Version || envelope.Error == nil || envelope.Error.Code != wantCode {
		t.Fatalf("failure envelope = %#v; want code %s", envelope, wantCode)
	}
	if response.Header().Get("X-Sema-Error-Code") != wantCode {
		t.Fatalf("error header = %q; want %q", response.Header().Get("X-Sema-Error-Code"), wantCode)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", response.Header().Get("Cache-Control"))
	}
}
