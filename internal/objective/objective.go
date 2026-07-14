// Package objective evaluates and compares versioned matchmaking policy outcomes.
package objective

import (
	"cmp"
	"math"
	"time"

	"sema/internal/constraint"
	"sema/internal/domain"
)

// Evaluation contains an admissibility decision and its replayable evidence.
type Evaluation struct {
	Admissible    bool
	HardViolation bool
	Evidence      domain.ScoreEvidence
}

// Evaluate calculates hard violations, active relaxation, and soft evidence.
func Evaluate(
	now time.Time,
	teams [][]domain.MatchTicket,
	policy domain.MatchmakingPolicy,
	kind domain.ProposalKind,
) Evaluation {
	evaluation := Evaluation{Admissible: true, HardViolation: constraint.HardViolation(teams, policy, kind)}
	var roleCounts []map[string]int
	if kind == domain.ProposalNewMatch && len(policy.RoleRequirements) > 0 {
		roleCounts = make([]map[string]int, len(teams))
	}
	minimumAverage, maximumAverage := 0, 0
	hasAverage := false

	for team, tickets := range teams {
		if roleCounts != nil {
			roleCounts[team] = make(map[string]int)
		}
		teamSkill, teamPlayers := 0, 0
		for _, ticket := range tickets {
			waitMillis := now.Sub(ticket.EnqueuedAt).Milliseconds()
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
		if kind == domain.ProposalBackfill || teamPlayers == 0 {
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

	if kind == domain.ProposalNewMatch {
		evaluation.Evidence.TeamSkillGap = maximumAverage - minimumAverage
		for _, requirement := range policy.RoleRequirements {
			for team := range teams {
				deficit := requirement.MinPerTeam - roleCounts[team][requirement.Role]
				if deficit <= 0 {
					continue
				}
				if requirement.Hard {
					continue
				}
				evaluation.Evidence.RolePenalty += deficit
			}
		}
	}

	level, step := activeStep(policy.RelaxationSteps, time.Duration(evaluation.Evidence.OldestWaitMillis)*time.Millisecond)
	evaluation.Evidence.RelaxationLevel = level
	evaluation.Evidence.WaitPriority = step.PrioritizeWait
	if evaluation.HardViolation || evaluation.Evidence.TeamSkillGap > step.MaxTeamSkillGap || evaluation.Evidence.RolePenalty > step.MaxRolePenalty {
		evaluation.Admissible = false
	}
	return evaluation
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
