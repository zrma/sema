package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const (
	policyPayloadSchema     = "sema.policy.v1"
	maxPolicyCommitAttempts = 8
)

type PolicyRecord struct {
	Policy         domain.MatchmakingPolicy
	Fingerprint    domain.PolicyFingerprint
	StorageVersion repository.Version
}

type PolicySnapshot struct {
	RepositoryVersion repository.Version
	Records           []PolicyRecord
}

type PolicyMutation struct {
	Policy         domain.MatchmakingPolicy
	Fingerprint    domain.PolicyFingerprint
	StorageVersion repository.Version
	Replayed       bool
}

// Policies owns the tenant-scoped immutable version-to-content contract.
// Re-registering identical content records a new operation receipt without
// changing the canonical policy payload.
type Policies struct {
	repository repository.Repository
	now        func() time.Time
}

func NewPolicies(owner repository.Repository, now func() time.Time) (*Policies, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Policies{repository: owner, now: now}, nil
}

func (service *Policies) Put(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	policy domain.MatchmakingPolicy,
) (PolicyMutation, error) {
	fingerprint, err := domain.FingerprintPolicy(policy)
	if err != nil {
		return PolicyMutation{}, err
	}
	policy = canonicalPolicy(policy)
	payload, err := encodePolicy(policy, fingerprint)
	if err != nil {
		return PolicyMutation{}, err
	}
	operation := repository.Operation{
		Scope: scope, ID: operationID, Kind: "policy.put",
		Digest: repository.Digest(append([]byte("put\x00"), payload...)), At: service.now().UTC(),
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return PolicyMutation{}, err
	}
	if replayed, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return PolicyMutation{}, err
	} else if exists {
		return PolicyMutation{
			Policy: policy, Fingerprint: fingerprint,
			StorageVersion: replayed.Version, Replayed: true,
		}, nil
	}

	key := Key(scope, ResourcePolicy, policy.Version)
	for attempt := 0; attempt < maxPolicyCommitAttempts; attempt++ {
		snapshot, err := service.repository.Snapshot(ctx, scope)
		if err != nil {
			return PolicyMutation{}, err
		}
		expected := repository.Version(0)
		if current, exists := findResource(snapshot, key); exists {
			if current.Deleted {
				return PolicyMutation{}, fmt.Errorf("policy %q has an unexpected tombstone", policy.Version)
			}
			_, storedFingerprint, err := decodePolicy(current.Payload)
			if err != nil {
				return PolicyMutation{}, err
			}
			if storedFingerprint != fingerprint {
				return PolicyMutation{}, domain.NewFailure(
					domain.FailurePolicyConflict,
					"policy version %q is already registered with different content",
					policy.Version,
				)
			}
			expected = current.Version
		}
		result, err := service.repository.Commit(ctx, operation, []repository.Mutation{{
			Key: key, ExpectedVersion: expected, Payload: payload,
		}})
		if err == nil {
			return PolicyMutation{
				Policy: policy, Fingerprint: fingerprint,
				StorageVersion: result.Version, Replayed: result.Replayed,
			}, nil
		}
		if !repository.IsConflict(err) {
			return PolicyMutation{}, err
		}
	}
	return PolicyMutation{}, domain.NewFailure(
		domain.FailureStaleSnapshot,
		"policy version %q changed repeatedly while registration was committed",
		policy.Version,
	)
}

func (service *Policies) Get(
	ctx context.Context,
	scope string,
	version string,
) (PolicyRecord, bool, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourcePolicy, version))
	if !exists || resource.Deleted {
		return PolicyRecord{}, false, nil
	}
	policy, fingerprint, err := decodePolicy(resource.Payload)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	return PolicyRecord{
		Policy: policy, Fingerprint: fingerprint, StorageVersion: resource.Version,
	}, true, nil
}

func (service *Policies) Snapshot(ctx context.Context, scope string) (PolicySnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return PolicySnapshot{}, err
	}
	records := make([]PolicyRecord, 0)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourcePolicy) || resource.Deleted {
			continue
		}
		policy, fingerprint, err := decodePolicy(resource.Payload)
		if err != nil {
			return PolicySnapshot{}, err
		}
		records = append(records, PolicyRecord{
			Policy: policy, Fingerprint: fingerprint, StorageVersion: resource.Version,
		})
	}
	slices.SortFunc(records, func(left, right PolicyRecord) int {
		if left.Policy.Version < right.Policy.Version {
			return -1
		}
		if left.Policy.Version > right.Policy.Version {
			return 1
		}
		return 0
	})
	return PolicySnapshot{RepositoryVersion: snapshot.Version, Records: records}, nil
}

type persistedPolicy struct {
	Schema                   string                     `json:"schema"`
	Fingerprint              domain.PolicyFingerprint   `json:"fingerprint"`
	Version                  string                     `json:"version"`
	TeamCount                int                        `json:"team_count"`
	TeamSize                 int                        `json:"team_size"`
	MaxLatencyMillis         int                        `json:"max_latency_millis"`
	MaxProposals             int                        `json:"max_proposals"`
	MaxSearchNodes           int                        `json:"max_search_nodes"`
	MaxCandidateTickets      int                        `json:"max_candidate_tickets"`
	MaxCandidatesPerProposal int                        `json:"max_candidates_per_proposal"`
	MaxBatchCandidates       int                        `json:"max_batch_candidates"`
	MaxBatchSearchNodes      int                        `json:"max_batch_search_nodes"`
	RoleRequirements         []persistedRoleRequirement `json:"role_requirements,omitempty"`
	RelaxationSteps          []persistedRelaxationStep  `json:"relaxation_steps,omitempty"`
}

