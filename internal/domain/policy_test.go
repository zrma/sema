package domain_test

import (
	"testing"
	"time"

	"sema/internal/domain"
)

func TestFingerprintPolicyCanonicalizesRoleRequirementOrder(t *testing.T) {
	first := fingerprintPolicy()
	second := first
	second.RoleRequirements = []domain.RoleRequirement{
		first.RoleRequirements[1],
		first.RoleRequirements[0],
	}

	firstFingerprint, err := domain.FingerprintPolicy(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := domain.FingerprintPolicy(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint {
		t.Fatalf("role requirement order changed fingerprint: %q != %q", firstFingerprint, secondFingerprint)
	}
}

func TestFingerprintPolicyChangesWithVersionedRuleContent(t *testing.T) {
	first := fingerprintPolicy()
	second := first
	second.MaxLatencyMillis++

	firstFingerprint, err := domain.FingerprintPolicy(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := domain.FingerprintPolicy(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatalf("different rule content reused fingerprint %q", firstFingerprint)
	}
}

func fingerprintPolicy() domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  "policy-content-v1",
		TeamCount:                2,
		TeamSize:                 3,
		MaxLatencyMillis:         200,
		MaxProposals:             8,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
		RoleRequirements: []domain.RoleRequirement{
			{Role: "tank", MinPerTeam: 1, Hard: true},
			{Role: "healer", MinPerTeam: 1},
		},
		RelaxationSteps: []domain.RelaxationStep{
			{AfterWait: 0, MaxTeamSkillGap: 50, MaxRolePenalty: 0},
			{AfterWait: 30 * time.Second, MaxTeamSkillGap: 200, MaxRolePenalty: 2, PrioritizeWait: true},
		},
	}
}
