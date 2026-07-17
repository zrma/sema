package flowui_test

import (
	"context"
	"strings"
	"testing"

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
	if err := model.RunSteps(context.Background(), 32); err != nil {
		t.Fatal(err)
	}
	content := model.Content()
	for _, expected := range []string{"SEMA FLOW", "[●─●]", "◉ C2/P1", "✓ C1/P", "departed"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("snapshot omitted %q:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "more lifecycle matches") {
		t.Fatalf("snapshot reported hidden matches when all active matches fit:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if width := lipgloss.Width(line); width > 120 {
			t.Fatalf("rendered line width = %d; want <= 120:\n%s", width, line)
		}
	}
}

func TestSnapshotSupportsASCIIFallback(t *testing.T) {
	simulator := openSimulator(t)
	options := flowui.DefaultOptions()
	options.Unicode = false
	options.Color = false
	model := flowui.New(simulator, options)
	if err := model.RunSteps(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	content := model.Content()
	if !strings.Contains(content, "[o-o]") || strings.Contains(content, "●") || strings.Contains(content, "╭") {
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
	if err := model.RunSteps(context.Background(), 34); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(model.Content(), "\n")
	if len(lines) > 24 {
		t.Fatalf("compact snapshot lines = %d; want <= 24:\n%s", len(lines), model.Content())
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("compact line width = %d; want <= 80:\n%s", width, line)
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
	simulator, err := flow.Open(flow.DefaultConfig())
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
