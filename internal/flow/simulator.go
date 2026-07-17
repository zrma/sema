// Package flow drives a deterministic, embedded HTTP lifecycle for the Sema Flow TUI.
package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
	"github.com/zrma/sema/internal/league"
)

const (
	defaultTeamCount        = 2
	defaultTeamSize         = 5
	defaultMatchesPerCycle  = 2
	defaultReservationTTL   = 30 * time.Second
	defaultGameDuration     = 45 * time.Second
	defaultArrivalInterval  = time.Second
	defaultPlanningInterval = 5 * time.Second
	defaultMaxReturnDelay   = 30 * time.Second
	defaultTickDuration     = time.Second
	operationDuration       = time.Second
)

// EventKind identifies a lifecycle transition rendered by the TUI.
type EventKind string

const (
	EventTicketQueued        EventKind = "ticket_queued"
	EventTicketReturned      EventKind = "ticket_returned"
	EventPlanCompleted       EventKind = "plan_completed"
	EventProposalReserved    EventKind = "proposal_reserved"
	EventAssignmentConfirmed EventKind = "assignment_confirmed"
	EventTimeAdvanced        EventKind = "time_advanced"
	EventMatchCompleted      EventKind = "match_completed"
)

// Config controls the deterministic embedded workload.
type Config struct {
	Seed             int64
	Start            time.Time
	PopulationSize   int
	MatchesPerCycle  int
	ReservationTTL   time.Duration
	GameDuration     time.Duration
	ArrivalInterval  time.Duration
	PlanningInterval time.Duration
	MaxReturnDelay   time.Duration
	TickDuration     time.Duration
}

// DefaultConfig returns the reference 1,000-player 5v5 league configuration.
func DefaultConfig() Config {
	return Config{
		Seed:             42,
		Start:            time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		PopulationSize:   1000,
		MatchesPerCycle:  defaultMatchesPerCycle,
		ReservationTTL:   defaultReservationTTL,
		GameDuration:     defaultGameDuration,
		ArrivalInterval:  defaultArrivalInterval,
		PlanningInterval: defaultPlanningInterval,
		MaxReturnDelay:   defaultMaxReturnDelay,
		TickDuration:     defaultTickDuration,
	}
}

// Event is a defensive lifecycle result produced by one simulator step.
type Event struct {
	Kind                  EventKind
	At                    time.Time
	Cycle                 int
	Ticket                *api.MatchTicket
	Batch                 *api.ProposalBatch
	Proposal              *api.MatchProposal
	Reservation           *api.Reservation
	Assignment            *api.Assignment
	Result                *league.Result
	ArrivalScheduledAt    time.Time
	GameStartedAt         time.Time
	GameEndsAt            time.Time
	QueueTickets          int
	QueuePlayers          int
	IngressBacklogTickets int
	IngressBacklogPlayers int
	ActiveMatches         int
	InGamePlayers         int
	IdlePlayers           int
	CooldownPlayers       int
	Population            league.Stats
	MaxCandidates         int
	MaxSearchNodes        int
}

// State is a defensive full read model used to initialize a renderer.
type State struct {
	Now                   time.Time
	Tickets               []api.MatchTicket
	QueueTickets          int
	QueuePlayers          int
	IngressBacklogTickets int
	IngressBacklogPlayers int
	ActiveMatches         int
	InGamePlayers         int
	IdlePlayers           int
	CooldownPlayers       int
	Population            league.Stats
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
	at         time.Time
}

type scheduledArrival struct {
	ticketID  string
	at        time.Time
	returning bool
}

