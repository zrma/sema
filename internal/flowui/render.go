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
	completed string
	ellipsis  string
	separator string
	spinners  []string
	border    lipgloss.Border
}

func (model *Model) glyphs() glyphSet {
	if !model.options.Unicode {
		return glyphSet{
			player: "o", link: "-", dot: ".", arrow: ">", track: "=", fill: "#", empty: "-",
			proposed: "o", reserved: "*", confirmed: "+", completed: "OK", ellipsis: "...",
			separator: " | ", spinners: []string{"|", "/", "-", "\\"},
			border: lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"},
		}
	}
	return glyphSet{
		player: "●", link: "─", dot: "·", arrow: "▷", track: "━", fill: "█", empty: "░",
		proposed: "◉", reserved: "◆", confirmed: "✓", completed: "✓", ellipsis: "…",
		separator: " · ", spinners: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
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
		return model.renderCompact(glyphs, width, height, header, status)
	}
	footer := model.paint(
		"[space] pause  [n] step  [+/-] speed  [u] unicode  [m] motion  [q] quit",
		"#7f8c98",
	)
	panelHeight := height - lineCount(header) - lineCount(status) - lineCount(footer)
	mainHeight, historyHeight, eventHeight := fullPanelHeights(panelHeight)

	var main string
	if width >= 96 {
		leftWidth := (width - 1) / 2
		rightWidth := width - leftWidth - 1
		main = lipgloss.JoinHorizontal(
			lipgloss.Top,
			model.panel("WAITING POOL", model.waitingLines(glyphs, mainHeight-3), leftWidth, mainHeight, glyphs),
			" ",
			model.panel("MATCH LIFECYCLE", model.activeLines(glyphs, mainHeight-3), rightWidth, mainHeight, glyphs),
		)
	} else {
		waitingHeight := max(4, mainHeight/3)
		lifecycleHeight := mainHeight - waitingHeight
		main = lipgloss.JoinVertical(
			lipgloss.Left,
			model.panel("WAITING POOL", model.waitingLines(glyphs, waitingHeight-3), width, waitingHeight, glyphs),
			model.panel("MATCH LIFECYCLE", model.activeLines(glyphs, lifecycleHeight-3), width, lifecycleHeight, glyphs),
		)
	}

	history := model.panel(
		"COMPLETED MATCHES",
		model.historyLines(glyphs, historyHeight-3),
		width,
		historyHeight,
		glyphs,
	)
	events := model.panel("EVENT STREAM", model.eventLines(glyphs, eventHeight-3), width, eventHeight, glyphs)
	return strings.Join([]string{header, status, main, history, events, footer}, "\n")
}

func (model *Model) renderCompact(glyphs glyphSet, width, height int, header, status string) string {
	footer := "[space] pause  [n] step  [+/-] speed  [u] unicode  [m] motion  [q] quit"
	if width < 72 {
		footer = "space pause | n step | +/- speed | q quit"
	}
	available := height - lineCount(header) - lineCount(status) - 1
	flowHeight, recentHeight := compactPanelHeights(available)
	flowSlots := max(2, flowHeight-5)
	waitingSlots := max(1, flowSlots/3)
	lifecycleSlots := max(1, flowSlots-waitingSlots)
	flowLines := []string{model.paint("waiting", "#9ca3af")}
	flowLines = append(flowLines, model.waitingLines(glyphs, waitingSlots)...)
	flowLines = append(flowLines, model.paint("lifecycle", "#9ca3af"))
	flowLines = append(flowLines, model.activeLines(glyphs, lifecycleSlots)...)
	recentSlots := max(2, recentHeight-3)
	historySlots := max(1, recentSlots/2)
	eventSlots := max(1, recentSlots-historySlots)
	recentLines := model.historyLines(glyphs, historySlots)
	recentLines = append(recentLines, model.eventLines(glyphs, eventSlots)...)
	return strings.Join([]string{
		header,
		status,
		model.panel("FLOW", flowLines, width, flowHeight, glyphs),
		model.panel("RECENT", recentLines, width, recentHeight, glyphs),
		model.paint(footer, "#7f8c98"),
	}, "\n")
}

