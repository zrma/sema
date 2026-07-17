package planner_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestPlanReturnsDeterministicDisjointMatches(t *testing.T) {
	snapshot := snapshotWith("multi-match", policy(2, 2), partyTickets([]int{1, 1, 1, 1, 1, 1, 1, 1}))

	first, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reversed := snapshot
	reversed.MatchTickets = slices.Clone(snapshot.MatchTickets)
	slices.Reverse(reversed.MatchTickets)
	second, err := planner.Plan(reversed)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("planning is not deterministic:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if len(first.Proposals) != 2 || len(first.Unmatched) != 0 {
		t.Fatalf("proposals = %d, unmatched = %d; want 2, 0", len(first.Proposals), len(first.Unmatched))
	}
	assertDisjointAndCapacity(t, first, snapshot.MatchTickets, 2, 2)
}

func TestProposalIdentityIncludesPolicyContent(t *testing.T) {
	tickets := partyTickets([]int{1, 1, 1, 1})
	firstPolicy := policy(2, 2)
	firstPolicy.Version = "shared-version"
	secondPolicy := firstPolicy
	secondPolicy.MaxLatencyMillis++

	first, err := planner.Plan(snapshotWith("policy-identity", firstPolicy, tickets))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := planner.Plan(snapshotWith("policy-identity", firstPolicy, tickets))
	if err != nil {
		t.Fatal(err)
	}
	second, err := planner.Plan(snapshotWith("policy-identity", secondPolicy, tickets))
	if err != nil {
		t.Fatal(err)
	}
	firstProposal := first.Proposals[0]
	if firstProposal.ID != repeated.Proposals[0].ID ||
		firstProposal.PolicyFingerprint != repeated.Proposals[0].PolicyFingerprint {
		t.Fatal("same snapshot and policy did not preserve proposal identity")
	}
	secondProposal := second.Proposals[0]
	if firstProposal.PolicyVersion != secondProposal.PolicyVersion {
		t.Fatalf("policy versions differ: %q != %q", firstProposal.PolicyVersion, secondProposal.PolicyVersion)
	}
	if firstProposal.PolicyFingerprint == secondProposal.PolicyFingerprint || firstProposal.ID == secondProposal.ID {
		t.Fatalf("different policy content reused identity: first=%#v second=%#v", firstProposal, secondProposal)
	}
	if !reflect.DeepEqual(firstProposal.Teams, secondProposal.Teams) {
		t.Fatalf("fixture changed placement instead of only policy identity: first=%#v second=%#v", firstProposal.Teams, secondProposal.Teams)
	}
}

func TestPlanPreservesParties(t *testing.T) {
	snapshot := snapshotWith("party-preservation", policy(2, 3), partyTickets([]int{2, 2, 1, 1}))

	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 {
		t.Fatalf("proposals = %d; want 1", len(batch.Proposals))
	}
	assertDisjointAndCapacity(t, batch, snapshot.MatchTickets, 2, 3)
}

func TestPlanCoversReferenceTeamWorkloads(t *testing.T) {
	for _, teamSize := range []int{2, 3, 5, 10, 16, 20, 50} {
		variants := map[string][]int{
			"all-solo":   repeatedPartySizes(teamSize*2, 1),
			"full-party": {teamSize, teamSize},
			"mixed-party": func() []int {
				if teamSize == 2 {
					return []int{2, 1, 1}
				}
				return []int{teamSize - 1, 1, teamSize - 1, 1}
			}(),
		}
		for name, sizes := range variants {
			t.Run(fmt.Sprintf("%dv%d/%s", teamSize, teamSize, name), func(t *testing.T) {
				tickets := partyTickets(sizes)
				batch, err := planner.Plan(snapshotWith(t.Name(), policy(2, teamSize), tickets))
				if err != nil {
					t.Fatal(err)
				}
				if len(batch.Proposals) != 1 || len(batch.Unmatched) != 0 {
					t.Fatalf("proposals = %d, unmatched = %d; want 1, 0", len(batch.Proposals), len(batch.Unmatched))
				}
				assertDisjointAndCapacity(t, batch, tickets, 2, teamSize)
			})
		}
	}
}

