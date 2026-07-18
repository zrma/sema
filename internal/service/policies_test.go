package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

func TestPolicyRegistrationSerializesVersionContentCompetition(t *testing.T) {
	backend := repository.NewMemoryBackend()
	left, err := service.NewPolicies(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	right, err := service.NewPolicies(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	policies := []domain.MatchmakingPolicy{servicePolicy("policy-shared"), servicePolicy("policy-shared")}
	policies[1].MaxLatencyMillis++
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for index, owner := range []*service.Policies{left, right} {
		index, owner := index, owner
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, putErr := owner.Put(
				context.Background(), "tenant-a", domain.OperationID("register-policy-"+string(rune('a'+index))),
				policies[index],
			)
			results <- putErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		if failureCode(result) == domain.FailurePolicyConflict {
			conflicts++
			continue
		}
		t.Fatalf("policy competition error = %v", result)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("policy competition successes=%d conflicts=%d; want 1/1", successes, conflicts)
	}
}

func TestIdenticalConcurrentPolicyRegistrationsBothReceiveDurableReceipts(t *testing.T) {
	backend := repository.NewMemoryBackend()
	owners := make([]*service.Policies, 2)
	for index := range owners {
		var err error
		owners[index], err = service.NewPolicies(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	versions := make(chan repository.Version, 2)
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for index, owner := range owners {
		index, owner := index, owner
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, putErr := owner.Put(
				context.Background(), "tenant-a", domain.OperationID("register-identical-"+string(rune('a'+index))),
				servicePolicy("policy-identical"),
			)
			if putErr == nil {
				versions <- result.StorageVersion
			}
			errors <- putErr
		}()
	}
	close(start)
	group.Wait()
	close(versions)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("identical policy registration: %v", err)
		}
	}
	seen := make(map[repository.Version]bool)
	for version := range versions {
		seen[version] = true
	}
	if len(seen) != 2 {
		t.Fatalf("durable policy receipt versions = %#v; want two commits", seen)
	}
}

func servicePolicy(version string) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version: version, TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 100,
		MaxProposals: 8, MaxSearchNodes: 1000,
		RoleRequirements: []domain.RoleRequirement{
			{Role: "front", MinPerTeam: 1, Hard: true},
		},
		RelaxationSteps: []domain.RelaxationStep{
			{AfterWait: 0, MaxTeamSkillGap: 50},
			{AfterWait: 10 * time.Second, MaxTeamSkillGap: 100, PrioritizeWait: true},
		},
	}
}
