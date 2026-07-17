package evaluation

import (
	"cmp"
	"fmt"
	"math/bits"
	"slices"
	"strconv"
	"strings"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/objective"
)

const (
	MaxBatchFrontierTickets   = 12
	MaxBatchFrontierBackfills = 2
	MaxBatchFrontierTeams     = 2
	qualityScale              = 1_000
)

// BatchQuality is one coverage, wait, and per-match quality point.
// Totals are evidence; dominance uses the maximum and mean dimensions.
type BatchQuality struct {
	SelectedBackfills     int
	ProposalCount         int
	MatchedTickets        int
	MatchedPlayers        int
	MaxRolePenalty        int
	MeanRolePenaltyMilli  int
	TotalRolePenalty      int
	MaxTeamSkillGap       int
	MeanTeamSkillGapMilli int
	TotalTeamSkillGap     int
	MaxLatencyMillis      int
	OldestWaitMillis      int64
	MeanWaitMillis        int64
	TotalWaitMillis       int64
}

type BatchFrontierRelation string

const (
	BatchFrontierEquivalent   BatchFrontierRelation = "frontier_equivalent"
	BatchFrontierDominated    BatchFrontierRelation = "frontier_dominated"
	BatchFrontierIncomparable BatchFrontierRelation = "frontier_incomparable"
)

// BatchFrontierResult is exhaustive only inside the documented small-snapshot bound.
type BatchFrontierResult struct {
	PlacementsEvaluated  int
	AdmissibleCandidates int
	BatchesEvaluated     int
	Points               []BatchQuality
}

type BatchFrontierComparison struct {
	Relation   BatchFrontierRelation
	Planner    BatchQuality
	Dominating *BatchQuality
	Frontier   BatchFrontierResult
}

type frontierCandidate struct {
	ticketMask     uint16
	backfillIndex  int
	matchedPlayers int
	evidence       domain.ScoreEvidence
	key            string
}

type frontierCandidateIdentity struct {
	ticketMask    uint16
	backfillIndex int
	rolePenalty   int
	teamSkillGap  int
	maxLatency    int
	oldestWait    int64
	totalWait     int64
}

// BatchFrontierEligible reports whether the snapshot is within the exhaustive safety bound.
func BatchFrontierEligible(snapshot domain.MatchmakingSnapshot) bool {
	return len(snapshot.MatchTickets) <= MaxBatchFrontierTickets &&
		len(snapshot.BackfillTickets) <= MaxBatchFrontierBackfills &&
		snapshot.Policy.TeamCount <= MaxBatchFrontierTeams
}

// ExhaustiveBatchFrontier enumerates all admissible proposal candidates and disjoint batches.
func ExhaustiveBatchFrontier(snapshot domain.MatchmakingSnapshot) (BatchFrontierResult, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return BatchFrontierResult{}, err
	}
	if len(snapshot.MatchTickets) > MaxBatchFrontierTickets {
		return BatchFrontierResult{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"batch frontier ticket count %d exceeds limit %d",
			len(snapshot.MatchTickets),
			MaxBatchFrontierTickets,
		)
	}
	if len(snapshot.BackfillTickets) > MaxBatchFrontierBackfills {
		return BatchFrontierResult{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"batch frontier backfill count %d exceeds limit %d",
			len(snapshot.BackfillTickets),
			MaxBatchFrontierBackfills,
		)
	}
	if snapshot.Policy.TeamCount > MaxBatchFrontierTeams {
		return BatchFrontierResult{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"batch frontier team count %d exceeds limit %d",
			snapshot.Policy.TeamCount,
			MaxBatchFrontierTeams,
		)
	}

	tickets := canonicalMatchTickets(snapshot.MatchTickets)
	backfills := canonicalBackfillTickets(snapshot.BackfillTickets)
	candidates, placements, admissible := enumerateFrontierCandidates(snapshot, tickets, backfills)
	result := enumerateBatchFrontier(candidates, len(tickets), frontierProposalLimit(snapshot.Policy))
	result.PlacementsEvaluated = placements
	result.AdmissibleCandidates = admissible
	return result, nil
}

