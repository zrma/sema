// Package objective evaluates and compares versioned matchmaking policy outcomes.
package objective

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/zrma/sema/internal/constraint"
	"github.com/zrma/sema/internal/domain"
)

// Evaluation contains an admissibility decision and its replayable evidence.
type Evaluation struct {
	Admissible         bool
	HardViolation      bool
	PriorityWaitMillis []int64
	Evidence           domain.ScoreEvidence
}

// Evaluate calculates hard violations, active relaxation, and soft evidence.
func Evaluate(
	now time.Time,
	teams [][]domain.MatchTicket,
	policy domain.MatchmakingPolicy,
	kind domain.ProposalKind,
) Evaluation {
	return evaluate(now, teams, policy, kind, nil)
}

// EvaluateBackfill evaluates incoming placement against a roster-versioned team summary.
func EvaluateBackfill(
	now time.Time,
	teams [][]domain.MatchTicket,
	policy domain.MatchmakingPolicy,
	existing []domain.RosterTeamSummary,
	backfillEnqueuedAt time.Time,
) Evaluation {
	evaluation := evaluate(now, teams, policy, domain.ProposalBackfill, existing)
	priority, waitMillis := TicketWaitPriority(now, backfillEnqueuedAt, policy)
	evaluation.Evidence.OldestWaitMillis = max(evaluation.Evidence.OldestWaitMillis, waitMillis)
	evaluation.Evidence.TotalWaitMillis += waitMillis
	if priority {
		evaluation.Evidence.WaitPriority = true
		evaluation.PriorityWaitMillis = append(evaluation.PriorityWaitMillis, waitMillis)
		slices.SortFunc(evaluation.PriorityWaitMillis, func(left, right int64) int {
			return cmp.Compare(right, left)
		})
	}
	return evaluation
}

func evaluate(
	now time.Time,
	teams [][]domain.MatchTicket,
	policy domain.MatchmakingPolicy,
	kind domain.ProposalKind,
	existing []domain.RosterTeamSummary,
) Evaluation {
	evaluation := Evaluation{Admissible: true, HardViolation: constraint.HardViolation(teams, policy, kind)}
	var roleCounts []map[string]int
	qualityContext := kind == domain.ProposalNewMatch || len(existing) > 0
	if qualityContext && len(policy.RoleRequirements) > 0 {
		roleCounts = make([]map[string]int, len(teams))
	}
	minimumAverage, maximumAverage := 0, 0
	hasAverage := false
	hasWaitPriority := slices.ContainsFunc(policy.RelaxationSteps, func(step domain.RelaxationStep) bool {
		return step.PrioritizeWait
	})

	for team, tickets := range teams {
		teamSkill, teamPlayers := 0, 0
		if len(existing) > 0 {
			teamSkill = existing[team].SkillTotal
			teamPlayers = existing[team].PlayerCount
			evaluation.Evidence.MaxLatencyMillis = max(evaluation.Evidence.MaxLatencyMillis, existing[team].MaxLatencyMillis)
		}
		if roleCounts != nil {
			roleCounts[team] = make(map[string]int)
			if len(existing) > 0 {
				for _, role := range existing[team].RoleCounts {
					roleCounts[team][role.Role] = role.Count
				}
			}
		}
		for _, ticket := range tickets {
			waitMillis := max(int64(0), now.Sub(ticket.EnqueuedAt).Milliseconds())
			if hasWaitPriority {
				if priority, _ := TicketWaitPriority(now, ticket.EnqueuedAt, policy); priority {
					evaluation.PriorityWaitMillis = append(evaluation.PriorityWaitMillis, waitMillis)
				}
			}
			if waitMillis > evaluation.Evidence.OldestWaitMillis {
				evaluation.Evidence.OldestWaitMillis = waitMillis
			}
			evaluation.Evidence.TotalWaitMillis += waitMillis
			for _, player := range ticket.Players {
				teamSkill += player.Skill
				teamPlayers++
				if roleCounts != nil {
					roleCounts[team][player.Role]++
				}
				if player.LatencyMillis > evaluation.Evidence.MaxLatencyMillis {
					evaluation.Evidence.MaxLatencyMillis = player.LatencyMillis
				}
			}
		}
		if !qualityContext || teamPlayers == 0 {
			continue
		}
		average := teamSkill / teamPlayers
		if !hasAverage || average < minimumAverage {
			minimumAverage = average
		}
		if !hasAverage || average > maximumAverage {
			maximumAverage = average
		}
		hasAverage = true
	}

	if qualityContext {
		evaluation.Evidence.TeamSkillGap = maximumAverage - minimumAverage
		for _, requirement := range policy.RoleRequirements {
			for team := range teams {
				deficit := requirement.MinPerTeam - roleCounts[team][requirement.Role]
				if deficit <= 0 {
					continue
				}
				if requirement.Hard {
					evaluation.HardViolation = true
					continue
				}
				evaluation.Evidence.RolePenalty += deficit
			}
		}
	}
	slices.SortFunc(evaluation.PriorityWaitMillis, func(left, right int64) int {
		return cmp.Compare(right, left)
	})

	level, step := activeStep(policy.RelaxationSteps, time.Duration(evaluation.Evidence.OldestWaitMillis)*time.Millisecond)
	evaluation.Evidence.RelaxationLevel = level
	evaluation.Evidence.WaitPriority = step.PrioritizeWait
	if evaluation.HardViolation || evaluation.Evidence.TeamSkillGap > step.MaxTeamSkillGap || evaluation.Evidence.RolePenalty > step.MaxRolePenalty {
		evaluation.Admissible = false
	}
	return evaluation
}