func fullPanelHeights(available int) (int, int, int) {
	const (
		minimumMain    = 8
		minimumHistory = 5
		minimumEvents  = 5
	)
	extra := max(0, available-minimumMain-minimumHistory-minimumEvents)
	mainExtra := (extra + 1) / 2
	remaining := extra - mainExtra
	historyExtra := remaining / 2
	eventExtra := remaining - historyExtra
	return minimumMain + mainExtra, minimumHistory + historyExtra, minimumEvents + eventExtra
}

func compactPanelHeights(available int) (int, int) {
	const (
		minimumFlow   = 8
		minimumRecent = 5
	)
	extra := max(0, available-minimumFlow-minimumRecent)
	flowExtra := (extra * 2) / 3
	return minimumFlow + flowExtra, minimumRecent + extra - flowExtra
}

func lineCount(value string) int {
	return strings.Count(value, "\n") + 1
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
	speed := model.speedLabel()
	right := fmt.Sprintf(
		"%s %s  speed %s  seed %d  step %s",
		marker,
		state,
		speed,
		model.options.Seed,
		model.options.StepInterval.Round(time.Millisecond),
	)
	right = model.paint(right, stateColor)
	if lipgloss.Width(title)+1+lipgloss.Width(right) > width {
		right = model.paint(fmt.Sprintf("%s %s %s", marker, state, speed), stateColor)
	}
	padding := max(1, width-lipgloss.Width(title)-lipgloss.Width(right))
	return title + strings.Repeat(" ", padding) + right
}

func (model *Model) speedLabel() string {
	step := model.simulatedStep
	if step <= 0 {
		step = time.Second
	}
	multiplier := float64(step) / float64(model.options.StepInterval)
	switch {
	case multiplier >= 10:
		return fmt.Sprintf("%.0f×", multiplier)
	case multiplier >= 1:
		return fmt.Sprintf("%.1f×", multiplier)
	default:
		return fmt.Sprintf("%.2f×", multiplier)
	}
}

func (model *Model) renderStatus(glyphs glyphSet, width int) string {
	if width < 96 {
		population := fmt.Sprintf(
			"sim %s%sc %d%spop %d%sidle %d%sq %dt/%dp",
			model.now.Format("15:04:05"),
			glyphs.separator,
			model.cycle,
			glyphs.separator,
			model.population.Players,
			glyphs.separator,
			model.idlePlayers,
			glyphs.separator,
			model.queueTickets,
			model.queuePlayers,
		)
		activity := fmt.Sprintf(
			"games %d/%dp%sready %dt/%dp%scooldown %d%splayed %d",
			model.activeMatches,
			model.inGamePlayers,
			glyphs.separator,
			model.ingressTickets,
			model.ingressPlayers,
			glyphs.separator,
			model.cooldownPlayers,
			glyphs.separator,
			model.population.GamesPlayed,
		)
		return population + "\n" + activity + "\n" + model.ratingLine(glyphs, width)
	}
	left := fmt.Sprintf(
		"sim %s%scycle %04d%spopulation %d%sidle %d%squeued %dt/%dp%sready %dt/%dp%sgames %dm/%dp%scooldown %d%splayed %d",
		model.now.Format("15:04:05"),
		glyphs.separator,
		model.cycle,
		glyphs.separator,
		model.population.Players,
		glyphs.separator,
		model.idlePlayers,
		glyphs.separator,
		model.queueTickets,
		model.queuePlayers,
		glyphs.separator,
		model.ingressTickets,
		model.ingressPlayers,
		glyphs.separator,
		model.activeMatches,
		model.inGamePlayers,
		glyphs.separator,
		model.cooldownPlayers,
		glyphs.separator,
		model.population.GamesPlayed,
	)
	if model.inFlight && model.working != "" {
		working := left + glyphs.separator + model.working
		if lipgloss.Width(working) <= width {
			left = working
		}
	}
	lines := []string{left, model.ratingLine(glyphs, width)}
	if model.lastCandidatesMax == 0 || width < 88 {
		return strings.Join(lines, "\n")
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
	lines = append(lines, right)
	return strings.Join(lines, "\n")
}

func (model *Model) ratingLine(glyphs glyphSet, width int) string {
	stats := model.population
	if stats.Players == 0 {
		return "rating distribution unavailable"
	}
	line := fmt.Sprintf(
		"rating %d %s %d%sp10 %d%smedian %d%sp90 %d%smean %d%ssd %d",
		stats.Minimum,
		ratingHistogram(stats.Histogram, model.options.Unicode),
		stats.Maximum,
		glyphs.separator,
		stats.Percentile10,
		glyphs.separator,
		stats.Median,
		glyphs.separator,
		stats.Percentile90,
		glyphs.separator,
		stats.Mean,
		glyphs.separator,
		stats.StdDev,
	)
	return ansi.Truncate(line, width, glyphs.ellipsis)
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
			"%-16s %-11s %5s  r%-4d  %dms",
			lane,
			shortID(ticket.ticket.ID),
			wait,
			averageSkill(ticket.ticket),
			maximumLatency(ticket.ticket),
		))
	}
	if queued == 0 {
		return []string{model.paint("waiting for scheduled arrivals or returning parties", "#7f8c98")}
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
		stage := strings.ToUpper(string(match.stage))
		if match.stage == stagePlaying {
			remaining := max(time.Duration(0), match.endsAt.Sub(model.now)).Round(time.Second)
			stage += " " + remaining.String()
		}
		lines = append(lines, model.paint(fmt.Sprintf("%s %-8s %-15s %s", icon, matchLabel(identifier), stage, motion), color))
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
		if len(model.active) > 0 {
			return []string{fmt.Sprintf("%s %d active lifecycle matches", glyphs.ellipsis, len(model.active))}
		}
		return []string{model.paint("waiting for the next proposal batch", "#7f8c98")}
	}
	if hidden := len(model.activeOrder) - rendered; hidden > 0 {
		lines[len(lines)-1] = fmt.Sprintf("%s +%d more lifecycle matches", glyphs.ellipsis, hidden)
	}
	return lines
}

