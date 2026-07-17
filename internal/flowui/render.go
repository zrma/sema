package flowui

import (
	"fmt"
	"math"
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

var unicodeMatchMarkers = [...]string{
	"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩",
	"⑪", "⑫", "⑬", "⑭", "⑮", "⑯", "⑰", "⑱", "⑲", "⑳",
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
	if height < 30 || width < 72 {
		return model.renderCompact(glyphs, width, height, header, status)
	}
	footer := model.paint(
		"[space] pause  [n] step  [+/-] speed  [u] unicode  [m] motion  [q] quit",
		"#7f8c98",
	)
	panelHeight := height - lineCount(header) - lineCount(status) - lineCount(footer)
	mainHeight, analyticsHeight, recentHeight := fullPanelHeights(panelHeight)

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

	leftWidth := (width - 1) / 2
	rightWidth := width - leftWidth - 1
	analytics := lipgloss.JoinHorizontal(
		lipgloss.Top,
		model.panel(
			model.waitTrendTitle(glyphs, leftWidth-12),
			model.waitTrendLines(glyphs, leftWidth-4, analyticsHeight-3),
			leftWidth,
			analyticsHeight,
			glyphs,
		),
		" ",
		model.panel(
			model.trendTitle("RATING DENSITY", glyphs, rightWidth-12),
			model.ratingDensityLines(glyphs, rightWidth-4, analyticsHeight-3),
			rightWidth,
			analyticsHeight,
			glyphs,
		),
	)
	recent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		model.panel(
			"COMPLETED MATCHES",
			fitLines(model.historyLines(glyphs, recentHeight-3), leftWidth-12, glyphs.ellipsis),
			leftWidth,
			recentHeight,
			glyphs,
		),
		" ",
		model.panel(
			"EVENT STREAM",
			fitLines(model.eventLines(glyphs, recentHeight-3), rightWidth-12, glyphs.ellipsis),
			rightWidth,
			recentHeight,
			glyphs,
		),
	)
	return strings.Join([]string{header, status, main, analytics, recent, footer}, "\n")
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
		minimumMain      = 8
		minimumAnalytics = 8
		minimumRecent    = 5
	)
	extra := max(0, available-minimumMain-minimumAnalytics-minimumRecent)
	mainExtra := extra * 2 / 5
	analyticsExtra := extra * 2 / 5
	recentExtra := extra - mainExtra - analyticsExtra
	return minimumMain + mainExtra, minimumAnalytics + analyticsExtra, minimumRecent + recentExtra
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

