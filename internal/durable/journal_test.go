//go:build darwin || linux

package durable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
)

func TestPersistenceFailureReplaysLastSyncedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime, err := Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.journal.file.Close(); err != nil {
		t.Fatal(err)
	}
	readOnly, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime.journal.file = readOnly

	ticket := domain.MatchTicket{
		ID: "failed-persistence", Revision: 1, EnqueuedAt: time.Now().Add(-time.Second),
		Players: []domain.Player{{ID: "failed-persistence-player", Skill: 1000, LatencyMillis: 20}},
	}
	if err := runtime.SubmitMatchTicket(ticket); err == nil || !strings.Contains(err.Error(), "persist") {
		t.Fatalf("persistence failure = %v", err)
	}
	policy := domain.MatchmakingPolicy{
		Version: "rollback-v1", TeamCount: 2, TeamSize: 1, MaxLatencyMillis: 200,
	}
	if _, err := runtime.engine.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.engine.Snapshot("rollback-snapshot", time.Now(), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.MatchTickets) != 0 {
		t.Fatalf("failed mutation remained in memory: %#v", snapshot.MatchTickets)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}
