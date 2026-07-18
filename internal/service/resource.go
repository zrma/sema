// Package service defines the product-facing resource model above storage adapters.
package service

import (
	"slices"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

type ResourceKind string

const (
	ResourcePolicy           ResourceKind = "policy"
	ResourceMatchTicket      ResourceKind = "match_ticket"
	ResourceBackfillTicket   ResourceKind = "backfill_ticket"
	ResourcePlanningSnapshot ResourceKind = "planning_snapshot"
	ResourcePlanningRun      ResourceKind = "planning_run"
	ResourceProposal         ResourceKind = "proposal"
	ResourceReservation      ResourceKind = "reservation"
	ResourceAssignment       ResourceKind = "assignment"
	ResourceAcknowledgment   ResourceKind = "assignment_acknowledgment"
)

func (kind ResourceKind) Valid() bool {
	switch kind {
	case ResourcePolicy,
		ResourceMatchTicket,
		ResourceBackfillTicket,
		ResourcePlanningSnapshot,
		ResourcePlanningRun,
		ResourceProposal,
		ResourceReservation,
		ResourceAssignment,
		ResourceAcknowledgment:
		return true
	default:
		return false
	}
}

// Key maps one tenant-scoped service resource to the repository identity model.
func Key(scope string, kind ResourceKind, id string) repository.Key {
	return repository.Key{Scope: scope, Kind: string(kind), ID: id}
}

// PlanningSnapshot binds matcher input to the repository version that produced it.
// The matcher still receives only the immutable domain snapshot.
type PlanningSnapshot struct {
	repositoryVersion repository.Version
	input             domain.MatchmakingSnapshot
}

func NewPlanningSnapshot(
	version repository.Version,
	input domain.MatchmakingSnapshot,
) (PlanningSnapshot, error) {
	if version == 0 {
		return PlanningSnapshot{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"repository version is required",
		)
	}
	if err := domain.ValidateSnapshot(input); err != nil {
		return PlanningSnapshot{}, err
	}
	return PlanningSnapshot{repositoryVersion: version, input: cloneSnapshot(input)}, nil
}

func (snapshot PlanningSnapshot) RepositoryVersion() repository.Version {
	return snapshot.repositoryVersion
}

func (snapshot PlanningSnapshot) MatchmakingInput() domain.MatchmakingSnapshot {
	return cloneSnapshot(snapshot.input)
}

func cloneSnapshot(snapshot domain.MatchmakingSnapshot) domain.MatchmakingSnapshot {
	cloned := snapshot
	cloned.Policy = domain.ClonePolicy(snapshot.Policy)
	cloned.MatchTickets = make([]domain.MatchTicket, len(snapshot.MatchTickets))
	for index, ticket := range snapshot.MatchTickets {
		cloned.MatchTickets[index] = domain.CloneMatchTicket(ticket)
	}
	cloned.BackfillTickets = make([]domain.BackfillTicket, len(snapshot.BackfillTickets))
	for index, ticket := range snapshot.BackfillTickets {
		cloned.BackfillTickets[index] = domain.CloneBackfillTicket(ticket)
	}
	return cloned
}

func cloneTickets(tickets []domain.MatchTicket) []domain.MatchTicket {
	cloned := make([]domain.MatchTicket, len(tickets))
	for index, ticket := range tickets {
		cloned[index] = domain.CloneMatchTicket(ticket)
	}
	return slices.Clip(cloned)
}