type lifecycle struct {
	proposal      api.MatchProposal
	reservationID string
	assignmentID  string
	reservation   api.Reservation
	assignment    api.Assignment
	startedAt     time.Time
	endsAt        time.Time
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

// Simulator owns an isolated durable runtime and a closed player population.
type Simulator struct {
	mu sync.Mutex

	configuration Config
	clock         *demoClock
	directory     string
	runtime       *durable.Runtime
	server        *httptest.Server
	client        *http.Client
	population    *league.Population

	policy        api.MatchmakingPolicy
	cycle         int
	queued        map[string]api.MatchTicket
	queuedPlayers int
	active        map[string]*lifecycle
	pending       []operation
	arrivals      []scheduledArrival
	nextPlanAt    time.Time
	closed        bool
	closeOnce     sync.Once
}

// Open creates an isolated journal, loopback HTTP server, and an initially idle population.
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
	populationConfig := league.DefaultConfig()
	populationConfig.Seed = normalized.Seed
	populationConfig.PopulationSize = normalized.PopulationSize
	population, err := league.New(populationConfig)
	if err != nil {
		_ = runtime.Close()
		_ = os.RemoveAll(directory)
		return nil, fmt.Errorf("create flow population: %w", err)
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
		population:    population,
		queued:        make(map[string]api.MatchTicket),
		active:        make(map[string]*lifecycle),
		nextPlanAt:    normalized.Start,
	}
	simulator.policy = api.MatchmakingPolicy{
		Version:                  "flow-league-policy-v1",
		TeamCount:                defaultTeamCount,
		TeamSize:                 defaultTeamSize,
		MaxLatencyMillis:         200,
		MaxProposals:             normalized.MatchesPerCycle,
		MaxSearchNodes:           100_000,
		MaxCandidateTickets:      256,
		MaxCandidatesPerProposal: 64,
		RelaxationSteps: []api.RelaxationStep{
			{AfterWaitMillis: 0, MaxTeamSkillGap: 80},
			{AfterWaitMillis: 30_000, MaxTeamSkillGap: 200, PrioritizeWait: true},
			{AfterWaitMillis: 120_000, MaxTeamSkillGap: 400, PrioritizeWait: true},
		},
	}
	if _, err := request[api.PolicyRegistration](
		context.Background(), simulator.client, http.MethodPut,
		server.URL+"/v0alpha1/policies/"+url.PathEscape(simulator.policy.Version), simulator.policy,
	); err != nil {
		_ = simulator.Close()
		return nil, fmt.Errorf("register flow policy: %w", err)
	}
	for index, party := range population.Parties() {
		simulator.arrivals = append(simulator.arrivals, scheduledArrival{
			ticketID: party.ID,
			at:       normalized.Start.Add(time.Duration(index+1) * normalized.ArrivalInterval),
		})
	}
	simulator.sortArrivals()
	return simulator, nil
}

