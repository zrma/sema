package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zrma/sema/internal/flow"
	"github.com/zrma/sema/internal/flowui"
)

var version = "dev"

type config struct {
	seed             int64
	interval         time.Duration
	population       int
	matchesPerCycle  int
	gameDuration     time.Duration
	arrivalInterval  time.Duration
	planningInterval time.Duration
	maxReturnDelay   time.Duration
	snapshot         bool
	steps            int
	width            int
	height           int
	ascii            bool
	noColor          bool
	reducedMotion    bool
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	configuration, showVersion, err := parseConfig(args, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-tui %s\n", version)
		return 0
	}

	flowConfiguration := flow.DefaultConfig()
	flowConfiguration.Seed = configuration.seed
	flowConfiguration.PopulationSize = configuration.population
	flowConfiguration.MatchesPerCycle = configuration.matchesPerCycle
	flowConfiguration.GameDuration = configuration.gameDuration
	flowConfiguration.ArrivalInterval = configuration.arrivalInterval
	flowConfiguration.PlanningInterval = configuration.planningInterval
	flowConfiguration.MaxReturnDelay = configuration.maxReturnDelay
	simulator, err := flow.Open(flowConfiguration)
	if err != nil {
		fmt.Fprintf(stderr, "sema-tui: open flow simulator: %v\n", err)
		return 1
	}
	defer func() {
		if err := simulator.Close(); err != nil {
			fmt.Fprintf(stderr, "sema-tui: close flow simulator: %v\n", err)
		}
	}()

	options := flowui.DefaultOptions()
	options.Context = ctx
	options.StepInterval = configuration.interval
	options.Width = configuration.width
	options.Height = configuration.height
	options.Unicode = !configuration.ascii
	options.Color = !configuration.noColor && os.Getenv("NO_COLOR") == ""
	options.ReducedMotion = configuration.reducedMotion
	options.Seed = configuration.seed
	model := flowui.New(simulator, options)

	if configuration.snapshot {
		modelOptions := options
		modelOptions.Color = false
		modelOptions.ReducedMotion = true
		model = flowui.New(simulator, modelOptions)
		if err := model.RunSteps(ctx, configuration.steps); err != nil {
			fmt.Fprintf(stderr, "sema-tui: render snapshot: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, model.Content())
		return 0
	}

	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
		tea.WithFPS(30),
	)
	if _, err := program.Run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(stderr, "sema-tui: run: %v\n", err)
		return 1
	}
	return 0
}

func parseConfig(args []string, stderr io.Writer) (config, bool, error) {
	configuration := config{}
	flags := flag.NewFlagSet("sema-tui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Int64Var(&configuration.seed, "seed", 42, "deterministic workload seed")
	flags.DurationVar(&configuration.interval, "interval", 220*time.Millisecond, "lifecycle step interval")
	flags.IntVar(&configuration.population, "population", 1000, "closed player population")
	flags.IntVar(&configuration.matchesPerCycle, "matches-per-cycle", 2, "5v5 proposals targeted per planning cycle")
	flags.DurationVar(&configuration.gameDuration, "game-duration", 45*time.Second, "simulated duration of every match")
	flags.DurationVar(&configuration.arrivalInterval, "arrival-interval", time.Second, "interval between initial party arrivals")
	flags.DurationVar(&configuration.planningInterval, "planning-interval", 5*time.Second, "minimum interval between planning cycles")
	flags.DurationVar(&configuration.maxReturnDelay, "max-return-delay", 30*time.Second, "maximum delay before a completed party returns")
	flags.BoolVar(&configuration.snapshot, "snapshot", false, "render a deterministic non-interactive frame")
	flags.IntVar(&configuration.steps, "steps", 100, "lifecycle operations executed before snapshot rendering")
	flags.IntVar(&configuration.width, "width", 120, "initial or snapshot width")
	flags.IntVar(&configuration.height, "height", 38, "initial or snapshot height")
	flags.BoolVar(&configuration.ascii, "ascii", false, "use the ASCII compatibility glyph set")
	flags.BoolVar(&configuration.noColor, "no-color", false, "disable ANSI colors")
	flags.BoolVar(&configuration.reducedMotion, "reduced-motion", false, "disable staged movement animation")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-tui [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return config{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if configuration.seed < 0 || configuration.interval < 50*time.Millisecond || configuration.interval > 2*time.Second ||
		configuration.population < 10 || configuration.matchesPerCycle <= 0 || configuration.matchesPerCycle > 8 ||
		configuration.gameDuration <= 0 || configuration.arrivalInterval <= 0 || configuration.planningInterval <= 0 ||
		configuration.maxReturnDelay < time.Second || configuration.steps <= 0 ||
		configuration.width < 40 || configuration.height < 18 {
		fmt.Fprintln(stderr, "sema-tui: seed, interval, workload size, steps, width, or height is outside the supported range")
		return config{}, false, fmt.Errorf("invalid flow configuration")
	}
	return configuration, *showVersion, nil
}
