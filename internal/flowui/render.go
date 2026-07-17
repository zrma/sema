package flowui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	api "github.com/zrma/sema/internal/api/v0alpha1"
)

type glyphSet struct {
	player    string
	link      string
	dot       string
	arrow     string
	track     string
	fill      string
	empty     string
	proposed  string
	reserved  string
	confirmed string
	departed  string
	ellipsis  string
	separator string
	versus    string
	spinners  []string
	border    lipgloss.Border
}

func (model *Model) glyphs() glyphSet {
	if !model.options.Unicode {
		return glyphSet{
			player: "o", link: "-", dot: ".", arrow: ">", track: "=", fill: "#", empty: "-",
			proposed: "o", reserved: "*", confirmed: "+", departed: "OK", ellipsis: "...",
			separator: " | ", versus: " vs ", spinners: []string{"|", "/", "-", "\\"},
			border: lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"},
		}
	}
	return glyphSet{
		player: "●", link: "─", dot: "·", arrow: "▷", track: "━", fill: "█", empty: "░",
		proposed: "◉", reserved: "◆", confirmed: "✓", departed: "✓", ellipsis: "…",
		separator: " · ", versus: " vs ", spinners: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		border: lipgloss.RoundedBorder(),
	}
}

func (model *Model) render() string {
	glyphs := model.glyphs()
	width := max(model.width, 40)
	height := max(model.height, 18)

	header := model.renderHeader(glyphs, width)
	status := model.renderStatus(glyphs, width)
	if height < 30 {
		return model.renderCompact(glyphs, width, header, status)
	}
	mainHeight := max(8, min(14, height-18))

	var main string
	if width >= 96 {
		leftWidth := (width - 1) / 2
		rightWidth := width - leftWidth - 1
		main = lipgloss.JoinHorizontal(
			lipgloss.Top,
			model.panel("WAITING POOL", model.waitingLines(glyphs, mainHeight-3), leftWidth, glyphs),
			" ",
			model.panel("MATCH LIFECYCLE", model.activeLines(glyphs, mainHeight-3), rightWidth, glyphs),
		)
	} else {
		sectionHeight := max(6, mainHeight/2+2)
		main = lipgloss.JoinVertical(
			lipgloss.Left,
			model.panel("WAITING POOL", model.waitingLines(glyphs, sectionHeight-3), width, glyphs),
			model.panel("MATCH LIFECYCLE", model.activeLines(glyphs, sectionHeight-3), width, glyphs),
		)
	}

	history := model.panel("DEPARTED MATCHES", model.historyLines(glyphs, 4), width, glyphs)
	events := model.panel("EVENT STREAM", model.eventLines(glyphs, 5), width, glyphs)
	footer := model.paint(
		"[space] pause  [n] step  [+/-] speed  [u] unicode  [m] motion  [q] quit",
		"#7f8c98",
	)
	return strings.Join([]string{header, status, main, history, events, footer}, "\n")
}

func (model *Model) renderCompact(glyphs glyphSet, width int, header, status string) string {
	flowLines := []string{model.paint("waiting", "#9ca3af")}
	flowLines = append(flowLines, model.waitingLines(glyphs, 2)...)
	flowLines = append(flowLines, model.paint("lifecycle", "#9ca3af"))
	flowLines = append(flowLines, model.activeLines(glyphs, 4)...)
	recentLines := model.historyLines(glyphs, 1)
	recentLines = append(recentLines, model.eventLines(glyphs, 2)...)
	footer := "[space] pause  [n] step  [+/-] speed  [u] unicode  [m] motion  [q] quit"
	if width < 72 {
		footer = "space pause | n step | +/- speed | q quit"
	}
	return strings.Join([]string{
		header,
		status,
		model.panel("FLOW", flowLines, width, glyphs),
		model.panel("RECENT", recentLines, width, glyphs),
		model.paint(footer, "#7f8c98"),
	}, "\n")
}

func (model *Model) renderHeader(glyphs glyphSet, width int) string {
	state := "RUNNING"
	stateColor := "#57d38c"
	marker := glyphs.confirmed
	if model.paused {
		state = "PAUSED"
		stateColor = "#f0c36e"
		marker = "II"
	}
	if model.lastError != nil {
		state = "ERROR"
		stateColor = "#ff6b6b"
		marker = "!"
	}
	if model.inFlight {
		marker = glyphs.spinners[model.frame%len(glyphs.spinners)]
	}
	title := model.paint("SEMA FLOW", "#7dd3fc")
	right := fmt.Sprintf("%s %s  seed %d  step %s", marker, state, model.options.Seed, model.options.StepInterval.Round(time.Millisecond))
	right = model.paint(right, stateColor)
	if lipgloss.Width(title)+1+lipgloss.Width(right) > width {
		right = model.paint(marker+" "+state, stateColor)
	}
	padding := max(1, width-lipgloss.Width(title)-lipgloss.Width(right))
	return title + strings.Repeat(" ", padding) + right
}

