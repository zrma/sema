package flowui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zrma/sema/internal/flow"
	"github.com/zrma/sema/internal/flowui"
)

func TestSnapshotRendersUnicodeLifecycleWithinWidth(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Color = false
	options.ReducedMotion = true
	options.Width = 120
	model := flowui.New(simulator, options)
	if err := model.RunSteps(context.Background(), 60); err != nil {
		t.Fatal(err)
	}
	content := model.Content()
	lines := strings.Split(content, "\n")
	for _, expected := range []string{"SEMA FLOW", "speed 4.5×", "[●─●]", "rating", "team", "won"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("snapshot omitted %q:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "more lifecycle matches") {
		t.Fatalf("snapshot reported hidden matches when all active matches fit:\n%s", content)
	}
	if len(lines) != options.Height {
		t.Fatalf("rendered lines = %d; want %d:\n%s", len(lines), options.Height, content)
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 120 {
			t.Fatalf("rendered line width = %d; want <= 120:\n%s", width, line)
		}
	}
}

func TestSpeedIndicatorTracksControls(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Color = false
	model := flowui.New(simulator, options)
	if !strings.Contains(model.Content(), "speed 4.5×") {
		t.Fatalf("default speed indicator is missing:\n%s", model.Content())
	}

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "+", Code: '+'}))
	model = updated.(*flowui.Model)
	if !strings.Contains(model.Content(), "speed 5.7×") {
		t.Fatalf("accelerated speed indicator is missing:\n%s", model.Content())
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "-", Code: '-'}))
	model = updated.(*flowui.Model)
	if !strings.Contains(model.Content(), "speed 4.5×") {
		t.Fatalf("restored speed indicator is missing:\n%s", model.Content())
	}
}

func TestSnapshotSupportsASCIIFallback(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Unicode = false
	options.Color = false
	model := flowui.New(simulator, options)
	if err := model.RunSteps(context.Background(), 60); err != nil {
		t.Fatal(err)
	}
	content := model.Content()
	if !strings.Contains(content, "[o-o]") || strings.Contains(content, "●") || strings.Contains(content, "╭") || strings.Contains(content, "▁") {
		t.Fatalf("ASCII fallback contains unexpected glyphs:\n%s", content)
	}
}

func TestCompactSnapshotFitsStandardTerminal(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Color = false
	options.ReducedMotion = true
	options.Width = 80
	options.Height = 24
	model := flowui.New(simulator, options)
	if err := model.RunSteps(context.Background(), 24); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(model.Content(), "\n")
	if !strings.Contains(model.Content(), "PLAYING") {
		t.Fatalf("compact snapshot omitted an active game:\n%s", model.Content())
	}
	if len(lines) != 24 {
		t.Fatalf("compact snapshot lines = %d; want 24:\n%s", len(lines), model.Content())
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("compact line width = %d; want <= 80:\n%s", width, line)
		}
	}
}

func TestTallSnapshotExpandsAllLifecycleRegions(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Color = false
	options.ReducedMotion = true
	options.Width = 140
	options.Height = 56
	model := flowui.New(simulator, options)
	if err := model.RunSteps(context.Background(), 240); err != nil {
		t.Fatal(err)
	}
	content := model.Content()
	lines := strings.Split(content, "\n")
	if len(lines) != options.Height {
		t.Fatalf("tall snapshot lines = %d; want %d:\n%s", len(lines), options.Height, content)
	}
	if completed := strings.Count(content, " won  p "); completed < 6 {
		t.Fatalf("tall snapshot completed rows = %d; want at least 6:\n%s", completed, content)
	}
	if events := strings.Count(content, "│ 00:"); events < 8 {
		t.Fatalf("tall snapshot event rows = %d; want at least 8:\n%s", events, content)
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > options.Width {
			t.Fatalf("tall line width = %d; want <= %d:\n%s", width, options.Width, line)
		}
	}
}

func TestRunStepsRejectsEmptySnapshot(t *testing.T) {
	simulator := openSimulator(t)
	model := flowui.New(simulator, flowui.DefaultOptions())
	if err := model.RunSteps(context.Background(), 0); err == nil {
		t.Fatal("zero snapshot steps were accepted")
	}
}

func openSimulator(t *testing.T) *flow.Simulator {
	t.Helper()
	configuration := flow.DefaultConfig()
	configuration.PopulationSize = 40
	configuration.MaxConcurrentMatches = 2
	configuration.GameDuration = 20 * time.Second
	configuration.ArrivalInterval = time.Second
	configuration.PlanningInterval = 2 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	configuration.TickDuration = time.Second
	simulator, err := flow.Open(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})
	return simulator
}
