package policy_test

import (
	"sync"
	"testing"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/policy"
)

func TestCatalogRegistrationIsIdempotentAndDefensive(t *testing.T) {
	catalog := policy.NewCatalog()
	candidate := catalogPolicy()
	first, err := catalog.Register(candidate)
	if err != nil {
		t.Fatal(err)
	}

	reordered := candidate
	reordered.RoleRequirements = []domain.RoleRequirement{
		candidate.RoleRequirements[1],
		candidate.RoleRequirements[0],
	}
	second, err := catalog.Register(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("idempotent registration changed fingerprint: %q != %q", first.Fingerprint, second.Fingerprint)
	}

	candidate.RoleRequirements[0].Role = "mutated-caller"
	first.Policy.RoleRequirements[0].Role = "mutated-result"
	stored, exists := catalog.Get("catalog-v1")
	if !exists {
		t.Fatal("registered policy is missing")
	}
	if stored.Policy.RoleRequirements[0].Role != "healer" {
		t.Fatalf("caller mutation leaked into catalog: %#v", stored.Policy.RoleRequirements)
	}
	stored.Policy.RoleRequirements[0].Role = "mutated-read"
	again, _ := catalog.Get("catalog-v1")
	if again.Policy.RoleRequirements[0].Role != "healer" {
		t.Fatal("read result mutation leaked into catalog")
	}
}

func TestCatalogRejectsVersionContentConflictWithoutReplacingPolicy(t *testing.T) {
	catalog := policy.NewCatalog()
	original := catalogPolicy()
	registered, err := catalog.Register(original)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := original
	conflicting.MaxLatencyMillis++
	_, err = catalog.Register(conflicting)
	code, ok := domain.FailureCodeOf(err)
	if !ok || code != domain.FailurePolicyConflict {
		t.Fatalf("conflict error = %v; want %s", err, domain.FailurePolicyConflict)
	}
	stored, _ := catalog.Get(original.Version)
	if stored.Fingerprint != registered.Fingerprint || stored.Policy.MaxLatencyMillis != original.MaxLatencyMillis {
		t.Fatalf("conflict replaced registered policy: %#v", stored)
	}
}

func TestCatalogConcurrentRegistrationChoosesOneVersionContent(t *testing.T) {
	catalog := policy.NewCatalog()
	first := catalogPolicy()
	second := first
	second.MaxLatencyMillis++
	candidates := []domain.MatchmakingPolicy{first, second}

	start := make(chan struct{})
	results := make(chan error, len(candidates))
	var wait sync.WaitGroup
	for _, candidate := range candidates {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := catalog.Register(candidate)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if code, ok := domain.FailureCodeOf(err); ok && code == domain.FailurePolicyConflict {
			conflicts++
			continue
		}
		t.Fatalf("unexpected registration result: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1, 1", successes, conflicts)
	}
	if _, exists := catalog.Get(first.Version); !exists {
		t.Fatal("concurrent registration did not retain a policy")
	}
}

func catalogPolicy() domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  "catalog-v1",
		TeamCount:                2,
		TeamSize:                 2,
		MaxLatencyMillis:         200,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
		RoleRequirements: []domain.RoleRequirement{
			{Role: "healer", MinPerTeam: 1},
			{Role: "tank", MinPerTeam: 1},
		},
	}
}
