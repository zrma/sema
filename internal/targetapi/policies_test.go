package targetapi

import (
	"fmt"
	"net/http"
	"testing"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestPolicyLifecycleIsImmutableTenantScopedAndHistoricallyReplayable(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	policy := matchmakingPolicy("policy-a")
	created := requestData[api.PolicyMutation](
		t, handler, "tenant-a", "register-policy-a", http.MethodPut,
		"/v0alpha2/policies/policy-a", policy, http.StatusOK,
	)
	if created.Replayed || created.Resource.Fingerprint == "" || created.Resource.StorageVersion == 0 {
		t.Fatalf("created policy = %#v", created)
	}
	registeredAgain := requestData[api.PolicyMutation](
		t, handler, "tenant-a", "register-policy-a-again", http.MethodPut,
		"/v0alpha2/policies/policy-a", policy, http.StatusOK,
	)
	if registeredAgain.Replayed || registeredAgain.Resource.Fingerprint != created.Resource.Fingerprint ||
		registeredAgain.Resource.StorageVersion <= created.Resource.StorageVersion {
		t.Fatalf("re-registered policy = %#v; created=%#v", registeredAgain, created)
	}
	replayed := requestData[api.PolicyMutation](
		t, handler, "tenant-a", "register-policy-a", http.MethodPut,
		"/v0alpha2/policies/policy-a", policy, http.StatusOK,
	)
	if !replayed.Replayed || replayed.Resource.StorageVersion != created.Resource.StorageVersion {
		t.Fatalf("historical policy replay = %#v; created=%#v", replayed, created)
	}
	current := requestData[api.PolicyResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/policies/policy-a", nil, http.StatusOK,
	)
	if current.StorageVersion != registeredAgain.Resource.StorageVersion ||
		current.Fingerprint != created.Resource.Fingerprint {
		t.Fatalf("current policy = %#v", current)
	}
	conflicting := policy
	conflicting.MaxLatencyMillis++
	requestFailure(
		t, handler, "tenant-a", "conflict-policy-a", http.MethodPut,
		"/v0alpha2/policies/policy-a", conflicting, http.StatusConflict, "PolicyConflict",
	)
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/policies/policy-a", nil, http.StatusNotFound, "NotFound",
	)
	requestFailure(
		t, handler, "reader-a", "reader-register-policy", http.MethodPut,
		"/v0alpha2/policies/reader-policy", matchmakingPolicy("reader-policy"),
		http.StatusForbidden, "PermissionDenied",
	)
}

func TestPolicyPaginationIsStableAndKindBound(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	for index, version := range []string{"policy-a", "policy-b", "policy-c"} {
		requestData[api.PolicyMutation](
			t, handler, "tenant-a", fmt.Sprintf("register-policy-%d", index), http.MethodPut,
			"/v0alpha2/policies/"+version, matchmakingPolicy(version), http.StatusOK,
		)
	}
	first := requestData[api.PolicyPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/policies?limit=2", nil, http.StatusOK,
	)
	if len(first.Items) != 2 || first.Items[0].Policy.Version != "policy-a" ||
		first.Items[1].Policy.Version != "policy-b" || first.NextCursor == "" {
		t.Fatalf("first policy page = %#v", first)
	}
	second := requestData[api.PolicyPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/policies?cursor="+first.NextCursor, nil, http.StatusOK,
	)
	if len(second.Items) != 1 || second.Items[0].Policy.Version != "policy-c" || second.NextCursor != "" {
		t.Fatalf("second policy page = %#v", second)
	}
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?cursor="+first.NextCursor, nil,
		http.StatusBadRequest, "InvalidInput",
	)
}

func matchmakingPolicy(version string) api.MatchmakingPolicy {
	return api.MatchmakingPolicy{
		Version: version, TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 100,
		MaxProposals: 8, MaxSearchNodes: 1000,
		RoleRequirements: []api.RoleRequirement{
			{Role: "front", MinPerTeam: 1, Hard: true},
		},
		RelaxationSteps: []api.RelaxationStep{
			{AfterWaitMillis: 0, MaxTeamSkillGap: 50},
			{AfterWaitMillis: 10_000, MaxTeamSkillGap: 100, PrioritizeWait: true},
		},
	}
}