func (model *Model) renderStatus(glyphs glyphSet, width int) string {
	waitingTickets, waitingPlayers := model.waitingCounts()
	if width < 60 {
		return fmt.Sprintf(
			"cycle %d%sq %dt/%dp%sa %d%sout %d",
			model.cycle,
			glyphs.separator,
			waitingTickets,
			waitingPlayers,
			glyphs.separator,
			len(model.active),
			glyphs.separator,
			len(model.history),
		)
	}
	left := fmt.Sprintf(
		"cycle %04d%sdemand %dt/%dp%sunbound %dt/%dp%sactive %d%sdeparted %d",
		model.cycle,
		glyphs.separator,
		model.queueTickets,
		model.queuePlayers,
		glyphs.separator,
		waitingTickets,
		waitingPlayers,
		glyphs.separator,
		len(model.active),
		glyphs.separator,
		len(model.history),
	)
	if model.lastCandidatesMax == 0 || width < 88 {
		if model.inFlight && model.working != "" {
			working := left + glyphs.separator + model.working
			if lipgloss.Width(working) <= width {
				left = working
			}
		}
		return left
	}
	candidateBar := progressBar(model.lastCandidateTickets, model.lastCandidatesMax, 10, glyphs)
	searchBar := progressBar(model.lastSearchNodes, model.lastSearchMax, 10, glyphs)
	right := fmt.Sprintf(
		"candidates %s %d/%d%ssearch %s %d/%d",
		candidateBar,
		model.lastCandidateTickets,
		model.lastCandidatesMax,
		glyphs.separator,
		searchBar,
		model.lastSearchNodes,
		model.lastSearchMax,
	)
	if lipgloss.Width(left)+1+lipgloss.Width(right) > width {
		return left + "\n" + right
	}
	padding := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", padding) + right
}

func (model *Model) waitingLines(glyphs glyphSet, limit int) []string {
	lines := make([]string, 0, limit)
	queued := 0
	for _, identifier := range model.ticketOrder {
		ticket := model.tickets[identifier]
		if ticket == nil || ticket.state != ticketQueued {
			continue
		}
		queued++
		if len(lines) >= limit {
			continue
		}
		lane := strings.Repeat(glyphs.dot, ticket.position) + model.partyGlyph(len(ticket.ticket.Players)) +
			strings.Repeat(glyphs.dot, max(0, 6-ticket.position)) + glyphs.arrow
		wait := max(time.Duration(0), model.now.Sub(ticket.ticket.EnqueuedAt)).Round(100 * time.Millisecond)
		lines = append(lines, fmt.Sprintf(
			"%-18s %-12s  wait %-5s  skill %-4d  %dms",
			lane,
			shortID(ticket.ticket.ID),
			wait,
			averageSkill(ticket.ticket),
			maximumLatency(ticket.ticket),
		))
	}
	if queued == 0 {
		return []string{model.paint("no unbound tickets; arrivals continue", "#7f8c98")}
	}
	if queued > len(lines) {
		lines[len(lines)-1] = fmt.Sprintf("%s +%d more waiting tickets", glyphs.ellipsis, queued-len(lines)+1)
	}
	return lines
}

func (model *Model) activeLines(glyphs glyphSet, limit int) []string {
	lines := make([]string, 0, limit)
	rendered := 0
	for _, identifier := range model.activeOrder {
		match := model.active[identifier]
		if match == nil || len(lines)+4 > limit {
			continue
		}
		rendered++
		icon, color := stageStyle(match.stage, glyphs)
		motion := motionTrack(match.motion, 8, glyphs)
		lines = append(lines, model.paint(fmt.Sprintf("%s %-8s %-10s %s", icon, matchLabel(identifier), strings.ToUpper(string(match.stage)), motion), color))
		for teamIndex, team := range match.proposal.Teams {
			lines = append(lines, fmt.Sprintf("  %c  %s", 'A'+rune(teamIndex), model.teamGlyph(team, match.partySizes)))
		}
		lines = append(lines, fmt.Sprintf(
			"  gap %d%smax latency %dms%ssearch %d nodes",
			match.proposal.Evidence.TeamSkillGap,
			glyphs.separator,
			match.proposal.Evidence.MaxLatencyMillis,
			glyphs.separator,
			match.proposal.Evidence.SearchNodes,
		))
	}
	if len(lines) == 0 {
		return []string{model.paint("waiting for the next proposal batch", "#7f8c98")}
	}
	if hidden := len(model.activeOrder) - rendered; hidden > 0 {
		lines[limit-1] = fmt.Sprintf("%s +%d more lifecycle matches", glyphs.ellipsis, hidden)
	}
	return lines
}