func fitLines(lines []string, width int, tail string) []string {
	fitted := make([]string, 0, len(lines))
	for _, line := range lines {
		fitted = append(fitted, ansi.Truncate(line, max(1, width), tail))
	}
	return fitted
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
	limit := "<="
	if model.options.Unicode {
		limit = "≤"
	}
	right := fmt.Sprintf(
		"batch %d/%s%d every %s%scandidates %s %d/%d%ssearch %s %d/%d",
		model.lastProposals,
		limit,
		model.lastProposalsMax,
		formatAxisDuration(model.lastPlanningInterval),
		glyphs.separator,
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
	if limit <= 0 {
		return nil
	}
	displayed := 0
	maximumRow := -1
	for _, identifier := range model.ticketOrder {
		ticket := model.tickets[identifier]
		if ticket == nil || !ticket.displayedInQueue() {
			continue
		}
		displayed++
		maximumRow = max(maximumRow, ticket.queueRow)
	}
	if displayed == 0 {
		return []string{model.paint("waiting for scheduled arrivals or returning parties", "#7f8c98")}
	}

	needsSummary := displayed > limit || maximumRow >= limit
	ticketSlots := limit
	if needsSummary {
		ticketSlots = max(0, limit-1)
	}
	lines := make([]string, limit)
	for index := range lines {
		lines[index] = " "
	}
	rendered := 0
	for _, identifier := range model.ticketOrder {
		ticket := model.tickets[identifier]
		if ticket == nil || !ticket.displayedInQueue() || ticket.queueRow < 0 || ticket.queueRow >= ticketSlots {
			continue
		}
		lane := model.waitingLane(ticket, glyphs)
		wait := max(time.Duration(0), model.now.Sub(ticket.ticket.EnqueuedAt)).Round(100 * time.Millisecond)
		marker := " "
		if ticket.leaving {
			marker = matchVisualMarker(ticket.matchVisual, model.options.Unicode)
		}
		line := fmt.Sprintf(
			"%s %s %-11s %5s  r%-4d  %dms",
			marker,
			lane,
			shortID(ticket.ticket.ID),
			wait,
			averageSkill(ticket.ticket),
			maximumLatency(ticket.ticket),
		)
		if ticket.leaving {
			line = model.paint(line, matchVisualColor(ticket.matchVisual))
		}
		lines[ticket.queueRow] = line
		rendered++
	}
	if hidden := displayed - rendered; hidden > 0 {
		lines[len(lines)-1] = fmt.Sprintf("%s +%d more waiting tickets", glyphs.ellipsis, hidden)
	}
	return lines
}

func (model *Model) waitingLane(ticket *ticketView, glyphs glyphSet) string {
	const width = 16
	var lane string
	if ticket.leaving {
		travel := max(0, ticket.selectionFrame-selectionHoldFrames)
		lane = strings.Repeat(glyphs.dot, 6+travel) + model.partyGlyph(len(ticket.ticket.Players)) + glyphs.arrow
	} else {
		lane = strings.Repeat(glyphs.dot, ticket.position) + model.partyGlyph(len(ticket.ticket.Players)) +
			strings.Repeat(glyphs.dot, max(0, 6-ticket.position)) + glyphs.arrow
	}
	lane = ansi.Truncate(lane, width, "")
	return lane + strings.Repeat(" ", max(0, width-lipgloss.Width(lane)))
}

func (model *Model) activeLines(glyphs glyphSet, limit int) []string {
	lines := make([]string, 0, limit)
	hidden := 0
	animating := false
	for _, identifier := range model.activeDisplayOrder() {
		match := model.active[identifier]
		if match == nil {
			continue
		}
		block := model.lifecycleBlock(identifier, match, glyphs)
		visibleRows := lifecycleEntryRows(match.entryFrame, len(block))
		if visibleRows == 0 {
			animating = true
			continue
		}
		if visibleRows < len(block) {
			animating = true
		}
		if len(lines)+visibleRows > limit {
			hidden++
			continue
		}
		lines = append(lines, block[:visibleRows]...)
	}
	if len(lines) == 0 {
		if len(model.active) > 0 {
			return []string{model.paint("receiving selected matches", "#7f8c98")}
		}
		return []string{model.paint("waiting for the next proposal batch", "#7f8c98")}
	}
	if hidden > 0 && !animating {
		lines[len(lines)-1] = fmt.Sprintf("%s +%d more lifecycle matches", glyphs.ellipsis, hidden)
	}
	return lines
}

func (model *Model) lifecycleBlock(identifier string, match *matchView, glyphs glyphSet) []string {
	icon, _ := stageStyle(match.stage, glyphs)
	motion := motionTrack(match.motion, 8, glyphs)
	stage := strings.ToUpper(string(match.stage))
	if match.stage == stagePlaying {
		remaining := max(time.Duration(0), match.endsAt.Sub(model.now)).Round(time.Second)
		stage += " " + remaining.String()
	}
	color := matchVisualColor(match.matchVisual)
	marker := matchVisualMarker(match.matchVisual, model.options.Unicode)
	lines := []string{model.paint(fmt.Sprintf("%s %s %-8s %-15s %s", marker, icon, matchLabel(identifier), stage, motion), color)}
	for teamIndex, team := range match.proposal.Teams {
		lines = append(lines, model.paint(fmt.Sprintf("  %c  %s", 'A'+rune(teamIndex), model.teamGlyph(team, match.partySizes)), color))
	}
	return append(lines, model.paint(fmt.Sprintf(
		"  gap %d%smax latency %dms%ssearch %d nodes",
		match.proposal.Evidence.TeamSkillGap,
		glyphs.separator,
		match.proposal.Evidence.MaxLatencyMillis,
		glyphs.separator,
		match.proposal.Evidence.SearchNodes,
	), color))
}

func lifecycleEntryRows(frame, blockHeight int) int {
	if blockHeight <= 0 || frame <= 0 {
		return 0
	}
	if frame >= lifecycleEntryFrames {
		return blockHeight
	}
	return min(blockHeight, (frame*blockHeight+lifecycleEntryFrames-1)/lifecycleEntryFrames)
}

func (model *Model) activeDisplayOrder() []string {
	order := make([]string, 0, len(model.activeOrder))
	for _, identifier := range model.activeOrder {
		if model.active[identifier] != nil {
			order = append(order, identifier)
		}
	}
	return order
}

func matchVisualMarker(index int, unicode bool) string {
	index = max(0, index)
	if unicode && index < len(unicodeMatchMarkers) {
		return unicodeMatchMarkers[index]
	}
	return fmt.Sprintf("%02d", index+1)
}

func matchVisualColor(index int) string {
	const goldenAngle = 137.507764
	hue := math.Mod(190+float64(max(0, index))*goldenAngle, 360) / 360
	red, green, blue := hslToRGB(hue, 0.68, 0.58)
	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}

func hslToRGB(hue, saturation, lightness float64) (uint8, uint8, uint8) {
	if saturation == 0 {
		value := uint8(math.Round(lightness * 255))
		return value, value, value
	}
	maximum := lightness * (1 + saturation)
	if lightness >= 0.5 {
		maximum = lightness + saturation - lightness*saturation
	}
	minimum := 2*lightness - maximum
	convert := func(offset float64) uint8 {
		offset = math.Mod(offset+1, 1)
		var value float64
		switch {
		case offset < 1.0/6:
			value = minimum + (maximum-minimum)*6*offset
		case offset < 0.5:
			value = maximum
		case offset < 2.0/3:
			value = minimum + (maximum-minimum)*(2.0/3-offset)*6
		default:
			value = minimum
		}
		return uint8(math.Round(value * 255))
	}
	return convert(hue + 1.0/3), convert(hue), convert(hue - 1.0/3)
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

type trendColumn struct {
	sample trendSample
	valid  bool
}

type ratingBand struct {
	lower int
	upper int
	label string
}

func (model *Model) trendTitle(title string, glyphs glyphSet, width int) string {
	if len(model.trends) == 0 {
		return ansi.Truncate(title, width, glyphs.ellipsis)
	}
	arrow := "→"
	if !model.options.Unicode {
		arrow = ">"
	}
	first := model.trends[0].at.Format("15:04")
	last := model.trends[len(model.trends)-1].at.Format("15:04")
	return ansi.Truncate(fmt.Sprintf("%s  %s%s%s", title, first, arrow, last), width, glyphs.ellipsis)
}

func (model *Model) waitTrendTitle(glyphs glyphSet, width int) string {
	title := "AVERAGE QUEUE WAIT"
	if len(model.trends) > 0 {
		title += " " + formatAxisDuration(model.trends[len(model.trends)-1].averageWait)
	}
	return model.trendTitle(title, glyphs, width)
}

func (model *Model) waitTrendLines(glyphs glyphSet, width, height int) []string {
	if height <= 0 {
		return nil
	}
	const axisWidth = 7
	// Leave a small right gutter. Lipgloss preserves long leading-space runs in
	// sparse chart rows, which can otherwise wrap the last populated cells.
	chartWidth := max(1, width-axisWidth-12)
	columns := trendColumns(model.trends, chartWidth)
	maximumWait := time.Duration(0)
	for _, column := range columns {
		if column.valid {
			maximumWait = max(maximumWait, column.sample.averageWait)
		}
	}
	maximumWait = niceWaitCeiling(maximumWait)
	lines := make([]string, 0, height)
	for row := range height {
		axisValue := maximumWait
		if height > 1 {
			axisValue = maximumWait * time.Duration(height-1-row) / time.Duration(height-1)
		}
		label := ""
		if row == 0 || row == height/2 || row == height-1 {
			label = formatAxisDuration(axisValue)
		}
		axis := "│"
		if !model.options.Unicode {
			axis = "|"
		}
		var chart strings.Builder
		for _, column := range columns {
			if !column.valid {
				chart.WriteByte(' ')
				continue
			}
			pointRow := height - 1
			if height > 1 && maximumWait > 0 {
				pointRow -= int(column.sample.averageWait * time.Duration(height-1) / maximumWait)
			}
			switch {
			case row == pointRow:
				point := "*"
				if model.options.Unicode {
					point = "●"
				}
				chart.WriteString(model.paint(point, "#d97706"))
			case row > pointRow:
				fill := ":"
				if model.options.Unicode {
					fill = "░"
				}
				chart.WriteString(model.paint(fill, "#f0c36e"))
			default:
				chart.WriteByte(' ')
			}
		}
		lines = append(lines, fmt.Sprintf("%6s%s%s", label, axis, chart.String()))
	}
	return lines
}

func (model *Model) ratingDensityLines(glyphs glyphSet, width, height int) []string {
	if height <= 0 {
		return nil
	}
	bandCount := min(9, height)
	bands := ratingBands(bandCount)
	const axisWidth = 6
	chartWidth := max(1, width-axisWidth-12)
	columns := trendColumns(model.trends, chartWidth)
	axis := "│"
	if !model.options.Unicode {
		axis = "|"
	}
	lines := make([]string, 0, height)
	for row := range height {
		bandIndex := min(bandCount-1, row*bandCount/height)
		band := bands[bandIndex]
		labelRow := ratingBandLabelRow(bandIndex, bandCount, height)
		label := ""
		if row == labelRow {
			label = band.label
		}
		var chart strings.Builder
		for _, column := range columns {
			if !column.valid {
				chart.WriteByte(' ')
				continue
			}
			players := 0
			for bucket := band.lower; bucket <= band.upper; bucket++ {
				players += column.sample.ratingHistogram[bucket]
			}
			centered := band.lower <= 4 && band.upper >= 4 && row == labelRow
			chart.WriteString(model.densityCell(players, column.sample.population, centered))
		}
		lines = append(lines, fmt.Sprintf("%5s%s%s", label, axis, chart.String()))
	}
	return lines
}

func ratingBandLabelRow(index, count, height int) int {
	start := (index*height + count - 1) / count
	end := ((index+1)*height + count - 1) / count
	return (start + end - 1) / 2
}

func trendColumns(samples []trendSample, width int) []trendColumn {
	columns := make([]trendColumn, max(1, width))
	if len(samples) == 0 {
		return columns
	}
	if len(samples) == 1 || width == 1 {
		columns[len(columns)-1] = trendColumn{sample: samples[len(samples)-1], valid: true}
		return columns
	}
	first := samples[0].at
	span := samples[len(samples)-1].at.Sub(first)
	if span <= 0 {
		columns[len(columns)-1] = trendColumn{sample: samples[len(samples)-1], valid: true}
		return columns
	}
	for _, sample := range samples {
		position := int(float64(sample.at.Sub(first)) / float64(span) * float64(len(columns)-1))
		position = min(len(columns)-1, max(0, position))
		columns[position] = trendColumn{sample: sample, valid: true}
	}
	var previous trendColumn
	for index := range columns {
		if columns[index].valid {
			previous = columns[index]
			continue
		}
		if previous.valid {
			columns[index] = previous
		}
	}
	return columns
}

func ratingBands(count int) []ratingBand {
	labels := [...]string{"<1400", "1400", "1450", "1475", "1500", "1501", "1526", "1551", ">1600"}
	bands := make([]ratingBand, 0, count)
	for row := range count {
		group := count - 1 - row
		lower := group * len(labels) / count
		upper := (group+1)*len(labels)/count - 1
		labelIndex := lower
		if lower <= 4 && upper >= 4 {
			labelIndex = 4
		} else if upper < 4 {
			labelIndex = upper
		}
		bands = append(bands, ratingBand{lower: lower, upper: upper, label: labels[labelIndex]})
	}
	return bands
}

func (model *Model) densityCell(players, population int, centered bool) string {
	if centered && (players <= 0 || population <= 0) {
		axis := "─"
		if !model.options.Unicode {
			axis = "-"
		}
		return model.paint(axis, "#e2e8f0")
	}
	if players <= 0 || population <= 0 {
		return " "
	}
	basisPoints := players * 10_000 / population
	level := 4
	switch {
	case basisPoints <= 100:
		level = 0
	case basisPoints <= 500:
		level = 1
	case basisPoints <= 1_500:
		level = 2
	case basisPoints <= 3_500:
		level = 3
	}
	colors := [...]string{"#cbd5e1", "#93c5d8", "#38bdf8", "#0284c7", "#075985"}
	glyphs := [...]string{"·", "░", "▒", "▓", "█"}
	if !model.options.Unicode {
		glyphs = [...]string{".", ":", "*", "#", "%"}
	}
	return model.paint(glyphs[level], colors[level])
}

func niceWaitCeiling(value time.Duration) time.Duration {
	step := time.Second
	switch {
	case value >= 5*time.Minute:
		step = time.Minute
	case value >= time.Minute:
		step = 30 * time.Second
	case value >= 10*time.Second:
		step = 5 * time.Second
	}
	value = max(5*time.Second, value)
	return (value + step - 1) / step * step
}

func formatAxisDuration(value time.Duration) string {
	value = max(time.Duration(0), value.Round(time.Second))
	if value >= time.Minute {
		minutes := int(value / time.Minute)
		seconds := int(value%time.Minute) / int(time.Second)
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", int(value/time.Second))
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