func normalizeConfig(configuration Config) (Config, error) {
	defaults := DefaultConfig()
	if configuration.Start.IsZero() {
		configuration.Start = defaults.Start
	}
	if configuration.PopulationSize == 0 {
		configuration.PopulationSize = defaults.PopulationSize
	}
	if configuration.MatchesPerCycle == 0 {
		configuration.MatchesPerCycle = defaults.MatchesPerCycle
	}
	if configuration.ReservationTTL == 0 {
		configuration.ReservationTTL = defaults.ReservationTTL
	}
	if configuration.GameDuration == 0 {
		configuration.GameDuration = defaults.GameDuration
	}
	if configuration.ArrivalInterval == 0 {
		configuration.ArrivalInterval = defaults.ArrivalInterval
	}
	if configuration.PlanningInterval == 0 {
		configuration.PlanningInterval = defaults.PlanningInterval
	}
	if configuration.MaxReturnDelay == 0 {
		configuration.MaxReturnDelay = defaults.MaxReturnDelay
	}
	if configuration.TickDuration == 0 {
		configuration.TickDuration = defaults.TickDuration
	}
	maximumMatches := configuration.PopulationSize / (defaultTeamCount * defaultTeamSize)
	if configuration.Seed < 0 || configuration.PopulationSize < 10 || configuration.MatchesPerCycle <= 0 ||
		configuration.MatchesPerCycle > 8 || configuration.MatchesPerCycle > maximumMatches || configuration.ReservationTTL <= 0 ||
		configuration.GameDuration <= 0 || configuration.ArrivalInterval <= 0 || configuration.PlanningInterval <= 0 ||
		configuration.MaxReturnDelay < time.Second || configuration.TickDuration <= 0 {
		return Config{}, fmt.Errorf("flow population, batch, timing, or seed configuration is invalid")
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

// Snapshot returns the current full waiting population and aggregate state.
func (simulator *Simulator) Snapshot() State {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()
	ingressTickets, ingressPlayers := simulator.ingressBacklog()
	state := State{
		Now:                   simulator.clock.Now(),
		QueueTickets:          len(simulator.queued),
		QueuePlayers:          simulator.queuedPlayers,
		IngressBacklogTickets: ingressTickets,
		IngressBacklogPlayers: ingressPlayers,
		Population:            simulator.population.Stats(),
		ActiveMatches:         simulator.activeGameCount(),
		InGamePlayers:         simulator.inGamePlayerCount(),
		IdlePlayers:           simulator.idlePlayerCount(),
		CooldownPlayers:       simulator.cooldownPlayerCount(),
		Tickets:               make([]api.MatchTicket, 0, len(simulator.queued)),
	}
	identifiers := make([]string, 0, len(simulator.queued))
	for identifier := range simulator.queued {
		identifiers = append(identifiers, identifier)
	}
	slices.Sort(identifiers)
	for _, identifier := range identifiers {
		state.Tickets = append(state.Tickets, cloneTicket(simulator.queued[identifier]))
	}
	return state
}

// Step emits one logical event while keeping presentation frames separate from simulated time.
func (simulator *Simulator) Step(ctx context.Context) (Event, error) {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()
	if simulator.closed {
		return Event{}, fmt.Errorf("flow simulator is closed")
	}
	if simulator.hasDueArrival() {
		return simulator.submitArrival(ctx)
	}
	if proposalID := simulator.nextCompletedGame(); proposalID != "" {
		return simulator.execute(ctx, operation{kind: operationAcknowledge, proposalID: proposalID, at: simulator.clock.Now()})
	}
	if current, due := simulator.nextDueOperation(); due {
		simulator.pending = simulator.pending[1:]
		return simulator.execute(ctx, current)
	}
	if simulator.canPlan(simulator.clock.Now()) {
		return simulator.plan(ctx)
	}
	return simulator.advanceTime(), nil
}

func (simulator *Simulator) plan(ctx context.Context) (Event, error) {
	now := simulator.clock.Now()
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
	simulator.nextPlanAt = now.Add(simulator.configuration.PlanningInterval)
	reserveAt := now.Add(operationDuration)
	confirmAt := reserveAt.Add(operationDuration)
	for index, proposal := range batch.Proposals {
		state := &lifecycle{
			proposal:      proposal,
			reservationID: fmt.Sprintf("flow-reservation-%04d-%02d", simulator.cycle, index+1),
			assignmentID:  fmt.Sprintf("flow-assignment-%04d-%02d", simulator.cycle, index+1),
		}
		simulator.active[proposal.ID] = state
		simulator.pending = append(simulator.pending, operation{kind: operationReserve, proposalID: proposal.ID, at: reserveAt})
	}
	for _, proposal := range batch.Proposals {
		simulator.pending = append(simulator.pending, operation{kind: operationConfirm, proposalID: proposal.ID, at: confirmAt})
	}
	simulator.sortPending()
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
	now := simulator.clock.Now()
	if current.at.After(now) {
		return Event{}, fmt.Errorf("flow operation for %q is scheduled in the future", current.proposalID)
	}
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
		state.startedAt = now
		state.endsAt = now.Add(simulator.configuration.GameDuration)
		for _, reference := range proposal.Tickets {
			if ticket, queued := simulator.queued[reference.ID]; queued {
				simulator.queuedPlayers -= len(ticket.Players)
				delete(simulator.queued, reference.ID)
			}
		}
		return simulator.event(Event{
			Kind: EventAssignmentConfirmed, At: now, Cycle: simulator.cycle,
			Proposal: &proposal, Reservation: &state.reservation, Assignment: &assignment,
			GameStartedAt: state.startedAt, GameEndsAt: state.endsAt,
		}), nil
	case operationAcknowledge:
		assignment, err := request[api.Assignment](
			ctx, simulator.client, http.MethodPost,
			simulator.server.URL+"/v0alpha1/assignments/"+url.PathEscape(state.assignmentID)+"/acknowledgments",
			api.AcknowledgeAssignmentRequest{OperationID: state.assignmentID + "-completed", Outcome: "completed"},
		)
		if err != nil {
			return Event{}, fmt.Errorf("acknowledge %s: %w", proposal.ID, err)
		}
		result, err := simulator.play(proposal)
		if err != nil {
			return Event{}, fmt.Errorf("resolve %s: %w", proposal.ID, err)
		}
		delete(simulator.active, current.proposalID)
		for _, reference := range proposal.Tickets {
			party, exists := simulator.population.Party(reference.ID)
			if !exists {
				return Event{}, fmt.Errorf("flow party %q does not exist", reference.ID)
			}
			simulator.scheduleArrival(scheduledArrival{
				ticketID:  reference.ID,
				at:        now.Add(simulator.returnDelay(reference.ID, party.Revision)),
				returning: true,
			})
		}
		return simulator.event(Event{
			Kind: EventMatchCompleted, At: now, Cycle: simulator.cycle,
			Proposal: &proposal, Reservation: &state.reservation, Assignment: &assignment, Result: &result,
			GameStartedAt: state.startedAt, GameEndsAt: state.endsAt,
		}), nil
	default:
		return Event{}, fmt.Errorf("unknown flow operation %d", current.kind)
	}
}

func (simulator *Simulator) submitArrival(ctx context.Context) (Event, error) {
	arrival := simulator.arrivals[0]
	simulator.arrivals = simulator.arrivals[1:]
	party, exists := simulator.population.Party(arrival.ticketID)
	if !exists {
		return Event{}, fmt.Errorf("flow party %q does not exist", arrival.ticketID)
	}
	// A due arrival is already scheduled on the server clock. Processing it is a
	// lifecycle event for presentation, not additional simulated queue time.
	now := simulator.clock.Now()
	ticket := simulator.ticket(party, now)
	if _, err := request[api.MutationResult](
		ctx, simulator.client, http.MethodPut,
		simulator.server.URL+"/v0alpha1/match-tickets/"+url.PathEscape(ticket.ID), ticket,
	); err != nil {
		return Event{}, fmt.Errorf("queue %s: %w", ticket.ID, err)
	}
	simulator.queued[ticket.ID] = ticket
	simulator.queuedPlayers += len(ticket.Players)
	copy := cloneTicket(ticket)
	kind := EventTicketQueued
	if arrival.returning {
		kind = EventTicketReturned
	}
	return simulator.event(Event{Kind: kind, At: now, Cycle: simulator.cycle, Ticket: &copy, ArrivalScheduledAt: arrival.at}), nil
}

func (simulator *Simulator) hasDueArrival() bool {
	return len(simulator.arrivals) > 0 && !simulator.arrivals[0].at.After(simulator.clock.Now())
}

func (simulator *Simulator) scheduleArrival(arrival scheduledArrival) {
	simulator.arrivals = append(simulator.arrivals, arrival)
	simulator.sortArrivals()
}

func (simulator *Simulator) nextDueOperation() (operation, bool) {
	if len(simulator.pending) == 0 || simulator.pending[0].at.After(simulator.clock.Now()) {
		return operation{}, false
	}
	return simulator.pending[0], true
}

func (simulator *Simulator) sortPending() {
	slices.SortFunc(simulator.pending, func(left, right operation) int {
		if compared := left.at.Compare(right.at); compared != 0 {
			return compared
		}
		if left.kind != right.kind {
			return int(left.kind) - int(right.kind)
		}
		return strings.Compare(left.proposalID, right.proposalID)
	})
}

func (simulator *Simulator) sortArrivals() {
	slices.SortFunc(simulator.arrivals, func(left, right scheduledArrival) int {
		if compared := left.at.Compare(right.at); compared != 0 {
			return compared
		}
		return strings.Compare(left.ticketID, right.ticketID)
	})
}

func (simulator *Simulator) returnDelay(ticketID string, revision uint64) time.Duration {
	bucket := stableMetric(simulator.configuration.Seed, fmt.Sprintf("%s-r%d-return-bucket", ticketID, revision), 100)
	if bucket < 20 {
		return 0
	}
	minimum := 5 * time.Second
	if bucket >= 70 {
		minimum = 20 * time.Second
	}
	maximum := simulator.configuration.MaxReturnDelay
	if minimum >= maximum {
		return maximum
	}
	seconds := int((maximum-minimum)/time.Second) + 1
	offset := stableMetric(simulator.configuration.Seed, fmt.Sprintf("%s-r%d-return-offset", ticketID, revision), seconds)
	return minimum + time.Duration(offset)*time.Second
}

func (simulator *Simulator) play(proposal api.MatchProposal) (league.Result, error) {
	teams := [2][]string{}
	if len(proposal.Teams) != len(teams) {
		return league.Result{}, fmt.Errorf("proposal has %d teams; want 2", len(proposal.Teams))
	}
	for index, team := range proposal.Teams {
		for _, reference := range team.Tickets {
			teams[index] = append(teams[index], reference.ID)
		}
	}
	return simulator.population.Play(proposal.ID, teams)
}

func (simulator *Simulator) advanceTime() Event {
	current := simulator.clock.Now()
	next := current.Add(simulator.configuration.TickDuration)
	consider := func(candidate time.Time) {
		if !candidate.IsZero() && candidate.After(current) && candidate.Before(next) {
			next = candidate
		}
	}
	if len(simulator.arrivals) > 0 {
		consider(simulator.arrivals[0].at)
	}
	if len(simulator.pending) > 0 {
		consider(simulator.pending[0].at)
	}
	for _, match := range simulator.active {
		consider(match.endsAt)
	}
	if simulator.planningDemandReady() {
		consider(simulator.nextPlanAt)
	}
	now := simulator.clock.Advance(next.Sub(current))
	return simulator.event(Event{Kind: EventTimeAdvanced, At: now, Cycle: simulator.cycle})
}

func (simulator *Simulator) planningDemandReady() bool {
	return simulator.queuedPlayers >= defaultTeamCount*defaultTeamSize*simulator.configuration.MatchesPerCycle
}

func (simulator *Simulator) canPlan(now time.Time) bool {
	return simulator.planningDemandReady() && !now.Before(simulator.nextPlanAt)
}

func (simulator *Simulator) hasDueEvent() bool {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()
	return simulator.hasDueEventLocked()
}

func (simulator *Simulator) hasDueEventLocked() bool {
	if simulator.hasDueArrival() || simulator.nextCompletedGame() != "" {
		return true
	}
	if _, due := simulator.nextDueOperation(); due {
		return true
	}
	return simulator.canPlan(simulator.clock.Now())
}

func (simulator *Simulator) nextCompletedGame() string {
	now := simulator.clock.Now()
	identifiers := make([]string, 0, len(simulator.active))
	for identifier, match := range simulator.active {
		if !match.endsAt.IsZero() && !match.endsAt.After(now) {
			identifiers = append(identifiers, identifier)
		}
	}
	slices.Sort(identifiers)
	if len(identifiers) == 0 {
		return ""
	}
	return identifiers[0]
}

func (simulator *Simulator) ticket(party league.Party, enqueuedAt time.Time) api.MatchTicket {
	ticket := api.MatchTicket{ID: party.ID, Revision: party.Revision, EnqueuedAt: enqueuedAt, Players: make([]api.Player, 0, len(party.Players))}
	roles := [...]string{"tank", "damage", "support", "damage", "flex"}
	for _, player := range party.Players {
		roleIndex := stableMetric(simulator.configuration.Seed, player.ID+"-role", len(roles))
		ticket.Players = append(ticket.Players, api.Player{
			ID:            player.ID,
			Skill:         player.Rating,
			Role:          roles[roleIndex],
			LatencyMillis: 18 + stableMetric(simulator.configuration.Seed, player.ID+"-latency", 43),
		})
	}
	return ticket
}

func stableMetric(seed int64, value string, modulus int) int {
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "%d:%s", seed, value)
	return int(hash.Sum64() % uint64(modulus))
}