type persistedRoleRequirement struct {
	Role       string `json:"role"`
	MinPerTeam int    `json:"min_per_team"`
	Hard       bool   `json:"hard"`
}

type persistedRelaxationStep struct {
	AfterWaitNanos  int64 `json:"after_wait_nanos"`
	MaxTeamSkillGap int   `json:"max_team_skill_gap"`
	MaxRolePenalty  int   `json:"max_role_penalty"`
	PrioritizeWait  bool  `json:"prioritize_wait"`
}

func encodePolicy(policy domain.MatchmakingPolicy, fingerprint domain.PolicyFingerprint) ([]byte, error) {
	requirements := make([]persistedRoleRequirement, len(policy.RoleRequirements))
	for index, requirement := range policy.RoleRequirements {
		requirements[index] = persistedRoleRequirement(requirement)
	}
	steps := make([]persistedRelaxationStep, len(policy.RelaxationSteps))
	for index, step := range policy.RelaxationSteps {
		steps[index] = persistedRelaxationStep{
			AfterWaitNanos: int64(step.AfterWait), MaxTeamSkillGap: step.MaxTeamSkillGap,
			MaxRolePenalty: step.MaxRolePenalty, PrioritizeWait: step.PrioritizeWait,
		}
	}
	encoded, err := json.Marshal(persistedPolicy{
		Schema: policyPayloadSchema, Fingerprint: fingerprint, Version: policy.Version,
		TeamCount: policy.TeamCount, TeamSize: policy.TeamSize, MaxLatencyMillis: policy.MaxLatencyMillis,
		MaxProposals: policy.MaxProposals, MaxSearchNodes: policy.MaxSearchNodes,
		MaxCandidateTickets:      policy.MaxCandidateTickets,
		MaxCandidatesPerProposal: policy.MaxCandidatesPerProposal,
		MaxBatchCandidates:       policy.MaxBatchCandidates, MaxBatchSearchNodes: policy.MaxBatchSearchNodes,
		RoleRequirements: requirements, RelaxationSteps: steps,
	})
	if err != nil {
		return nil, fmt.Errorf("encode policy resource: %w", err)
	}
	return encoded, nil
}

func decodePolicy(payload []byte) (domain.MatchmakingPolicy, domain.PolicyFingerprint, error) {
	var stored persistedPolicy
	if err := decodeStrict(payload, &stored); err != nil {
		return domain.MatchmakingPolicy{}, "", fmt.Errorf("decode policy resource: %w", err)
	}
	if stored.Schema != policyPayloadSchema {
		return domain.MatchmakingPolicy{}, "", fmt.Errorf("unsupported policy resource schema %q", stored.Schema)
	}
	requirements := make([]domain.RoleRequirement, len(stored.RoleRequirements))
	for index, requirement := range stored.RoleRequirements {
		requirements[index] = domain.RoleRequirement(requirement)
	}
	steps := make([]domain.RelaxationStep, len(stored.RelaxationSteps))
	for index, step := range stored.RelaxationSteps {
		steps[index] = domain.RelaxationStep{
			AfterWait: time.Duration(step.AfterWaitNanos), MaxTeamSkillGap: step.MaxTeamSkillGap,
			MaxRolePenalty: step.MaxRolePenalty, PrioritizeWait: step.PrioritizeWait,
		}
	}
	policy := domain.MatchmakingPolicy{
		Version: stored.Version, TeamCount: stored.TeamCount, TeamSize: stored.TeamSize,
		MaxLatencyMillis: stored.MaxLatencyMillis, MaxProposals: stored.MaxProposals,
		MaxSearchNodes: stored.MaxSearchNodes, MaxCandidateTickets: stored.MaxCandidateTickets,
		MaxCandidatesPerProposal: stored.MaxCandidatesPerProposal,
		MaxBatchCandidates:       stored.MaxBatchCandidates, MaxBatchSearchNodes: stored.MaxBatchSearchNodes,
		RoleRequirements: requirements, RelaxationSteps: steps,
	}
	fingerprint, err := domain.FingerprintPolicy(policy)
	if err != nil {
		return domain.MatchmakingPolicy{}, "", fmt.Errorf("stored policy is invalid: %w", err)
	}
	if stored.Fingerprint == "" || stored.Fingerprint != fingerprint {
		return domain.MatchmakingPolicy{}, "", fmt.Errorf("stored policy fingerprint does not match content")
	}
	return policy, fingerprint, nil
}

func canonicalPolicy(policy domain.MatchmakingPolicy) domain.MatchmakingPolicy {
	policy = domain.ClonePolicy(policy)
	slices.SortFunc(policy.RoleRequirements, func(left, right domain.RoleRequirement) int {
		if left.Role < right.Role {
			return -1
		}
		if left.Role > right.Role {
			return 1
		}
		return 0
	})
	return policy
}