// TicketWaitPriority reports whether one demand has reached a policy step that
// gives service age precedence over quality, together with its non-negative wait.
func TicketWaitPriority(now, enqueuedAt time.Time, policy domain.MatchmakingPolicy) (bool, int64) {
	waitMillis := max(int64(0), now.Sub(enqueuedAt).Milliseconds())
	_, step := activeStep(policy.RelaxationSteps, time.Duration(waitMillis)*time.Millisecond)
	return step.PrioritizeWait, waitMillis
}

// Compare returns a negative value when left is preferred over right.
func Compare(left, right Evaluation) int {
	waitFirst := left.Evidence.WaitPriority || right.Evidence.WaitPriority
	if waitFirst {
		if result := descending(left.Evidence.OldestWaitMillis, right.Evidence.OldestWaitMillis); result != 0 {
			return result
		}
		if result := descending(left.Evidence.TotalWaitMillis, right.Evidence.TotalWaitMillis); result != 0 {
			return result
		}
	}
	if result := cmp.Compare(left.Evidence.RolePenalty, right.Evidence.RolePenalty); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Evidence.TeamSkillGap, right.Evidence.TeamSkillGap); result != 0 {
		return result
	}
	if !waitFirst {
		if result := descending(left.Evidence.OldestWaitMillis, right.Evidence.OldestWaitMillis); result != 0 {
			return result
		}
		if result := descending(left.Evidence.TotalWaitMillis, right.Evidence.TotalWaitMillis); result != 0 {
			return result
		}
	}
	return cmp.Compare(left.Evidence.MaxLatencyMillis, right.Evidence.MaxLatencyMillis)
}

func activeStep(steps []domain.RelaxationStep, waited time.Duration) (int, domain.RelaxationStep) {
	if len(steps) == 0 {
		return 0, domain.RelaxationStep{
			MaxTeamSkillGap: math.MaxInt,
			MaxRolePenalty:  math.MaxInt,
		}
	}
	level := 0
	for index := 1; index < len(steps); index++ {
		if waited < steps[index].AfterWait {
			break
		}
		level = index
	}
	return level, steps[level]
}

func descending[T cmp.Ordered](left, right T) int {
	return cmp.Compare(right, left)
}
