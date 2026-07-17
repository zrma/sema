// Package evaluation owns synthetic workload generation and planner quality evidence.
package evaluation

import (
	"fmt"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/simulation"
)

type PartySizeWeight struct {
	Size   int
	Weight int
}

type RoleWeight struct {
	Role   string
	Weight int
}

// WorkloadModel describes a deterministic queue snapshot, not a production traffic claim.
type WorkloadModel struct {
	ID           domain.SnapshotID
	Seed         uint64
	Now          time.Time
	TicketCount  int
	MaxPartySize int
	PartySizes   []PartySizeWeight
	SkillCenter  int
	SkillSpread  int
	Roles        []RoleWeight
	MinLatencyMS int
	MaxLatencyMS int
	MinWait      time.Duration
	MaxWait      time.Duration
}

// Generate creates the same immutable scenario for the same validated model.
func Generate(model WorkloadModel) (simulation.Scenario, error) {
	if err := validateModel(model); err != nil {
		return simulation.Scenario{}, err
	}

	random := newGenerator(model.Seed)
	tickets := make([]domain.MatchTicket, model.TicketCount)
	playerSequence := 0
	for ticketIndex := range tickets {
		partySize := choosePartySize(&random, model.PartySizes)
		players := make([]domain.Player, partySize)
		for playerIndex := range players {
			players[playerIndex] = domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("%s-player-%05d", model.ID, playerSequence)),
				Skill:         model.SkillCenter + random.centered(model.SkillSpread),
				Role:          chooseRole(&random, model.Roles),
				LatencyMillis: random.between(model.MinLatencyMS, model.MaxLatencyMS),
			}
			playerSequence++
		}
		wait := random.durationBetween(model.MinWait, model.MaxWait)
		tickets[ticketIndex] = domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("%s-ticket-%04d", model.ID, ticketIndex)),
			Revision:   1,
			EnqueuedAt: model.Now.Add(-wait),
			Players:    players,
		}
	}
	return simulation.Scenario{ID: model.ID, Now: model.Now, MatchTickets: tickets}, nil
}

func validateModel(model WorkloadModel) error {
	if model.ID == "" || model.Now.IsZero() || model.TicketCount <= 0 {
		return domain.NewFailure(domain.FailureInvalidInput, "workload model identity, time, and positive ticket count are required")
	}
	if model.MaxPartySize <= 0 || len(model.PartySizes) == 0 {
		return domain.NewFailure(domain.FailureInvalidInput, "workload model needs a positive party limit and distribution")
	}
	for _, candidate := range model.PartySizes {
		if candidate.Size <= 0 || candidate.Size > model.MaxPartySize || candidate.Weight <= 0 {
			return domain.NewFailure(domain.FailureInvalidInput, "party sizes need a feasible size and positive weight")
		}
	}
	if model.SkillCenter < 0 || model.SkillSpread < 0 || model.SkillSpread > model.SkillCenter {
		return domain.NewFailure(domain.FailureInvalidInput, "skill center and spread must produce non-negative ratings")
	}
	for _, candidate := range model.Roles {
		if candidate.Role == "" || candidate.Weight <= 0 {
			return domain.NewFailure(domain.FailureInvalidInput, "roles need a name and positive weight")
		}
	}
	if model.MinLatencyMS < 0 || model.MaxLatencyMS < model.MinLatencyMS {
		return domain.NewFailure(domain.FailureInvalidInput, "latency range is invalid")
	}
	if model.MinWait < 0 || model.MaxWait < model.MinWait {
		return domain.NewFailure(domain.FailureInvalidInput, "wait range is invalid")
	}
	return nil
}

func choosePartySize(random *generator, candidates []PartySizeWeight) int {
	total := 0
	for _, candidate := range candidates {
		total += candidate.Weight
	}
	selected := random.between(1, total)
	for _, candidate := range candidates {
		selected -= candidate.Weight
		if selected <= 0 {
			return candidate.Size
		}
	}
	panic("unreachable party distribution")
}

func chooseRole(random *generator, candidates []RoleWeight) string {
	if len(candidates) == 0 {
		return ""
	}
	total := 0
	for _, candidate := range candidates {
		total += candidate.Weight
	}
	selected := random.between(1, total)
	for _, candidate := range candidates {
		selected -= candidate.Weight
		if selected <= 0 {
			return candidate.Role
		}
	}
	panic("unreachable role distribution")
}

// generator uses SplitMix64 so fixture generation does not depend on math/rand changes.
type generator struct {
	state uint64
}

func newGenerator(seed uint64) generator {
	return generator{state: seed}
}

func (random *generator) next() uint64 {
	random.state += 0x9e3779b97f4a7c15
	value := random.state
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func (random *generator) between(minimum, maximum int) int {
	if minimum == maximum {
		return minimum
	}
	return minimum + int(random.next()%uint64(maximum-minimum+1))
}

func (random *generator) centered(spread int) int {
	return random.between(-spread, spread)
}

func (random *generator) durationBetween(minimum, maximum time.Duration) time.Duration {
	if minimum == maximum {
		return minimum
	}
	return minimum + time.Duration(random.next()%uint64(maximum-minimum+1))
}