// CompareBatchFrontier locates the planner batch relative to the exhaustive frontier.
func CompareBatchFrontier(
	snapshot domain.MatchmakingSnapshot,
	batch domain.ProposalBatch,
) (BatchFrontierComparison, error) {
	plannerQuality, err := plannerBatchQuality(snapshot, batch)
	if err != nil {
		return BatchFrontierComparison{}, err
	}
	frontier, err := ExhaustiveBatchFrontier(snapshot)
	if err != nil {
		return BatchFrontierComparison{}, err
	}
	comparison := BatchFrontierComparison{
		Relation: BatchFrontierIncomparable,
		Planner:  plannerQuality,
		Frontier: frontier,
	}
	for _, point := range frontier.Points {
		if batchQualityEquivalent(point, plannerQuality) {
			comparison.Relation = BatchFrontierEquivalent
			return comparison, nil
		}
	}
	for _, point := range frontier.Points {
		if batchQualityDominates(point, plannerQuality) {
			witness := point
			comparison.Relation = BatchFrontierDominated
			comparison.Dominating = &witness
			return comparison, nil
		}
	}
	return comparison, nil
}

func canonicalMatchTickets(input []domain.MatchTicket) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, len(input))
	for index, ticket := range input {
		tickets[index] = domain.CloneMatchTicket(ticket)
	}
	slices.SortFunc(tickets, func(left, right domain.MatchTicket) int {
		if result := cmp.Compare(left.ID, right.ID); result != 0 {
			return result
		}
		return cmp.Compare(left.Revision, right.Revision)
	})
	return tickets
}

func canonicalBackfillTickets(input []domain.BackfillTicket) []domain.BackfillTicket {
	backfills := make([]domain.BackfillTicket, len(input))
	for index, ticket := range input {
		backfills[index] = domain.CloneBackfillTicket(ticket)
	}
	slices.SortFunc(backfills, func(left, right domain.BackfillTicket) int {
		if result := cmp.Compare(left.ID, right.ID); result != 0 {
			return result
		}
		return cmp.Compare(left.Revision, right.Revision)
	})
	return backfills
}

func enumerateFrontierCandidates(
	snapshot domain.MatchmakingSnapshot,
	tickets []domain.MatchTicket,
	backfills []domain.BackfillTicket,
) ([]frontierCandidate, int, int) {
	candidates := make([]frontierCandidate, 0)
	seen := make(map[frontierCandidateIdentity]struct{})
	placementsEvaluated := 0
	admissibleCandidates := 0
	record := func(kind domain.ProposalKind, backfillIndex int, placement [][]domain.MatchTicket, mask uint16, players int) {
		placementsEvaluated++
		evaluation := objective.Evaluate(snapshot.Now, placement, snapshot.Policy, kind)
		if backfillIndex >= 0 {
			backfill := backfills[backfillIndex]
			evaluation = objective.EvaluateBackfill(
				snapshot.Now, placement, snapshot.Policy, backfill.ExistingTeams, backfill.EnqueuedAt,
			)
		}
		if !evaluation.Admissible {
			return
		}
		identity := frontierCandidateIdentity{
			ticketMask: mask, backfillIndex: backfillIndex,
			rolePenalty: evaluation.Evidence.RolePenalty, teamSkillGap: evaluation.Evidence.TeamSkillGap,
			maxLatency: evaluation.Evidence.MaxLatencyMillis,
			oldestWait: evaluation.Evidence.OldestWaitMillis, totalWait: evaluation.Evidence.TotalWaitMillis,
		}
		if _, duplicate := seen[identity]; duplicate {
			return
		}
		seen[identity] = struct{}{}
		admissibleCandidates++
		candidates = append(candidates, frontierCandidate{
			ticketMask: mask, backfillIndex: backfillIndex, matchedPlayers: players,
			evidence: evaluation.Evidence, key: frontierCandidateKey(identity),
		})
	}

	newMatchSlots := make([]int, snapshot.Policy.TeamCount)
	for team := range newMatchSlots {
		newMatchSlots[team] = snapshot.Policy.TeamSize
	}
	enumerateExactPlacements(tickets, newMatchSlots, func(placement [][]domain.MatchTicket, mask uint16, players int) {
		record(domain.ProposalNewMatch, -1, placement, mask, players)
	})
	for index, backfill := range backfills {
		enumerateExactPlacements(tickets, backfill.OpenSlotsByTeam, func(placement [][]domain.MatchTicket, mask uint16, players int) {
			record(domain.ProposalBackfill, index, placement, mask, players)
		})
	}
	slices.SortFunc(candidates, func(left, right frontierCandidate) int {
		return cmp.Compare(left.key, right.key)
	})
	return candidates, placementsEvaluated, admissibleCandidates
}

