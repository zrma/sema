package evaluation

import (
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/objective"
)

const (
	MaxOracleTickets = 12
	MaxOracleTeams   = 2
)

type OracleResult struct {
	Found                bool
	CandidatesEvaluated  int
	AdmissibleCandidates int
	BestEvidence         domain.ScoreEvidence
}

type QualityRelation string

const (
	QualityEquivalent       QualityRelation = "equivalent"
	QualityOraclePreferred  QualityRelation = "oracle_preferred"
	QualityPlannerPreferred QualityRelation = "planner_preferred"
)

type OracleComparison struct {
	Relation        QualityRelation
	PlannerFound    bool
	Oracle          OracleResult
	PlannerEvidence domain.ScoreEvidence
}

// OracleEligible reports whether exhaustive new-match enumeration is bounded for a snapshot.
func OracleEligible(snapshot domain.MatchmakingSnapshot) bool {
	return len(snapshot.BackfillTickets) == 0 &&
		snapshot.Policy.TeamCount <= MaxOracleTeams &&
		len(snapshot.MatchTickets) <= MaxOracleTickets
}

// ExhaustiveNewMatch finds the best admissible single new-match quality vector.
func ExhaustiveNewMatch(snapshot domain.MatchmakingSnapshot) (OracleResult, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return OracleResult{}, err
	}
	if len(snapshot.BackfillTickets) > 0 {
		return OracleResult{}, domain.NewFailure(domain.FailureInvalidInput, "oracle supports new-match snapshots without backfill")
	}
	if snapshot.Policy.TeamCount > MaxOracleTeams {
		return OracleResult{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"oracle team count %d exceeds limit %d",
			snapshot.Policy.TeamCount,
			MaxOracleTeams,
		)
	}
	if len(snapshot.MatchTickets) > MaxOracleTickets {
		return OracleResult{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"oracle ticket count %d exceeds limit %d",
			len(snapshot.MatchTickets),
			MaxOracleTickets,
		)
	}

	result := OracleResult{}
	remaining := make([]int, snapshot.Policy.TeamCount)
	for team := range remaining {
		remaining[team] = snapshot.Policy.TeamSize
	}
	assigned := make([][]domain.MatchTicket, snapshot.Policy.TeamCount)
	suffixPlayers := make([]int, len(snapshot.MatchTickets)+1)
	for index := len(snapshot.MatchTickets) - 1; index >= 0; index-- {
		suffixPlayers[index] = suffixPlayers[index+1] + len(snapshot.MatchTickets[index].Players)
	}

	var enumerate func(index, needed int)
	enumerate = func(index, needed int) {
		if needed == 0 {
			evaluation := objective.Evaluate(snapshot.Now, assigned, snapshot.Policy, domain.ProposalNewMatch)
			result.CandidatesEvaluated++
			if !evaluation.Admissible {
				return
			}
			result.AdmissibleCandidates++
			if !result.Found || objective.Compare(evaluation, objective.Evaluation{Admissible: true, Evidence: result.BestEvidence}) < 0 {
				result.Found = true
				result.BestEvidence = evaluation.Evidence
			}
			return
		}
		if index == len(snapshot.MatchTickets) || suffixPlayers[index] < needed {
			return
		}

		ticket := snapshot.MatchTickets[index]
		partySize := len(ticket.Players)
		for team := range remaining {
			if partySize > remaining[team] {
				continue
			}
			remaining[team] -= partySize
			assigned[team] = append(assigned[team], ticket)
			enumerate(index+1, needed-partySize)
			assigned[team] = assigned[team][:len(assigned[team])-1]
			remaining[team] += partySize
		}
		enumerate(index+1, needed)
	}

	enumerate(0, snapshot.Policy.TeamCount*snapshot.Policy.TeamSize)
	return result, nil
}

// CompareBatch compares the first planner proposal with the exhaustive quality optimum.
func CompareBatch(snapshot domain.MatchmakingSnapshot, batch domain.ProposalBatch) (OracleComparison, error) {
	if batch.SnapshotID != snapshot.ID {
		return OracleComparison{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"batch snapshot %q does not match evaluation snapshot %q",
			batch.SnapshotID,
			snapshot.ID,
		)
	}
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			return OracleComparison{}, err
		}
		if proposal.Kind != domain.ProposalNewMatch || proposal.PolicyVersion != snapshot.Policy.Version {
			return OracleComparison{}, domain.NewFailure(
				domain.FailureInvalidInput,
				"oracle comparison needs new-match proposals from policy %q",
				snapshot.Policy.Version,
			)
		}
	}
	oracle, err := ExhaustiveNewMatch(snapshot)
	if err != nil {
		return OracleComparison{}, err
	}
	comparison := OracleComparison{Oracle: oracle, PlannerFound: len(batch.Proposals) > 0}
	if comparison.PlannerFound {
		comparison.PlannerEvidence = batch.Proposals[0].Evidence
	}
	switch {
	case !comparison.PlannerFound && !oracle.Found:
		comparison.Relation = QualityEquivalent
	case !comparison.PlannerFound && oracle.Found:
		comparison.Relation = QualityOraclePreferred
	case comparison.PlannerFound && !oracle.Found:
		comparison.Relation = QualityPlannerPreferred
	default:
		plannerEvaluation := objective.Evaluation{Admissible: true, Evidence: comparison.PlannerEvidence}
		oracleEvaluation := objective.Evaluation{Admissible: true, Evidence: oracle.BestEvidence}
		result := objective.Compare(plannerEvaluation, oracleEvaluation)
		if result < 0 {
			comparison.Relation = QualityPlannerPreferred
		} else if result > 0 {
			comparison.Relation = QualityOraclePreferred
		} else {
			comparison.Relation = QualityEquivalent
		}
	}
	return comparison, nil
}