func TestPlanCoversBattleRoyalePartyEnvelope(t *testing.T) {
	for _, partySize := range []int{2, 4} {
		t.Run(fmt.Sprintf("party-%d", partySize), func(t *testing.T) {
			tickets := partyTickets(repeatedPartySizes(100/partySize, partySize))
			batch, err := planner.Plan(snapshotWith(t.Name(), policy(1, 100), tickets))
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Proposals) != 1 || len(batch.Unmatched) != 0 {
				t.Fatalf("proposals = %d, unmatched = %d; want 1, 0", len(batch.Proposals), len(batch.Unmatched))
			}
			assertDisjointAndCapacity(t, batch, tickets, 1, 100)
		})
	}
}

func TestPlanPrioritizesBackfill(t *testing.T) {
	snapshot := snapshotWith("backfill-first", policy(2, 2), partyTickets([]int{1, 1, 1, 1}))
	snapshot.BackfillTickets = []domain.BackfillTicket{
		{
			ID:              "backfill-a",
			Revision:        3,
			SessionID:       "session-a",
			RosterVersion:   7,
			OpenSlotsByTeam: []int{1, 1},
			EnqueuedAt:      fixtureNow.Add(-2 * time.Minute),
		},
	}

	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || batch.Proposals[0].Kind != domain.ProposalBackfill {
		t.Fatalf("proposals = %#v; want one backfill", batch.Proposals)
	}
	if len(batch.Unmatched) != 2 {
		t.Fatalf("unmatched = %d; want 2", len(batch.Unmatched))
	}
	target := batch.Proposals[0].Backfill
	if target == nil || target.Ticket.Revision != 3 || target.RosterVersion != 7 {
		t.Fatalf("backfill target = %#v; freshness was not preserved", target)
	}
}