func enumerateExactPlacements(
	tickets []domain.MatchTicket,
	slots []int,
	record func([][]domain.MatchTicket, uint16, int),
) {
	remaining := slices.Clone(slots)
	assigned := make([][]int, len(slots))
	suffixPlayers := make([]int, len(tickets)+1)
	for index := len(tickets) - 1; index >= 0; index-- {
		suffixPlayers[index] = suffixPlayers[index+1] + len(tickets[index].Players)
	}
	needed := 0
	for _, capacity := range slots {
		needed += capacity
	}

	var enumerate func(int, int, uint16)
	enumerate = func(index, playersNeeded int, mask uint16) {
		if playersNeeded == 0 {
			placement := make([][]domain.MatchTicket, len(assigned))
			for team, indexes := range assigned {
				placement[team] = make([]domain.MatchTicket, len(indexes))
				for position, ticketIndex := range indexes {
					placement[team][position] = tickets[ticketIndex]
				}
			}
			record(placement, mask, needed)
			return
		}
		if index == len(tickets) || suffixPlayers[index] < playersNeeded {
			return
		}
		partySize := len(tickets[index].Players)
		for team := range remaining {
			if partySize > remaining[team] {
				continue
			}
			remaining[team] -= partySize
			assigned[team] = append(assigned[team], index)
			enumerate(index+1, playersNeeded-partySize, mask|1<<index)
			assigned[team] = assigned[team][:len(assigned[team])-1]
			remaining[team] += partySize
		}
		enumerate(index+1, playersNeeded, mask)
	}
	enumerate(0, needed, 0)
}

func frontierCandidateKey(identity frontierCandidateIdentity) string {
	return fmt.Sprintf(
		"%04x/%03d/%d/%d/%d/%d/%d",
		identity.ticketMask,
		identity.backfillIndex+1,
		identity.rolePenalty,
		identity.teamSkillGap,
		identity.maxLatency,
		identity.oldestWait,
		identity.totalWait,
	)
}

func enumerateBatchFrontier(candidates []frontierCandidate, ticketCount, maxProposals int) BatchFrontierResult {
	result := BatchFrontierResult{}
	if ticketCount == 0 {
		result.BatchesEvaluated = 1
		result.Points = []BatchQuality{finalizeBatchQuality(BatchQuality{})}
		return result
	}
	byTicket := make([][]int, ticketCount)
	for candidateIndex, candidate := range candidates {
		for ticket := range ticketCount {
			if candidate.ticketMask&(1<<ticket) != 0 {
				byTicket[ticket] = append(byTicket[ticket], candidateIndex)
			}
		}
	}
	available := uint16(1<<ticketCount) - 1
	selected := make([]int, 0, maxProposals)
	seenBatches := make(map[string]struct{})
	frontier := make([]BatchQuality, 0)

	var search func(uint16, uint8, BatchQuality)
	search = func(remaining uint16, usedBackfills uint8, quality BatchQuality) {
		key := frontierBatchKey(selected)
		if _, seen := seenBatches[key]; !seen {
			seenBatches[key] = struct{}{}
			result.BatchesEvaluated++
			frontier = addFrontierPoint(frontier, finalizeBatchQuality(quality))
		}
		if len(selected) >= maxProposals || remaining == 0 {
			return
		}
		anchor := bits.TrailingZeros16(remaining)
		anchorMask := uint16(1 << anchor)
		search(remaining&^anchorMask, usedBackfills, quality)
		for _, candidateIndex := range byTicket[anchor] {
			candidate := candidates[candidateIndex]
			if candidate.ticketMask&remaining != candidate.ticketMask {
				continue
			}
			nextBackfills := usedBackfills
			if candidate.backfillIndex >= 0 {
				backfillMask := uint8(1 << candidate.backfillIndex)
				if usedBackfills&backfillMask != 0 {
					continue
				}
				nextBackfills |= backfillMask
			}
			selected = append(selected, candidateIndex)
			search(remaining&^candidate.ticketMask, nextBackfills, addCandidateQuality(quality, candidate))
			selected = selected[:len(selected)-1]
		}
	}
	search(available, 0, BatchQuality{})
	sortBatchQuality(frontier)
	result.Points = frontier
	return result
}

