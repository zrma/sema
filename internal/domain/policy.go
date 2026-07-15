package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
)

// FingerprintPolicy binds a validated policy version to all rule content.
func FingerprintPolicy(policy MatchmakingPolicy) (PolicyFingerprint, error) {
	if err := ValidatePolicy(policy); err != nil {
		return "", err
	}

	encoded := make([]byte, 0, 256)
	encoded = appendCanonicalString(encoded, policy.Version)
	encoded = appendCanonicalInt(encoded, policy.TeamCount)
	encoded = appendCanonicalInt(encoded, policy.TeamSize)
	encoded = appendCanonicalInt(encoded, policy.MaxLatencyMillis)
	encoded = appendCanonicalInt(encoded, policy.MaxProposals)
	encoded = appendCanonicalInt(encoded, policy.MaxSearchNodes)
	encoded = appendCanonicalInt(encoded, policy.MaxCandidatesPerProposal)

	requirements := slices.Clone(policy.RoleRequirements)
	slices.SortFunc(requirements, func(left, right RoleRequirement) int {
		if left.Role < right.Role {
			return -1
		}
		if left.Role > right.Role {
			return 1
		}
		return 0
	})
	encoded = appendCanonicalInt(encoded, len(requirements))
	for _, requirement := range requirements {
		encoded = appendCanonicalString(encoded, requirement.Role)
		encoded = appendCanonicalInt(encoded, requirement.MinPerTeam)
		encoded = appendCanonicalBool(encoded, requirement.Hard)
	}

	encoded = appendCanonicalInt(encoded, len(policy.RelaxationSteps))
	for _, step := range policy.RelaxationSteps {
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(step.AfterWait))
		encoded = appendCanonicalInt(encoded, step.MaxTeamSkillGap)
		encoded = appendCanonicalInt(encoded, step.MaxRolePenalty)
		encoded = appendCanonicalBool(encoded, step.PrioritizeWait)
	}

	digest := sha256.Sum256(encoded)
	return PolicyFingerprint(hex.EncodeToString(digest[:])), nil
}

func appendCanonicalString(encoded []byte, value string) []byte {
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func appendCanonicalInt(encoded []byte, value int) []byte {
	return binary.BigEndian.AppendUint64(encoded, uint64(value))
}

func appendCanonicalBool(encoded []byte, value bool) []byte {
	if value {
		return append(encoded, 1)
	}
	return append(encoded, 0)
}