func (simulator *Simulator) activeGameCount() int {
	count := 0
	for _, match := range simulator.active {
		if !match.startedAt.IsZero() {
			count++
		}
	}
	return count
}

func (simulator *Simulator) inGamePlayerCount() int {
	players := 0
	for _, match := range simulator.active {
		if match.startedAt.IsZero() {
			continue
		}
		for _, team := range match.proposal.Teams {
			for _, reference := range team.Tickets {
				party, exists := simulator.population.Party(reference.ID)
				if exists {
					players += len(party.Players)
				}
			}
		}
	}
	return players
}

func (simulator *Simulator) cooldownPlayerCount() int {
	now := simulator.clock.Now()
	players := 0
	for _, arrival := range simulator.arrivals {
		if !arrival.returning || !arrival.at.After(now) {
			continue
		}
		party, exists := simulator.population.Party(arrival.ticketID)
		if exists {
			players += len(party.Players)
		}
	}
	return players
}

func (simulator *Simulator) ingressBacklog() (int, int) {
	now := simulator.clock.Now()
	tickets := 0
	players := 0
	for _, arrival := range simulator.arrivals {
		if arrival.at.After(now) {
			break
		}
		party, exists := simulator.population.Party(arrival.ticketID)
		if !exists {
			continue
		}
		tickets++
		players += len(party.Players)
	}
	return tickets, players
}

func (simulator *Simulator) idlePlayerCount() int {
	_, ingressPlayers := simulator.ingressBacklog()
	return max(0, simulator.configuration.PopulationSize-simulator.queuedPlayers-ingressPlayers-simulator.inGamePlayerCount()-simulator.cooldownPlayerCount())
}

func (simulator *Simulator) event(event Event) Event {
	ingressTickets, ingressPlayers := simulator.ingressBacklog()
	event.QueueTickets = len(simulator.queued)
	event.QueuePlayers = simulator.queuedPlayers
	event.IngressBacklogTickets = ingressTickets
	event.IngressBacklogPlayers = ingressPlayers
	event.ActiveMatches = simulator.activeGameCount()
	event.InGamePlayers = simulator.inGamePlayerCount()
	event.IdlePlayers = simulator.idlePlayerCount()
	event.CooldownPlayers = simulator.cooldownPlayerCount()
	event.Population = simulator.population.Stats()
	return event
}

func cloneTicket(ticket api.MatchTicket) api.MatchTicket {
	ticket.Players = slices.Clone(ticket.Players)
	return ticket
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