func frontierProposalLimit(policy domain.MatchmakingPolicy) int {
	if policy.MaxProposals > 0 {
		return policy.MaxProposals
	}
	return MaxBatchFrontierTickets + MaxBatchFrontierBackfills
}

func frontierBatchKey(selected []int) string {
	if len(selected) == 0 {
		return ""
	}
	ordered := slices.Clone(selected)
	slices.Sort(ordered)
	var builder strings.Builder
	for index, candidate := range ordered {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.Itoa(candidate))
	}
	return builder.String()
}

func addCandidateQuality(quality BatchQuality, candidate frontierCandidate) BatchQuality {
	evidence := candidate.evidence
	quality.ProposalCount++
	quality.MatchedTickets += bits.OnesCount16(candidate.ticketMask)
	quality.MatchedPlayers += candidate.matchedPlayers
	if candidate.backfillIndex >= 0 {
		quality.SelectedBackfills++
	}
	quality.MaxRolePenalty = max(quality.MaxRolePenalty, evidence.RolePenalty)
	quality.TotalRolePenalty += evidence.RolePenalty
	quality.MaxTeamSkillGap = max(quality.MaxTeamSkillGap, evidence.TeamSkillGap)
	quality.TotalTeamSkillGap += evidence.TeamSkillGap
	quality.MaxLatencyMillis = max(quality.MaxLatencyMillis, evidence.MaxLatencyMillis)
	quality.OldestWaitMillis = max(quality.OldestWaitMillis, evidence.OldestWaitMillis)
	quality.TotalWaitMillis += evidence.TotalWaitMillis
	return quality
}

func finalizeBatchQuality(quality BatchQuality) BatchQuality {
	if quality.ProposalCount == 0 {
		return quality
	}
	quality.MeanRolePenaltyMilli = quality.TotalRolePenalty * qualityScale / quality.ProposalCount
	quality.MeanTeamSkillGapMilli = quality.TotalTeamSkillGap * qualityScale / quality.ProposalCount
	if quality.MatchedTickets > 0 {
		quality.MeanWaitMillis = quality.TotalWaitMillis / int64(quality.MatchedTickets)
	}
	return quality
}

func addFrontierPoint(frontier []BatchQuality, candidate BatchQuality) []BatchQuality {
	for _, point := range frontier {
		if batchQualityEquivalent(point, candidate) || batchQualityDominates(point, candidate) {
			return frontier
		}
	}
	kept := frontier[:0]
	for _, point := range frontier {
		if !batchQualityDominates(candidate, point) {
			kept = append(kept, point)
		}
	}
	return append(kept, candidate)
}

