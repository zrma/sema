// Package flow drives a deterministic, embedded HTTP lifecycle for the Sema Flow TUI.
package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
)

const (
	defaultTeamCount       = 2
	defaultTeamSize        = 5
	defaultMatchesPerCycle = 2
	defaultReservationTTL  = 30 * time.Second
)

// EventKind identifies a lifecycle transition rendered by the TUI.
type EventKind string

const (
	EventTicketAccepted      EventKind = "ticket_accepted"
	EventPlanCompleted       EventKind = "plan_completed"
	EventProposalReserved    EventKind = "proposal_reserved"
	EventAssignmentConfirmed EventKind = "assignment_confirmed"
	EventMatchDeparted       EventKind = "match_departed"
)

// Config controls the deterministic embedded workload.
type Config struct {
	Seed            int64
	Start           time.Time
	MatchesPerCycle int
	ReservationTTL  time.Duration
}

// DefaultConfig returns the reference 5v5 mixed-party demo configuration.
func DefaultConfig() Config {
	return Config{
		Seed:            42,
		Start:           time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		MatchesPerCycle: defaultMatchesPerCycle,
		ReservationTTL:  defaultReservationTTL,
	}
}

// Event is a defensive lifecycle result produced by one simulator step.
type Event struct {
	Kind           EventKind
	At             time.Time
	Cycle          int
	Ticket         *api.MatchTicket
	Batch          *api.ProposalBatch
	Proposal       *api.MatchProposal
	Reservation    *api.Reservation
	Assignment     *api.Assignment
	QueueTickets   int
	QueuePlayers   int
	MaxCandidates  int
	MaxSearchNodes int
}

type operationKind uint8

const (
	operationReserve operationKind = iota + 1
	operationConfirm
	operationAcknowledge
)

type operation struct {
	kind       operationKind
	proposalID string
}

type lifecycle struct {
	proposal      api.MatchProposal
	reservationID string
	assignmentID  string
	reservation   api.Reservation
	assignment    api.Assignment
}

type demoClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *demoClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *demoClock) Advance(duration time.Duration) time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
	return clock.now
}

// Simulator owns an isolated durable runtime and drives it exclusively through v0alpha1 HTTP.
type Simulator struct {
	mu sync.Mutex

	configuration Config
	clock         *demoClock
	directory     string
	runtime       *durable.Runtime
	server        *httptest.Server
	client        *http.Client

	policy        api.MatchmakingPolicy
	nextTicket    int
	nextPlayer    int
	cycle         int
	queued        map[string]api.MatchTicket
	queuedPlayers int
	active        map[string]*lifecycle
	pending       []operation
	arrivalTurn   bool
	forceArrival  bool
	closed        bool
	closeOnce     sync.Once
}

// Open creates an isolated journal, loopback HTTP server, and registered demo policy.
func Open(configuration Config) (*Simulator, error) {
	normalized, err := normalizeConfig(configuration)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "sema-flow-")
	if err != nil {
		return nil, fmt.Errorf("create flow runtime directory: %w", err)
	}
	runtime, err := durable.Open(filepath.Join(directory, "sema.journal"), normalized.ReservationTTL)
	if err != nil {
		_ = os.RemoveAll(directory)
		return nil, fmt.Errorf("open flow runtime: %w", err)
	}

	clock := &demoClock{now: normalized.Start}
	server := httptest.NewServer(httpapi.NewWithClock(runtime, clock.Now))
	simulator := &Simulator{
		configuration: normalized,
		clock:         clock,
		directory:     directory,
		runtime:       runtime,
		server:        server,
		client:        server.Client(),
		queued:        make(map[string]api.MatchTicket),
		active:        make(map[string]*lifecycle),
	}
	simulator.policy = api.MatchmakingPolicy{
		Version:                  "flow-policy-v1",
		TeamCount:                defaultTeamCount,
		TeamSize:                 defaultTeamSize,
		MaxLatencyMillis:         200,
		MaxProposals:             normalized.MatchesPerCycle,
		MaxSearchNodes:           100_000,
		MaxCandidateTickets:      256,
		MaxCandidatesPerProposal: 64,
	}
	if _, err := request[api.PolicyRegistration](
		context.Background(), simulator.client, http.MethodPut,
		server.URL+"/v0alpha1/policies/"+url.PathEscape(simulator.policy.Version), simulator.policy,
	); err != nil {
		_ = simulator.Close()
		return nil, fmt.Errorf("register flow policy: %w", err)
	}
	return simulator, nil
}