func (model *Model) historyLines(glyphs glyphSet, limit int) []string {
	if len(model.history) == 0 {
		return []string{model.paint("confirmed matches will depart through this lane", "#7f8c98")}
	}
	lines := make([]string, 0, min(limit, len(model.history)))
	for _, match := range model.history[:min(limit, len(model.history))] {
		teamSizes := make([]string, 0, len(match.proposal.Teams))
		for _, team := range match.proposal.Teams {
			players := 0
			for _, reference := range team.Tickets {
				players += match.partySizes[reference.ID]
			}
			teamSizes = append(teamSizes, fmt.Sprint(players))
		}
		lines = append(lines, model.paint(fmt.Sprintf(
			"%s %-8s  %s  gap %-4d latency %-3dms  %s",
			glyphs.departed,
			matchLabel(match.proposal.ID),
			strings.Join(teamSizes, glyphs.versus),
			match.proposal.Evidence.TeamSkillGap,
			match.proposal.Evidence.MaxLatencyMillis,
			motionTrack(8, 8, glyphs)+" departed",
		), "#57d38c"))
	}
	return lines
}

func (model *Model) eventLines(glyphs glyphSet, limit int) []string {
	if model.lastError != nil {
		return []string{model.paint("! "+model.lastError.Error(), "#ff6b6b")}
	}
	if len(model.logs) == 0 {
		return []string{model.paint("event stream is starting", "#7f8c98")}
	}
	start := max(0, len(model.logs)-limit)
	return append([]string(nil), model.logs[start:]...)
}

func (model *Model) panel(title string, lines []string, width int, glyphs glyphSet) string {
	contentWidth := max(10, width-4)
	title = model.paint(title, "#9ca3af")
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], contentWidth, glyphs.ellipsis)
	}
	content := title
	if len(lines) > 0 {
		content += "\n" + strings.Join(lines, "\n")
	}
	style := lipgloss.NewStyle().Border(glyphs.border, true).Padding(0, 1).Width(contentWidth)
	if model.options.Color {
		style = style.BorderForeground(lipgloss.Color("#44515c"))
	}
	return style.Render(content)
}

func (model *Model) partyGlyph(players int) string {
	glyphs := model.glyphs()
	if players <= 0 {
		return "[]"
	}
	return "[" + strings.Repeat(glyphs.player+glyphs.link, players-1) + glyphs.player + "]"
}

func (model *Model) teamGlyph(team api.TeamAssignment, partySizes map[string]int) string {
	parties := make([]string, 0, len(team.Tickets))
	for _, reference := range team.Tickets {
		parties = append(parties, model.partyGlyph(max(1, partySizes[reference.ID])))
	}
	return strings.Join(parties, " ")
}

func (model *Model) paint(value, color string) string {
	if !model.options.Color {
		return value
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(value)
}

func progressBar(value, maximum, width int, glyphs glyphSet) string {
	if maximum <= 0 || width <= 0 {
		return ""
	}
	filled := min(width, max(0, (value*width+maximum-1)/maximum))
	return "[" + strings.Repeat(glyphs.fill, filled) + strings.Repeat(glyphs.empty, width-filled) + "]"
}

func (model *Model) waitingCounts() (int, int) {
	tickets := 0
	players := 0
	for _, ticket := range model.tickets {
		if ticket.state == ticketQueued {
			tickets++
			players += len(ticket.ticket.Players)
		}
	}
	return tickets, players
}

func motionTrack(value, maximum int, glyphs glyphSet) string {
	value = min(maximum, max(0, value))
	return strings.Repeat(glyphs.track, value) + glyphs.arrow + strings.Repeat(glyphs.dot, maximum-value)
}

func stageStyle(stage matchStage, glyphs glyphSet) (string, string) {
	switch stage {
	case stageReserved:
		return glyphs.reserved, "#c084fc"
	case stageConfirmed:
		return glyphs.confirmed, "#57d38c"
	default:
		return glyphs.proposed, "#67e8f9"
	}
}

func averageSkill(ticket api.MatchTicket) int {
	total := 0
	for _, player := range ticket.Players {
		total += player.Skill
	}
	if len(ticket.Players) == 0 {
		return 0
	}
	return total / len(ticket.Players)
}

func maximumLatency(ticket api.MatchTicket) int {
	result := 0
	for _, player := range ticket.Players {
		result = max(result, player.LatencyMillis)
	}
	return result
}