func (model *Model) historyLines(glyphs glyphSet, limit int) []string {
	if len(model.history) == 0 {
		return []string{model.paint("completed game results will appear here", "#7f8c98")}
	}
	lines := make([]string, 0, min(limit, len(model.history)))
	for _, match := range model.history[:min(limit, len(model.history))] {
		winner := match.result.WinnerTeam + 1
		lines = append(lines, model.paint(fmt.Sprintf(
			"%s %-8s  team %d won  p %.0f%%  rating %d/%d  delta %+d/%+d",
			glyphs.completed,
			matchLabel(match.proposal.ID),
			winner,
			match.result.WinnerProbability*100,
			match.result.TeamRatingBefore[0],
			match.result.TeamRatingBefore[1],
			match.result.RatingDelta[0],
			match.result.RatingDelta[1],
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

func (model *Model) panel(title string, lines []string, width, height int, glyphs glyphSet) string {
	contentWidth := max(10, width-4)
	boxWidth := max(12, width-2)
	bodyHeight := max(0, height-3)
	fitted := make([]string, 0, bodyHeight)
	for _, line := range lines[:min(len(lines), bodyHeight)] {
		fitted = append(fitted, ansi.Truncate(line, contentWidth, glyphs.ellipsis))
	}
	for len(fitted) < bodyHeight {
		fitted = append(fitted, " ")
	}
	title = model.paint(title, "#9ca3af")
	content := title
	if len(fitted) > 0 {
		content += "\n" + strings.Join(fitted, "\n")
	}
	style := lipgloss.NewStyle().Border(glyphs.border, true).Padding(0, 1).Width(boxWidth)
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

func motionTrack(value, maximum int, glyphs glyphSet) string {
	value = min(maximum, max(0, value))
	return strings.Repeat(glyphs.track, value) + glyphs.arrow + strings.Repeat(glyphs.dot, maximum-value)
}

func stageStyle(stage matchStage, glyphs glyphSet) (string, string) {
	switch stage {
	case stageReserved:
		return glyphs.reserved, "#c084fc"
	case stagePlaying:
		return glyphs.confirmed, "#f0c36e"
	default:
		return glyphs.proposed, "#67e8f9"
	}
}

func ratingHistogram(histogram [9]int, unicode bool) string {
	maximum := 0
	for _, value := range histogram {
		maximum = max(maximum, value)
	}
	if maximum == 0 {
		return "........."
	}
	levels := []rune(".:-=+*#%@")
	if unicode {
		levels = []rune("·▁▂▃▄▅▆▇█")
	}
	result := make([]rune, len(histogram))
	for index, value := range histogram {
		if value == 0 {
			result[index] = levels[0]
			continue
		}
		level := max(1, (value*(len(levels)-1)+maximum-1)/maximum)
		result[index] = levels[min(level, len(levels)-1)]
	}
	return string(result)
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