func normalizeConfig(configuration Config) (Config, error) {
	defaults := DefaultConfig()
	if configuration.Start.IsZero() {
		configuration.Start = defaults.Start
	}
	if configuration.MatchesPerCycle == 0 {
		configuration.MatchesPerCycle = defaults.MatchesPerCycle
	}
	if configuration.ReservationTTL == 0 {
		configuration.ReservationTTL = defaults.ReservationTTL
	}
	if configuration.Seed < 0 || configuration.MatchesPerCycle <= 0 || configuration.MatchesPerCycle > 8 || configuration.ReservationTTL <= 0 {
		return Config{}, fmt.Errorf("flow seed, matches per cycle, or reservation TTL is invalid")
	}
	return configuration, nil
}

// Close stops the embedded server, closes the journal, and removes private demo state.
func (simulator *Simulator) Close() error {
	var result error
	simulator.closeOnce.Do(func() {
		simulator.mu.Lock()
		simulator.closed = true
		simulator.mu.Unlock()
		simulator.server.Close()
		result = errors.Join(simulator.runtime.Close(), os.RemoveAll(simulator.directory))
	})
	return result
}

// Seed returns the deterministic workload seed.
func (simulator *Simulator) Seed() int64 {
	return simulator.configuration.Seed
}

// Step executes exactly one serialized HTTP lifecycle operation.
func (simulator *Simulator) Step(ctx context.Context) (Event, error) {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()
	if simulator.closed {
		return Event{}, fmt.Errorf("flow simulator is closed")
	}

	if len(simulator.pending) > 0 {
		if simulator.arrivalTurn {
			simulator.arrivalTurn = false
			return simulator.submitTicket(ctx)
		}
		current := simulator.pending[0]
		simulator.pending = simulator.pending[1:]
		simulator.arrivalTurn = true
		return simulator.execute(ctx, current)
	}

	playersPerCycle := defaultTeamCount * defaultTeamSize * simulator.configuration.MatchesPerCycle
	if simulator.forceArrival || simulator.queuedPlayers < playersPerCycle {
		simulator.forceArrival = false
		return simulator.submitTicket(ctx)
	}
	return simulator.plan(ctx)
}

func (simulator *Simulator) submitTicket(ctx context.Context) (Event, error) {
	partyPattern := [...]int{2, 1, 1, 1, 3, 2}
	partySize := partyPattern[simulator.nextTicket%len(partyPattern)]
	simulator.nextTicket++
	now := simulator.clock.Advance(75 * time.Millisecond)
	ticketID := fmt.Sprintf("flow-ticket-%04d", simulator.nextTicket)
	ticket := api.MatchTicket{
		ID:         ticketID,
		Revision:   1,
		EnqueuedAt: now,
		Players:    make([]api.Player, 0, partySize),
	}
	roles := [...]string{"tank", "damage", "support", "damage", "flex"}
	for range partySize {
		simulator.nextPlayer++
		ordinal := int64(simulator.nextPlayer)
		ticket.Players = append(ticket.Players, api.Player{
			ID:            fmt.Sprintf("flow-player-%05d", simulator.nextPlayer),
			Skill:         950 + deterministic(simulator.configuration.Seed, ordinal*37, 101),
			Role:          roles[(simulator.nextPlayer-1)%len(roles)],
			LatencyMillis: 18 + deterministic(simulator.configuration.Seed, ordinal*19, 43),
		})
	}
	if _, err := request[api.MutationResult](
		ctx, simulator.client, http.MethodPut,
		simulator.server.URL+"/v0alpha1/match-tickets/"+url.PathEscape(ticket.ID), ticket,
	); err != nil {
		return Event{}, fmt.Errorf("submit %s: %w", ticket.ID, err)
	}
	simulator.queued[ticket.ID] = ticket
	simulator.queuedPlayers += len(ticket.Players)
	copy := ticket
	return simulator.event(Event{Kind: EventTicketAccepted, At: now, Ticket: &copy}), nil
}

func deterministic(seed, ordinal int64, modulus int) int {
	value := (seed*31 + ordinal*17 + 97) % int64(modulus)
	return int(value)
}

func (simulator *Simulator) plan(ctx context.Context) (Event, error) {
	now := simulator.clock.Advance(100 * time.Millisecond)
	nextCycle := simulator.cycle + 1
	snapshotID := fmt.Sprintf("flow-snapshot-%04d", nextCycle)
	batch, err := request[api.ProposalBatch](
		ctx, simulator.client, http.MethodPost, simulator.server.URL+"/v0alpha1/plans",
		api.PlanRequest{SnapshotID: snapshotID, PolicyVersion: simulator.policy.Version},
	)
	if err != nil {
		return Event{}, fmt.Errorf("plan cycle %d: %w", nextCycle, err)
	}
	simulator.cycle = nextCycle
	for index, proposal := range batch.Proposals {
		state := &lifecycle{
			proposal:      proposal,
			reservationID: fmt.Sprintf("flow-reservation-%04d-%02d", simulator.cycle, index+1),
			assignmentID:  fmt.Sprintf("flow-assignment-%04d-%02d", simulator.cycle, index+1),
		}
		simulator.active[proposal.ID] = state
		simulator.pending = append(simulator.pending, operation{kind: operationReserve, proposalID: proposal.ID})
	}
	for _, proposal := range batch.Proposals {
		simulator.pending = append(simulator.pending, operation{kind: operationConfirm, proposalID: proposal.ID})
	}
	for _, proposal := range batch.Proposals {
		simulator.pending = append(simulator.pending, operation{kind: operationAcknowledge, proposalID: proposal.ID})
	}
	simulator.arrivalTurn = len(simulator.pending) > 0
	simulator.forceArrival = len(batch.Proposals) == 0
	copy := batch
	return simulator.event(Event{
		Kind: EventPlanCompleted, At: now, Cycle: simulator.cycle, Batch: &copy,
		MaxCandidates: simulator.policy.MaxCandidateTickets, MaxSearchNodes: simulator.policy.MaxSearchNodes,
	}), nil
}