func batchQualityEquivalent(left, right BatchQuality) bool {
	return left.SelectedBackfills == right.SelectedBackfills &&
		left.ProposalCount == right.ProposalCount &&
		left.MatchedPlayers == right.MatchedPlayers &&
		left.OldestWaitMillis == right.OldestWaitMillis &&
		left.MeanWaitMillis == right.MeanWaitMillis &&
		left.MaxRolePenalty == right.MaxRolePenalty &&
		left.MeanRolePenaltyMilli == right.MeanRolePenaltyMilli &&
		left.MaxTeamSkillGap == right.MaxTeamSkillGap &&
		left.MeanTeamSkillGapMilli == right.MeanTeamSkillGapMilli &&
		left.MaxLatencyMillis == right.MaxLatencyMillis
}

func batchQualityDominates(left, right BatchQuality) bool {
	if left.ProposalCount == 0 {
		return false
	}
	if right.ProposalCount == 0 {
		return true
	}
	noWorse := left.SelectedBackfills >= right.SelectedBackfills &&
		left.ProposalCount >= right.ProposalCount &&
		left.MatchedPlayers >= right.MatchedPlayers &&
		left.OldestWaitMillis >= right.OldestWaitMillis &&
		left.MeanWaitMillis >= right.MeanWaitMillis &&
		left.MaxRolePenalty <= right.MaxRolePenalty &&
		left.MeanRolePenaltyMilli <= right.MeanRolePenaltyMilli &&
		left.MaxTeamSkillGap <= right.MaxTeamSkillGap &&
		left.MeanTeamSkillGapMilli <= right.MeanTeamSkillGapMilli &&
		left.MaxLatencyMillis <= right.MaxLatencyMillis
	if !noWorse {
		return false
	}
	return left.SelectedBackfills > right.SelectedBackfills ||
		left.ProposalCount > right.ProposalCount ||
		left.MatchedPlayers > right.MatchedPlayers ||
		left.OldestWaitMillis > right.OldestWaitMillis ||
		left.MeanWaitMillis > right.MeanWaitMillis ||
		left.MaxRolePenalty < right.MaxRolePenalty ||
		left.MeanRolePenaltyMilli < right.MeanRolePenaltyMilli ||
		left.MaxTeamSkillGap < right.MaxTeamSkillGap ||
		left.MeanTeamSkillGapMilli < right.MeanTeamSkillGapMilli ||
		left.MaxLatencyMillis < right.MaxLatencyMillis
}

func sortBatchQuality(points []BatchQuality) {
	slices.SortFunc(points, func(left, right BatchQuality) int {
		comparisons := []int{
			cmp.Compare(right.SelectedBackfills, left.SelectedBackfills),
			cmp.Compare(right.ProposalCount, left.ProposalCount),
			cmp.Compare(right.MatchedPlayers, left.MatchedPlayers),
			cmp.Compare(left.MaxRolePenalty, right.MaxRolePenalty),
			cmp.Compare(left.MeanRolePenaltyMilli, right.MeanRolePenaltyMilli),
			cmp.Compare(left.MaxTeamSkillGap, right.MaxTeamSkillGap),
			cmp.Compare(left.MeanTeamSkillGapMilli, right.MeanTeamSkillGapMilli),
			cmp.Compare(left.MaxLatencyMillis, right.MaxLatencyMillis),
			cmp.Compare(right.OldestWaitMillis, left.OldestWaitMillis),
			cmp.Compare(right.MeanWaitMillis, left.MeanWaitMillis),
			cmp.Compare(right.MatchedTickets, left.MatchedTickets),
		}
		for _, result := range comparisons {
			if result != 0 {
				return result
			}
		}
		return 0
	})
}

