package targetapi

import (
	"fmt"
	"net/http"
	"testing"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestPlanningRunCapturesImmutableDemandAndPagesResults(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	policy := matchmakingPolicy("planning-policy")
	policy.RoleRequirements = nil
	policy.MaxProposals = 2
	policy.MaxSearchNodes = 100_000
	requestData[api.PolicyMutation](
		t, handler, "tenant-a", "register-planning-policy", http.MethodPut,
		"/v0alpha2/policies/planning-policy", policy, http.StatusOK,
	)
	for index := 0; index < 10; index++ {
		id := fmt.Sprintf("planning-ticket-%02d", index)
		requestData[api.MatchTicketMutation](
			t, handler, "tenant-a", "create-"+id, http.MethodPut,
			"/v0alpha2/match-tickets/"+id, matchTicket(id, 1), http.StatusOK,
		)
	}
	created := requestData[api.PlanningRunMutation](
		t, handler, "tenant-a", "execute-planning-run", http.MethodPost,
		"/v0alpha2/planning-runs/run-a", api.PlanningRunRequest{PolicyVersion: "planning-policy"},
		http.StatusOK,
	)
	if created.Replayed || created.Resource.Status != "completed" ||
		created.Resource.ProposalCount != 2 || created.Resource.UnmatchedCount != 2 ||
		created.Resource.PolicyFingerprint == "" {
		t.Fatalf("created planning run = %#v", created)
	}
	polled := requestData[api.PlanningRunResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a", nil, http.StatusOK,
	)
	if polled.StorageVersion != created.Resource.StorageVersion || polled.Status != "completed" {
		t.Fatalf("polled planning run = %#v; created=%#v", polled, created)
	}
	firstProposals := requestData[api.ProposalPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a/proposals?limit=1", nil, http.StatusOK,
	)
	if len(firstProposals.Items) != 1 || firstProposals.NextCursor == "" {
		t.Fatalf("first proposal page = %#v", firstProposals)
	}
	requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-late-ticket", http.MethodPut,
		"/v0alpha2/match-tickets/late-ticket", matchTicket("late-ticket", 1), http.StatusOK,
	)
	secondProposals := requestData[api.ProposalPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a/proposals?limit=1&cursor="+firstProposals.NextCursor,
		nil, http.StatusOK,
	)
	if len(secondProposals.Items) != 1 || secondProposals.NextCursor != "" ||
		secondProposals.RunStorageVersion != firstProposals.RunStorageVersion {
		t.Fatalf("second proposal page = %#v; first=%#v", secondProposals, firstProposals)
	}
	firstUnmatched := requestData[api.UnmatchedPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a/unmatched?limit=1", nil, http.StatusOK,
	)
	if len(firstUnmatched.Items) != 1 || firstUnmatched.NextCursor == "" {
		t.Fatalf("first unmatched page = %#v", firstUnmatched)
	}
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a/proposals?cursor="+firstUnmatched.NextCursor,
		nil, http.StatusBadRequest, "InvalidInput",
	)
	replayed := requestData[api.PlanningRunMutation](
		t, handler, "tenant-a", "execute-planning-run", http.MethodPost,
		"/v0alpha2/planning-runs/run-a", api.PlanningRunRequest{PolicyVersion: "planning-policy"},
		http.StatusOK,
	)
	if !replayed.Replayed || replayed.Resource.StorageVersion != created.Resource.StorageVersion {
		t.Fatalf("historical planning replay = %#v; created=%#v", replayed, created)
	}
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/planning-runs/run-a", nil, http.StatusNotFound, "NotFound",
	)
	requestFailure(
		t, handler, "reader-a", "reader-plan", http.MethodPost,
		"/v0alpha2/planning-runs/reader-run", api.PlanningRunRequest{PolicyVersion: "planning-policy"},
		http.StatusForbidden, "PermissionDenied",
	)
}

func TestPlanningRunRejectsUnknownPolicyAndDuplicateIdentity(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	requestFailure(
		t, handler, "tenant-a", "missing-policy-run", http.MethodPost,
		"/v0alpha2/planning-runs/missing-policy-run", api.PlanningRunRequest{PolicyVersion: "missing"},
		http.StatusBadRequest, "InvalidInput",
	)
	policy := matchmakingPolicy("planning-policy")
	policy.RoleRequirements = nil
	requestData[api.PolicyMutation](
		t, handler, "tenant-a", "register-planning-policy", http.MethodPut,
		"/v0alpha2/policies/planning-policy", policy, http.StatusOK,
	)
	requestData[api.PlanningRunMutation](
		t, handler, "tenant-a", "create-empty-run", http.MethodPost,
		"/v0alpha2/planning-runs/empty-run", api.PlanningRunRequest{PolicyVersion: "planning-policy"},
		http.StatusOK,
	)
	requestFailure(
		t, handler, "tenant-a", "different-empty-run-command", http.MethodPost,
		"/v0alpha2/planning-runs/empty-run", api.PlanningRunRequest{PolicyVersion: "planning-policy"},
		http.StatusConflict, "InvalidRevision",
	)
}
