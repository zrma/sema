// Package flowui renders the deterministic Flow simulator as an interactive terminal UI.
package flowui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/flow"
)

const frameInterval = 70 * time.Millisecond

type ticketState string

const (
	ticketQueued    ticketState = "queued"
	ticketProposed  ticketState = "proposed"
	ticketReserved  ticketState = "reserved"
	ticketConfirmed ticketState = "confirmed"
)

type matchStage string

const (
	stageProposed  matchStage = "proposed"
	stageReserved  matchStage = "reserved"
	stageConfirmed matchStage = "confirmed"
	stageCompleted matchStage = "completed"
)

// Options controls terminal presentation without changing simulator authority.
type Options struct {
	Context       context.Context
	StepInterval  time.Duration
	Width         int
	Height        int
	Unicode       bool
	Color         bool
	ReducedMotion bool
	Seed          int64
}

// DefaultOptions returns the full-screen Unicode presentation baseline.
func DefaultOptions() Options {
	return Options{
		Context:      context.Background(),
		StepInterval: 220 * time.Millisecond,
		Width:        120,
		Height:       38,
		Unicode:      true,
		Color:        true,
	}
}

type ticketView struct {
	ticket   api.MatchTicket
	state    ticketState
	position int
}

type matchView struct {
	proposal   api.MatchProposal
	stage      matchStage
	assignment api.Assignment
	motion     int
	partySizes map[string]int
}

type frameMsg time.Time

type eventMsg struct {
	event flow.Event
}

type failureMsg struct {
	err error
}

// Model is the Bubble Tea state model for Sema Flow.
type Model struct {
	simulator *flow.Simulator
	options   Options

	width        int
	height       int
	paused       bool
	inFlight     bool
	singleStep   bool
	frame        int
	nextStepAt   time.Time
	working      string
	lastError    error
	now          time.Time
	queueTickets int
	queuePlayers int
	cycle        int

	tickets     map[string]*ticketView
	ticketOrder []string
	active      map[string]*matchView
	activeOrder []string
	history     []matchView
	logs        []string

	lastCandidateTickets int
	lastCandidatesMax    int
	lastSearchNodes      int
	lastSearchMax        int
}

// New creates a TUI model over an already-open simulator.
func New(simulator *flow.Simulator, options Options) *Model {
	defaults := DefaultOptions()
	if options.Context == nil {
		options.Context = defaults.Context
	}
	if options.StepInterval <= 0 {
		options.StepInterval = defaults.StepInterval
	}
	if options.Width <= 0 {
		options.Width = defaults.Width
	}
	if options.Height <= 0 {
		options.Height = defaults.Height
	}
	return &Model{
		simulator: simulator,
		options:   options,
		width:     options.Width,
		height:    options.Height,
		tickets:   make(map[string]*ticketView),
		active:    make(map[string]*matchView),
	}
}

// Init starts the animation clock and the first real lifecycle operation.
func (model *Model) Init() tea.Cmd {
	model.inFlight = true
	model.working = "advancing lifecycle"
	return tea.Batch(frameTick(), model.advance())
}

// Update applies terminal input, animation frames, and simulator events.
func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = max(message.Width, 40)
		model.height = max(message.Height, 18)
	case tea.KeyPressMsg:
		switch message.String() {
		case "ctrl+c", "q":
			return model, tea.Quit
		case "space":
			model.paused = !model.paused
			if !model.paused {
				model.nextStepAt = time.Time{}
			}
		case "n":
			if model.paused && !model.inFlight {
				model.inFlight = true
				model.singleStep = true
				model.working = "single step"
				return model, model.advance()
			}
		case "+", "=":
			model.options.StepInterval = max(50*time.Millisecond, model.options.StepInterval*4/5)
		case "-", "_":
			model.options.StepInterval = min(2*time.Second, model.options.StepInterval*5/4)
		case "u":
			model.options.Unicode = !model.options.Unicode
		case "m":
			model.options.ReducedMotion = !model.options.ReducedMotion
		}
	case frameMsg:
		now := time.Time(message)
		model.frame++
		model.animate()
		commands := []tea.Cmd{frameTick()}
		if !model.paused && !model.inFlight && (model.nextStepAt.IsZero() || !now.Before(model.nextStepAt)) {
			model.inFlight = true
			model.working = "advancing lifecycle"
			model.nextStepAt = now.Add(model.options.StepInterval)
			commands = append(commands, model.advance())
		}
		return model, tea.Batch(commands...)
	case eventMsg:
		model.inFlight = false
		model.working = ""
		model.apply(message.event)
		if model.singleStep {
			model.singleStep = false
			model.paused = true
		}
	case failureMsg:
		model.inFlight = false
		model.working = ""
		model.lastError = message.err
		model.paused = true
	}
	return model, nil
}

// View renders the full-window alternate-screen view.
func (model *Model) View() tea.View {
	view := tea.NewView(model.Content())
	view.AltScreen = true
	view.WindowTitle = "Sema Flow"
	return view
}

// Content renders the current model without changing terminal state.
func (model *Model) Content() string {
	return model.render()
}

// RunSteps executes a deterministic number of operations for snapshot output and tests.
func (model *Model) RunSteps(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("snapshot steps must be positive")
	}
	for range steps {
		event, err := model.simulator.Step(ctx)
		if err != nil {
			return err
		}
		model.apply(event)
		model.animateToEnd()
	}
	return nil
}

func frameTick() tea.Cmd {
	return tea.Tick(frameInterval, func(now time.Time) tea.Msg { return frameMsg(now) })
}

func (model *Model) advance() tea.Cmd {
	return func() tea.Msg {
		event, err := model.simulator.Step(model.options.Context)
		if err != nil {
			return failureMsg{err: err}
		}
		return eventMsg{event: event}
	}
}