func (simulator *Simulator) execute(ctx context.Context, current operation) (Event, error) {
	state, exists := simulator.active[current.proposalID]
	if !exists {
		return Event{}, fmt.Errorf("flow proposal %q is not active", current.proposalID)
	}
	now := simulator.clock.Advance(100 * time.Millisecond)
	proposal := state.proposal
	switch current.kind {
	case operationReserve:
		reservation, err := request[api.Reservation](
			ctx, simulator.client, http.MethodPost,
			simulator.server.URL+"/v0alpha1/reservations/"+url.PathEscape(state.reservationID),
			api.ReserveRequest{ProposalID: proposal.ID},
		)
		if err != nil {
			return Event{}, fmt.Errorf("reserve %s: %w", proposal.ID, err)
		}
		state.reservation = reservation
		return simulator.event(Event{
			Kind: EventProposalReserved, At: now, Cycle: simulator.cycle,
			Proposal: &proposal, Reservation: &reservation,
		}), nil
	case operationConfirm:
		assignment, err := request[api.Assignment](
			ctx, simulator.client, http.MethodPost,
			simulator.server.URL+"/v0alpha1/reservations/"+url.PathEscape(state.reservationID)+"/confirm",
			api.ConfirmRequest{AssignmentID: state.assignmentID},
		)
		if err != nil {
			return Event{}, fmt.Errorf("confirm %s: %w", proposal.ID, err)
		}
		state.assignment = assignment
		for _, reference := range proposal.Tickets {
			if ticket, queued := simulator.queued[reference.ID]; queued {
				simulator.queuedPlayers -= len(ticket.Players)
				delete(simulator.queued, reference.ID)
			}
		}
		return simulator.event(Event{
			Kind: EventAssignmentConfirmed, At: now, Cycle: simulator.cycle,
			Proposal: &proposal, Reservation: &state.reservation, Assignment: &assignment,
		}), nil
	case operationAcknowledge:
		assignment, err := request[api.Assignment](
			ctx, simulator.client, http.MethodPost,
			simulator.server.URL+"/v0alpha1/assignments/"+url.PathEscape(state.assignmentID)+"/acknowledgments",
			api.AcknowledgeAssignmentRequest{
				OperationID: state.assignmentID + "-completed",
				Outcome:     "completed",
			},
		)
		if err != nil {
			return Event{}, fmt.Errorf("acknowledge %s: %w", proposal.ID, err)
		}
		delete(simulator.active, current.proposalID)
		return simulator.event(Event{
			Kind: EventMatchDeparted, At: now, Cycle: simulator.cycle,
			Proposal: &proposal, Reservation: &state.reservation, Assignment: &assignment,
		}), nil
	default:
		return Event{}, fmt.Errorf("unknown flow operation %d", current.kind)
	}
}

func (simulator *Simulator) event(event Event) Event {
	event.QueueTickets = len(simulator.queued)
	event.QueuePlayers = simulator.queuedPlayers
	return event
}

func request[T any](ctx context.Context, client *http.Client, method, endpoint string, body any) (T, error) {
	var zero T
	var encoded io.Reader
	if body != nil {
		contents, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		encoded = bytes.NewReader(contents)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, encoded)
	if err != nil {
		return zero, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return zero, err
	}
	var envelope struct {
		APIVersion string          `json:"api_version"`
		Data       json.RawMessage `json:"data"`
		Error      *api.Failure    `json:"error"`
	}
	if err := json.Unmarshal(contents, &envelope); err != nil {
		return zero, fmt.Errorf("decode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Error != nil {
		if envelope.Error != nil {
			return zero, fmt.Errorf("service %s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return zero, fmt.Errorf("service status %d", response.StatusCode)
	}
	if envelope.APIVersion != api.Version {
		return zero, fmt.Errorf("service API version is %q", envelope.APIVersion)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return zero, fmt.Errorf("decode response data: %w", err)
	}
	return data, nil
}
