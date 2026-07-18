package durable_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
)

func TestReadLegacyJournalDoesNotMutateSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.journal")
	runtime, err := durable.Open(path, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.MatchmakingPolicy{
		Version: "legacy-policy", TeamCount: 2, TeamSize: 1, MaxLatencyMillis: 100,
	}
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SubmitMatchTicket(domain.MatchTicket{
		ID: "legacy-ticket", Revision: 1, EnqueuedAt: time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC),
		Players: []domain.Player{{ID: "legacy-player", Skill: 1500, LatencyMillis: 20}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	source, err := durable.ReadLegacyJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || beforeInfo.Mode() != afterInfo.Mode() ||
		beforeInfo.Size() != afterInfo.Size() || beforeInfo.ModTime() != afterInfo.ModTime() {
		t.Fatal("read-only legacy load changed source bytes or metadata")
	}
	digest := sha256.Sum256(before)
	if source.Schema != "sema-journal-v1" || source.RecordCount != 3 ||
		source.ReservationTTL != 30*time.Second || source.Digest != hex.EncodeToString(digest[:]) {
		t.Fatalf("legacy source = %#v", source)
	}
	if source.Events[1].Kind != durable.LegacyPolicyRegistered || source.Events[1].Policy == nil ||
		source.Events[2].Kind != durable.LegacyMatchTicketSubmitted || source.Events[2].MatchTicket == nil {
		t.Fatalf("legacy events = %#v", source.Events)
	}
}

func TestReadLegacyJournalRejectsIncompleteTailWithoutRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-incomplete.journal")
	runtime, err := durable.Open(path, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"incomplete":`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := durable.ReadLegacyJournal(path); err == nil {
		t.Fatal("read-only legacy load accepted an incomplete tail")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only legacy load repaired or truncated the source")
	}
}