func (model *Model) apply(event flow.Event) {
	model.now = event.At
	model.queueTickets = event.QueueTickets
	model.queuePlayers = event.QueuePlayers
	if event.Cycle > 0 {
		model.cycle = event.Cycle
	}

	switch event.Kind {
	case flow.EventTicketAccepted:
		if event.Ticket == nil {
			return
		}
		ticket := *event.Ticket
		position := 0
		if model.options.ReducedMotion {
			position = 6
		}
		model.tickets[ticket.ID] = &ticketView{ticket: ticket, state: ticketQueued, position: position}
		model.ticketOrder = append(model.ticketOrder, ticket.ID)
		model.logf("+ %s %s accepted", shortID(ticket.ID), model.partyGlyph(len(ticket.Players)))
	case flow.EventPlanCompleted:
		if event.Batch == nil {
			return
		}
		model.lastCandidateTickets = 0
		model.lastSearchNodes = 0
		model.lastCandidatesMax = event.MaxCandidates
		model.lastSearchMax = event.MaxSearchNodes
		for _, proposal := range event.Batch.Proposals {
			proposal := proposal
			motion := 0
			if model.options.ReducedMotion {
				motion = 8
			}
			partySizes := make(map[string]int, len(proposal.Tickets))
			for _, reference := range proposal.Tickets {
				if ticket := model.tickets[reference.ID]; ticket != nil {
					partySizes[reference.ID] = len(ticket.ticket.Players)
				}
			}
			model.active[proposal.ID] = &matchView{
				proposal: proposal, stage: stageProposed, motion: motion, partySizes: partySizes,
			}
			model.activeOrder = append(model.activeOrder, proposal.ID)
			model.lastCandidateTickets = max(model.lastCandidateTickets, proposal.Evidence.CandidateTickets)
			model.lastSearchNodes += proposal.Evidence.SearchNodes
			model.setTicketState(proposal.Tickets, ticketProposed)
		}
		model.logf("o cycle %04d formed %d proposals", event.Cycle, len(event.Batch.Proposals))
	case flow.EventProposalReserved:
		if event.Proposal == nil {
			return
		}
		if match := model.active[event.Proposal.ID]; match != nil {
			match.stage = stageReserved
			match.motion = 0
			model.setTicketState(match.proposal.Tickets, ticketReserved)
		}
		model.logf("* %s reserved", matchLabel(event.Proposal.ID))
	case flow.EventAssignmentConfirmed:
		if event.Proposal == nil || event.Assignment == nil {
			return
		}
		if match := model.active[event.Proposal.ID]; match != nil {
			match.stage = stageConfirmed
			match.assignment = *event.Assignment
			match.motion = 0
			model.setTicketState(match.proposal.Tickets, ticketConfirmed)
		}
		model.logf("# %s confirmed", matchLabel(event.Proposal.ID))
	case flow.EventMatchDeparted:
		if event.Proposal == nil || event.Assignment == nil {
			return
		}
		match := model.active[event.Proposal.ID]
		if match == nil {
			return
		}
		match.stage = stageCompleted
		match.assignment = *event.Assignment
		match.motion = 8
		model.history = append([]matchView{*match}, model.history...)
		if len(model.history) > 8 {
			model.history = model.history[:8]
		}
		delete(model.active, event.Proposal.ID)
		model.activeOrder = deleteString(model.activeOrder, event.Proposal.ID)
		for _, reference := range event.Proposal.Tickets {
			delete(model.tickets, reference.ID)
			model.ticketOrder = deleteString(model.ticketOrder, reference.ID)
		}
		model.logf("+ %s departed", matchLabel(event.Proposal.ID))
	}
}

func (model *Model) setTicketState(references []api.TicketRef, state ticketState) {
	for _, reference := range references {
		if ticket := model.tickets[reference.ID]; ticket != nil {
			ticket.state = state
		}
	}
}

func (model *Model) animate() {
	if model.options.ReducedMotion {
		model.animateToEnd()
		return
	}
	for _, ticket := range model.tickets {
		if ticket.position < 6 {
			ticket.position++
		}
	}
	for _, match := range model.active {
		if match.motion < 8 {
			match.motion++
		}
	}
}

func (model *Model) animateToEnd() {
	for _, ticket := range model.tickets {
		ticket.position = 6
	}
	for _, match := range model.active {
		match.motion = 8
	}
}

func (model *Model) logf(format string, arguments ...any) {
	separator := " · "
	if !model.options.Unicode {
		separator = " | "
	}
	timestamp := model.now.Format("15:04:05")
	entry := timestamp + separator + fmt.Sprintf(format, arguments...)
	model.logs = append(model.logs, entry)
	if len(model.logs) > 10 {
		model.logs = slices.Clone(model.logs[len(model.logs)-10:])
	}
}

func deleteString(values []string, target string) []string {
	for index, value := range values {
		if value == target {
			return append(values[:index], values[index+1:]...)
		}
	}
	return values
}

func shortID(identifier string) string {
	identifier = strings.TrimPrefix(identifier, "flow-")
	if len(identifier) <= 16 {
		return identifier
	}
	return identifier[:8]
}

func matchLabel(identifier string) string {
	parts := strings.Split(identifier, "/")
	if len(parts) >= 2 {
		cycle := strings.TrimPrefix(parts[0], "flow-snapshot-")
		proposal := strings.TrimPrefix(parts[1], "p")
		cycle = strings.TrimLeft(cycle, "0")
		proposal = strings.TrimLeft(proposal, "0")
		if cycle == "" {
			cycle = "0"
		}
		if proposal == "" {
			proposal = "0"
		}
		return "C" + cycle + "/P" + proposal
	}
	return shortID(identifier)
}
