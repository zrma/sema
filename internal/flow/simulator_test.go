package flow_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/zrma/sema/internal/flow"
)

func TestSimulatorRunsHTTPMatchLifecycle(t *testing.T) {
	simulator, err := flow.Open(flow.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})

	seen := make(map[flow.EventKind]int)
	for range 40 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		seen[event.Kind]++
		if event.Kind == flow.EventPlanCompleted {
			if event.Batch == nil || len(event.Batch.Proposals) != 2 || len(event.Batch.Unmatched) != 0 {
				t.Fatalf("plan event = %#v", event)
			}
		}
		if seen[flow.EventMatchDeparted] == 2 {
			break
		}
	}

	for _, kind := range []flow.EventKind{
		flow.EventTicketAccepted,
		flow.EventPlanCompleted,
		flow.EventProposalReserved,
		flow.EventAssignmentConfirmed,
		flow.EventMatchDeparted,
	} {
		if seen[kind] == 0 {
			t.Fatalf("lifecycle event %q was not emitted: %#v", kind, seen)
		}
	}
	if seen[flow.EventMatchDeparted] != 2 {
		t.Fatalf("departed matches = %d; want 2", seen[flow.EventMatchDeparted])
	}
}

func TestSimulatorTicketSequenceIsDeterministic(t *testing.T) {
	first := openSimulator(t, 73)
	second := openSimulator(t, 73)

	for step := range 6 {
		left, err := first.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		right, err := second.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(left.Ticket, right.Ticket) {
			t.Fatalf("step %d ticket mismatch:\nleft=%#v\nright=%#v", step, left.Ticket, right.Ticket)
		}
	}
}

func TestSimulatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := flow.Open(flow.Config{Seed: -1}); err == nil {
		t.Fatal("negative seed was accepted")
	}
}

func openSimulator(t *testing.T, seed int64) *flow.Simulator {
	t.Helper()
	configuration := flow.DefaultConfig()
	configuration.Seed = seed
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