func TestPlanKeepsHardConstraintFailuresUnmatched(t *testing.T) {
	tickets := partyTickets([]int{3, 1, 1, 1})
	tickets[len(tickets)-1].Players[0].LatencyMillis = 201
	batch, err := planner.Plan(snapshotWith("hard-constraints", policy(2, 2), tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 0 || len(batch.Unmatched) != len(tickets) {
		t.Fatalf("proposals = %d, unmatched = %d; want 0, %d", len(batch.Proposals), len(batch.Unmatched), len(tickets))
	}
}

func TestPlanReportsSearchBudgetExhaustion(t *testing.T) {
	configured := policy(2, 2)
	configured.MaxSearchNodes = 1
	batch, err := planner.Plan(snapshotWith("bounded-search", configured, partyTickets([]int{1, 1, 1, 1})))
	if err != nil {
		t.Fatal(err)
	}
	if !batch.BudgetExhausted || len(batch.Proposals) != 0 || len(batch.Unmatched) != 4 {
		t.Fatalf("batch = %#v; want an explicit best-known no-match", batch)
	}
}

func TestPlanSelectsBestSkillCandidate(t *testing.T) {
	configured := objectivePolicy(2, 1)
	configured.RoleRequirements = nil
	configured.RelaxationSteps[0].MaxTeamSkillGap = 1_000
	configured.RelaxationSteps[1].MaxTeamSkillGap = 1_000
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "b", skill: 1500, wait: 10 * time.Second, latency: 20},
		{id: "c", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "d", skill: 1500, wait: 10 * time.Second, latency: 20},
	})

	batch, err := planner.Plan(snapshotWith("best-skill", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	first := batch.Proposals[0]
	if first.Evidence.TeamSkillGap != 0 || !proposalHasTickets(first, "a", "c") {
		t.Fatalf("first proposal = %#v; want the canonical zero-gap pair a/c", first)
	}
}

func TestPlanAppliesRoleThresholdAndWaitRelaxation(t *testing.T) {
	configured := objectivePolicy(2, 2)
	short := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
		{id: "b", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
		{id: "c", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
		{id: "d", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
	})
	shortBatch, err := planner.Plan(snapshotWith("short-role", configured, short))
	if err != nil {
		t.Fatal(err)
	}
	if len(shortBatch.Proposals) != 0 {
		t.Fatalf("short-wait role violation produced a match: %#v", shortBatch.Proposals)
	}
	assertAllUnmatchedReason(t, shortBatch, domain.UnmatchedQualityThreshold)

	long := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
		{id: "b", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
		{id: "c", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
		{id: "d", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
	})
	longBatch, err := planner.Plan(snapshotWith("long-role", configured, long))
	if err != nil {
		t.Fatal(err)
	}
	if len(longBatch.Proposals) != 1 {
		t.Fatalf("relaxed role policy returned %d proposals; want 1", len(longBatch.Proposals))
	}
	evidence := longBatch.Proposals[0].Evidence
	if evidence.RelaxationLevel != 1 || !evidence.WaitPriority || evidence.RolePenalty != 2 {
		t.Fatalf("relaxed evidence = %#v", evidence)
	}
}

func TestPlanSatisfiesSoftRolesBeforeQueueOrder(t *testing.T) {
	configured := objectivePolicy(2, 2)
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a-healer", skill: 1000, role: "healer", wait: 10 * time.Second, latency: 20},
		{id: "b-healer", skill: 1000, role: "healer", wait: 10 * time.Second, latency: 20},
		{id: "c-dps", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
		{id: "d-dps", skill: 1000, role: "dps", wait: 10 * time.Second, latency: 20},
		{id: "e-dps", skill: 1000, role: "dps", wait: 11 * time.Second, latency: 20},
		{id: "f-dps", skill: 1000, role: "dps", wait: 11 * time.Second, latency: 20},
	})

	batch, err := planner.Plan(snapshotWith("role-quality", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) == 0 || !proposalHasTickets(batch.Proposals[0], "a-healer", "b-healer") {
		t.Fatalf("proposal did not satisfy the soft role requirement: %#v", batch.Proposals)
	}
	if batch.Proposals[0].Evidence.RolePenalty != 0 {
		t.Fatalf("role penalty = %d; want 0", batch.Proposals[0].Evidence.RolePenalty)
	}
}

func TestPlanUsesWaitThenLatencyTieBreaks(t *testing.T) {
	configured := objectivePolicy(2, 1)
	configured.RoleRequirements = nil
	waitTickets := namedSoloTickets([]ticketAttributes{
		{id: "a-old", skill: 1000, wait: time.Minute, latency: 40},
		{id: "b-old", skill: 1200, wait: time.Minute, latency: 40},
		{id: "c-new", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "d-new", skill: 1000, wait: 10 * time.Second, latency: 20},
	})
	waitBatch, err := planner.Plan(snapshotWith("wait-first", configured, waitTickets))
	if err != nil {
		t.Fatal(err)
	}
	if !proposalHasTickets(waitBatch.Proposals[0], "a-old", "b-old") {
		t.Fatalf("wait-first proposal = %#v; want both oldest tickets", waitBatch.Proposals[0])
	}

	configured.RelaxationSteps = configured.RelaxationSteps[:1]
	configured.RelaxationSteps[0].MaxTeamSkillGap = 0
	latencyTickets := namedSoloTickets([]ticketAttributes{
		{id: "a-high", skill: 1000, wait: 10 * time.Second, latency: 80},
		{id: "b-high", skill: 1000, wait: 10 * time.Second, latency: 80},
		{id: "c-low", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "d-low", skill: 1000, wait: 10 * time.Second, latency: 20},
	})
	latencyBatch, err := planner.Plan(snapshotWith("latency-tie", configured, latencyTickets))
	if err != nil {
		t.Fatal(err)
	}
	if !proposalHasTickets(latencyBatch.Proposals[0], "c-low", "d-low") {
		t.Fatalf("latency proposal = %#v; want the lower-latency pair", latencyBatch.Proposals[0])
	}
}

func TestPlanReportsStableUnmatchedReasons(t *testing.T) {
	configured := objectivePolicy(2, 1)
	configured.RoleRequirements = nil
	configured.MaxProposals = 1
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "b", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "c", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "d", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "e-hard", skill: 1000, wait: 10 * time.Second, latency: 201},
	})
	batch, err := planner.Plan(snapshotWith("unmatched-reasons", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	for _, unmatched := range batch.Unmatched {
		if unmatched.Ticket.ID == "e-hard" {
			if unmatched.Reason != domain.UnmatchedHardConstraint {
				t.Fatalf("hard rejection reason = %q", unmatched.Reason)
			}
			continue
		}
		if unmatched.Reason != domain.UnmatchedProposalLimit {
			t.Fatalf("ticket %q reason = %q; want proposal_limit", unmatched.Ticket.ID, unmatched.Reason)
		}
	}
}

func TestPlanExposesCandidateTruncation(t *testing.T) {
	configured := objectivePolicy(2, 1)
	configured.RoleRequirements = nil
	configured.MaxCandidatesPerProposal = 1
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "b", skill: 1000, wait: 10 * time.Second, latency: 20},
		{id: "c", skill: 1000, wait: 10 * time.Second, latency: 20},
	})
	batch, err := planner.Plan(snapshotWith("candidate-truncation", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if !batch.BudgetExhausted || len(batch.Proposals) == 0 {
		t.Fatalf("batch = %#v; want a best-known truncated proposal", batch)
	}
	evidence := batch.Proposals[0].Evidence
	if !evidence.SearchTruncated || evidence.CandidatesEvaluated != 1 {
		t.Fatalf("truncation evidence = %#v", evidence)
	}
}

func TestPlanCandidateWindowPrioritizesOldestTicketsAndExposesQualityGap(t *testing.T) {
	configured := policy(2, 1)
	configured.MaxProposals = 1
	configured.MaxCandidateTickets = 2
	configured.MaxCandidatesPerProposal = 64
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a-old-low", skill: 0, wait: time.Minute, latency: 20},
		{id: "b-old-high", skill: 1000, wait: time.Minute, latency: 20},
		{id: "c-new-mid", skill: 500, wait: 10 * time.Second, latency: 20},
		{id: "d-new-mid", skill: 500, wait: 10 * time.Second, latency: 20},
	})
	bounded, err := planner.Plan(snapshotWith("candidate-window", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(bounded.Proposals) != 1 || !proposalHasTickets(bounded.Proposals[0], "a-old-low", "b-old-high") {
		t.Fatalf("bounded proposal = %#v", bounded.Proposals)
	}
	evidence := bounded.Proposals[0].Evidence
	if evidence.CandidateTickets != 2 || !evidence.CandidateWindowTruncated || !evidence.SearchTruncated || !bounded.BudgetExhausted {
		t.Fatalf("candidate window evidence = %#v, batch=%#v", evidence, bounded)
	}
	if evidence.TeamSkillGap != 1000 {
		t.Fatalf("bounded skill gap = %d; want 1000", evidence.TeamSkillGap)
	}

	configured.MaxCandidateTickets = 0
	unbounded, err := planner.Plan(snapshotWith("candidate-window", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if !proposalHasTickets(unbounded.Proposals[0], "c-new-mid", "d-new-mid") || unbounded.Proposals[0].Evidence.TeamSkillGap != 0 {
		t.Fatalf("unbounded proposal = %#v", unbounded.Proposals[0])
	}
}

func TestPlanCandidateWindowSkipsPartiesThatCannotFitBackfill(t *testing.T) {
	configured := policy(2, 2)
	configured.MaxProposals = 1
	configured.MaxCandidateTickets = 2
	tickets := partyTickets([]int{2, 1, 1})
	snapshot := snapshotWith("backfill-candidate-window", configured, tickets)
	snapshot.BackfillTickets = []domain.BackfillTicket{{
		ID: "backfill", Revision: 1, SessionID: "session", RosterVersion: 1,
		OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
	}}
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || batch.Proposals[0].Kind != domain.ProposalBackfill {
		t.Fatalf("backfill batch = %#v", batch)
	}
	if !proposalHasTickets(batch.Proposals[0], "ticket-0001", "ticket-0002") {
		t.Fatalf("backfill placement = %#v", batch.Proposals[0])
	}
	if batch.Proposals[0].Evidence.CandidateTickets != 2 || batch.Proposals[0].Evidence.CandidateWindowTruncated {
		t.Fatalf("backfill candidate evidence = %#v", batch.Proposals[0].Evidence)
	}
}

func TestPlanReportsSearchBudgetWhenCandidateWindowCannotFillMatch(t *testing.T) {
	configured := policy(2, 1)
	configured.MaxCandidateTickets = 1
	tickets := partyTickets([]int{1, 1, 1})
	batch, err := planner.Plan(snapshotWith("candidate-window-no-match", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 0 || !batch.BudgetExhausted {
		t.Fatalf("batch = %#v; want budget-exhausted no-match", batch)
	}
	assertAllUnmatchedReason(t, batch, domain.UnmatchedSearchBudget)
}

func TestPlanTenThousandTicketCandidateWindow(t *testing.T) {
	configured := policy(2, 5)
	configured.MaxProposals = 1
	configured.MaxCandidateTickets = 256
	configured.MaxCandidatesPerProposal = 64
	tickets := partyTickets(repeatedPartySizes(10_000, 1))
	batch, err := planner.Plan(snapshotWith("queue-10000-window-256", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || len(batch.Proposals[0].Tickets) != 10 || len(batch.Unmatched) != 9_990 {
		t.Fatalf("large queue outcome: proposals=%d matched=%d unmatched=%d", len(batch.Proposals), len(batch.Proposals[0].Tickets), len(batch.Unmatched))
	}
	evidence := batch.Proposals[0].Evidence
	if evidence.CandidateTickets != 256 || !evidence.CandidateWindowTruncated || !batch.BudgetExhausted {
		t.Fatalf("large queue evidence = %#v, budget=%t", evidence, batch.BudgetExhausted)
	}
	assertDisjointAndCapacity(t, batch, tickets, 2, 5)
}

func TestPlanReportsHardRoleReason(t *testing.T) {
	configured := objectivePolicy(2, 1)
	configured.RoleRequirements[0].Hard = true
	tickets := namedSoloTickets([]ticketAttributes{
		{id: "a", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
		{id: "b", skill: 1000, role: "dps", wait: time.Minute, latency: 20},
	})
	batch, err := planner.Plan(snapshotWith("hard-role", configured, tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 0 {
		t.Fatalf("hard role violation produced a proposal: %#v", batch.Proposals)
	}
	assertAllUnmatchedReason(t, batch, domain.UnmatchedHardConstraint)
}

func BenchmarkPlanReferenceWorkloads(b *testing.B) {
	benchmarks := []struct {
		name    string
		policy  domain.MatchmakingPolicy
		parties []int
	}{
		{name: "2v2-solo", policy: policy(2, 2), parties: repeatedPartySizes(4, 1)},
		{name: "50v50-solo", policy: policy(2, 50), parties: repeatedPartySizes(100, 1)},
		{name: "battle-royale-duo", policy: policy(1, 100), parties: repeatedPartySizes(50, 2)},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			snapshot := snapshotWith(benchmark.name, benchmark.policy, partyTickets(benchmark.parties))
			b.ReportAllocs()
			for range b.N {
				if _, err := planner.Plan(snapshot); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPlanQueueSizes(b *testing.B) {
	for _, queueSize := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("5v5/queue-%d", queueSize), func(b *testing.B) {
			configured := policy(2, 5)
			configured.MaxProposals = 1
			configured.MaxCandidatesPerProposal = 64
			snapshot := snapshotWith(b.Name(), configured, partyTickets(repeatedPartySizes(queueSize, 1)))
			b.ReportAllocs()
			for range b.N {
				if _, err := planner.Plan(snapshot); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPlanLargeQueues keeps the P7 10K/100K capacity path in the planner benchmark gate.
func BenchmarkPlanLargeQueues(b *testing.B) {
	for _, candidateWindow := range []int{0, 256} {
		for _, queueSize := range []int{10_000, 100_000} {
			b.Run(fmt.Sprintf("5v5/window-%d/queue-%d", candidateWindow, queueSize), func(b *testing.B) {
				configured := policy(2, 5)
				configured.MaxProposals = 1
				configured.MaxCandidateTickets = candidateWindow
				configured.MaxCandidatesPerProposal = 64
				snapshot := snapshotWith(b.Name(), configured, partyTickets(repeatedPartySizes(queueSize, 1)))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := planner.Plan(snapshot); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func snapshotWith(id string, configured domain.MatchmakingPolicy, tickets []domain.MatchTicket) domain.MatchmakingSnapshot {
	return domain.MatchmakingSnapshot{
		ID:           domain.SnapshotID(id),
		Now:          fixtureNow,
		MatchTickets: tickets,
		Policy:       configured,
	}
}

func policy(teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:          "test-v1",
		TeamCount:        teamCount,
		TeamSize:         teamSize,
		MaxLatencyMillis: 200,
		MaxSearchNodes:   100_000,
	}
}

func objectivePolicy(teamCount, teamSize int) domain.MatchmakingPolicy {
	configured := policy(teamCount, teamSize)
	configured.MaxCandidatesPerProposal = 64
	configured.RoleRequirements = []domain.RoleRequirement{{Role: "healer", MinPerTeam: 1}}
	configured.RelaxationSteps = []domain.RelaxationStep{
		{AfterWait: 0, MaxTeamSkillGap: 50, MaxRolePenalty: 0},
		{AfterWait: 30 * time.Second, MaxTeamSkillGap: 200, MaxRolePenalty: 2, PrioritizeWait: true},
	}
	return configured
}

type ticketAttributes struct {
	id      string
	skill   int
	role    string
	wait    time.Duration
	latency int
}

func namedSoloTickets(attributes []ticketAttributes) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, len(attributes))
	for index, attributes := range attributes {
		tickets[index] = domain.MatchTicket{
			ID:         domain.TicketID(attributes.id),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-attributes.wait),
			Players: []domain.Player{{
				ID:            domain.PlayerID("player-" + attributes.id),
				Skill:         attributes.skill,
				Role:          attributes.role,
				LatencyMillis: attributes.latency,
			}},
		}
	}
	return tickets
}

func proposalHasTickets(proposal domain.MatchProposal, ids ...domain.TicketID) bool {
	seen := make(map[domain.TicketID]struct{}, len(proposal.Tickets))
	for _, ref := range proposal.Tickets {
		seen[ref.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			return false
		}
	}
	return true
}

func assertAllUnmatchedReason(t *testing.T, batch domain.ProposalBatch, reason domain.UnmatchedReason) {
	t.Helper()
	for _, unmatched := range batch.Unmatched {
		if unmatched.Reason != reason {
			t.Fatalf("ticket %q reason = %q; want %q", unmatched.Ticket.ID, unmatched.Reason, reason)
		}
	}
}

func partyTickets(sizes []int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, len(sizes))
	playerSequence := 0
	for ticketIndex, size := range sizes {
		players := make([]domain.Player, size)
		for playerIndex := range players {
			players[playerIndex] = domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("player-%04d", playerSequence)),
				Skill:         1000 + playerSequence%7,
				LatencyMillis: 20 + playerSequence%5,
			}
			playerSequence++
		}
		tickets[ticketIndex] = domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("ticket-%04d", ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(len(sizes)-ticketIndex) * time.Second),
			Players:    players,
		}
	}
	return tickets
}

func repeatedPartySizes(count, size int) []int {
	sizes := make([]int, count)
	for index := range sizes {
		sizes[index] = size
	}
	return sizes
}

func assertDisjointAndCapacity(
	t *testing.T,
	batch domain.ProposalBatch,
	tickets []domain.MatchTicket,
	teamCount int,
	teamSize int,
) {
	t.Helper()
	sizes := make(map[domain.TicketID]int, len(tickets))
	for _, ticket := range tickets {
		sizes[ticket.ID] = len(ticket.Players)
	}
	seen := make(map[domain.TicketID]struct{}, len(tickets))
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			t.Fatalf("invalid proposal: %v", err)
		}
		if len(proposal.Teams) != teamCount {
			t.Fatalf("teams = %d; want %d", len(proposal.Teams), teamCount)
		}
		for _, team := range proposal.Teams {
			players := 0
			for _, ref := range team.Tickets {
				if _, exists := seen[ref.ID]; exists {
					t.Fatalf("ticket %q appears more than once", ref.ID)
				}
				seen[ref.ID] = struct{}{}
				players += sizes[ref.ID]
			}
			if players != teamSize {
				t.Fatalf("team %d has %d players; want %d", team.Team, players, teamSize)
			}
		}
	}
}