func plannerBatchQuality(snapshot domain.MatchmakingSnapshot, batch domain.ProposalBatch) (BatchQuality, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return BatchQuality{}, err
	}
	if batch.SnapshotID != snapshot.ID {
		return BatchQuality{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"batch snapshot %q does not match frontier snapshot %q",
			batch.SnapshotID,
			snapshot.ID,
		)
	}
	if len(batch.Proposals) > frontierProposalLimit(snapshot.Policy) {
		return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "batch exceeds policy proposal limit")
	}
	tickets := make(map[domain.TicketID]domain.MatchTicket, len(snapshot.MatchTickets))
	ticketIndexes := make(map[domain.TicketID]int, len(snapshot.MatchTickets))
	canonicalTickets := canonicalMatchTickets(snapshot.MatchTickets)
	for index, ticket := range canonicalTickets {
		tickets[ticket.ID] = ticket
		ticketIndexes[ticket.ID] = index
	}
	backfills := make(map[domain.TicketID]domain.BackfillTicket, len(snapshot.BackfillTickets))
	for _, ticket := range snapshot.BackfillTickets {
		backfills[ticket.ID] = ticket
	}
	usedTickets := make(map[domain.TicketID]struct{})
	usedBackfills := make(map[domain.TicketID]struct{})
	policyFingerprint, err := domain.FingerprintPolicy(snapshot.Policy)
	if err != nil {
		return BatchQuality{}, err
	}
	quality := BatchQuality{}
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			return BatchQuality{}, err
		}
		if proposal.PolicyVersion != snapshot.Policy.Version || proposal.PolicyFingerprint != policyFingerprint || len(proposal.Teams) != snapshot.Policy.TeamCount {
			return BatchQuality{}, domain.NewFailure(
				domain.FailureInvalidInput,
				"proposal %q does not use frontier policy %q",
				proposal.ID,
				snapshot.Policy.Version,
			)
		}
		var targetBackfill *domain.BackfillTicket
		backfillIndex := -1
		if proposal.Kind == domain.ProposalBackfill {
			target := proposal.Backfill
			backfill, exists := backfills[target.Ticket.ID]
			if !exists || backfill.Revision != target.Ticket.Revision || backfill.SessionID != target.SessionID || backfill.RosterVersion != target.RosterVersion {
				return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "proposal %q references stale backfill %q", proposal.ID, target.Ticket.ID)
			}
			if _, used := usedBackfills[target.Ticket.ID]; used {
				return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "batch reuses backfill %q", target.Ticket.ID)
			}
			usedBackfills[target.Ticket.ID] = struct{}{}
			targetBackfill = &backfill
			backfillIndex = 0
		}
		placement := make([][]domain.MatchTicket, len(proposal.Teams))
		matchedPlayers := 0
		var ticketMask uint16
		for teamIndex, team := range proposal.Teams {
			teamPlayers := 0
			for _, reference := range team.Tickets {
				ticket, exists := tickets[reference.ID]
				if !exists || ticket.Revision != reference.Revision {
					return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "proposal %q references stale ticket %q", proposal.ID, reference.ID)
				}
				if _, used := usedTickets[reference.ID]; used {
					return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "batch reuses ticket %q", reference.ID)
				}
				usedTickets[reference.ID] = struct{}{}
				placement[teamIndex] = append(placement[teamIndex], ticket)
				teamPlayers += len(ticket.Players)
				matchedPlayers += len(ticket.Players)
			}
			expected := snapshot.Policy.TeamSize
			if targetBackfill != nil {
				expected = targetBackfill.OpenSlotsByTeam[teamIndex]
			}
			if teamPlayers != expected {
				return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "proposal %q team %d fills %d players; want %d", proposal.ID, teamIndex, teamPlayers, expected)
			}
		}
		evaluation := objective.Evaluate(snapshot.Now, placement, snapshot.Policy, proposal.Kind)
		if targetBackfill != nil {
			evaluation = objective.EvaluateBackfill(
				snapshot.Now, placement, snapshot.Policy, targetBackfill.ExistingTeams, targetBackfill.EnqueuedAt,
			)
		}
		if !evaluation.Admissible {
			return BatchQuality{}, domain.NewFailure(domain.FailureInvalidInput, "proposal %q is not admissible", proposal.ID)
		}
		for _, reference := range proposal.Tickets {
			ticketMask |= 1 << ticketIndexes[reference.ID]
		}
		quality = addCandidateQuality(quality, frontierCandidate{
			ticketMask: ticketMask, backfillIndex: backfillIndex, matchedPlayers: matchedPlayers,
			evidence: evaluation.Evidence,
		})
	}
	return finalizeBatchQuality(quality), nil
}
