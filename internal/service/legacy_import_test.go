package service_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

func TestLegacyImporterPreservesSourceAndBuildsCompletedTarget(t *testing.T) {
	path := createLegacyLifecycleJournal(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := repository.NewMemory()
	importer, err := service.NewLegacyImporter(owner, service.LegacyImportOptions{
		Now: func() time.Time { return demandFixtureNow.Add(time.Hour) }, BatchSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "tenant-a", "legacy-import-a", path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.Status.State != service.LegacyImportCompleted ||
		result.Status.SourceRecords == 0 || result.Status.ImportedResources == 0 {
		t.Fatalf("legacy import result = %#v", result)
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
		t.Fatal("legacy importer changed source bytes or metadata")
	}

	status, exists, err := importer.Status(context.Background(), "tenant-a", "legacy-import-a")
	if err != nil || !exists || status.State != service.LegacyImportCompleted {
		t.Fatalf("legacy import status = %#v exists=%t err=%v", status, exists, err)
	}
	if _, err := service.RequireLegacyImportCompleted(
		context.Background(), owner, "tenant-a", "legacy-import-a", status.SourceDigest,
	); err != nil {
		t.Fatal(err)
	}
	replayed, err := importer.Import(context.Background(), "tenant-a", "legacy-import-a", path)
	if err != nil || !replayed.Replayed || replayed.Status.StorageVersion != status.StorageVersion {
		t.Fatalf("replayed legacy import = %#v err=%v; status=%#v", replayed, err, status)
	}

	policies, err := service.NewPolicies(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	if policy, exists, err := policies.Get(context.Background(), "tenant-a", "legacy-policy"); err != nil || !exists || policy.Policy.Version != "legacy-policy" {
		t.Fatalf("imported policy = %#v exists=%t err=%v", policy, exists, err)
	}
	runs, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	run, exists, err := runs.Get(context.Background(), "tenant-a", "legacy-run")
	if err != nil || !exists || run.Status != service.PlanningRunCompleted || run.ProposalCount != 1 {
		t.Fatalf("imported planning run = %#v exists=%t err=%v", run, exists, err)
	}
	proposals, err := runs.Proposals(context.Background(), "tenant-a", "legacy-run")
	if err != nil || len(proposals.Records) != 1 {
		t.Fatalf("imported proposals = %#v err=%v", proposals, err)
	}
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	reservation, exists, err := reservations.Get(context.Background(), "tenant-a", "legacy-reservation")
	if err != nil || !exists || reservation.Reservation.Status != domain.ReservationConfirmed {
		t.Fatalf("imported reservation = %#v exists=%t err=%v", reservation, exists, err)
	}
	assignments, err := service.NewAssignments(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	assignment, exists, err := assignments.Get(context.Background(), "tenant-a", "legacy-assignment")
	if err != nil || !exists || assignment.Assignment.Status != domain.AssignmentCompleted ||
		assignment.Assignment.Acknowledgment == nil {
		t.Fatalf("imported assignment = %#v exists=%t err=%v", assignment, exists, err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	if active, exists, err := tickets.Get(context.Background(), "tenant-a", "legacy-active"); err != nil || !exists || active.Ticket.Revision != 1 {
		t.Fatalf("imported active ticket = %#v exists=%t err=%v", active, exists, err)
	}
}

func TestLegacyImporterMarksPartialTargetForDiscardAndRetry(t *testing.T) {
	path := createLegacyLifecycleJournal(t)
	owner := repository.NewMemory()
	failing := &failCommitRepository{Repository: owner, failAt: 3}
	importer, err := service.NewLegacyImporter(failing, service.LegacyImportOptions{
		Now: func() time.Time { return demandFixtureNow.Add(time.Hour) }, BatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(context.Background(), "tenant-a", "legacy-import-partial", path); err == nil {
		t.Fatal("injected partial import unexpectedly completed")
	}
	status, exists, err := importer.Status(context.Background(), "tenant-a", "legacy-import-partial")
	if err != nil || !exists || status.State != service.LegacyImportInProgress {
		t.Fatalf("partial import status = %#v exists=%t err=%v", status, exists, err)
	}
	retry, err := service.NewLegacyImporter(owner, service.LegacyImportOptions{
		Now: func() time.Time { return demandFixtureNow.Add(2 * time.Hour) }, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retry.Import(
		context.Background(), "tenant-a", "legacy-import-partial", path,
	); !errors.Is(err, service.ErrLegacyImportIncomplete) {
		t.Fatalf("retry incomplete target = %v; want %v", err, service.ErrLegacyImportIncomplete)
	}

	fresh := repository.NewMemory()
	freshImporter, err := service.NewLegacyImporter(fresh, service.LegacyImportOptions{
		Now: func() time.Time { return demandFixtureNow.Add(2 * time.Hour) }, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := freshImporter.Import(
		context.Background(), "tenant-a", "legacy-import-partial", path,
	)
	if err != nil || completed.Status.State != service.LegacyImportCompleted {
		t.Fatalf("discard-and-retry import = %#v err=%v", completed, err)
	}
}

type failCommitRepository struct {
	repository.Repository
	commits int
	failAt  int
}

func (owner *failCommitRepository) Commit(
	ctx context.Context,
	operation repository.Operation,
	mutations []repository.Mutation,
) (repository.CommitResult, error) {
	owner.commits++
	if owner.commits == owner.failAt {
		return repository.CommitResult{}, errors.New("injected legacy import interruption")
	}
	return owner.Repository.Commit(ctx, operation, mutations)
}

func createLegacyLifecycleJournal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-lifecycle.journal")
	runtime, err := durable.Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	policy := servicePolicy("legacy-policy")
	policy.RoleRequirements = nil
	policy.RelaxationSteps = []domain.RelaxationStep{{AfterWait: 0, MaxTeamSkillGap: 100}}
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		id := domain.TicketID(fmt.Sprintf("legacy-ticket-%d", index))
		if err := runtime.SubmitMatchTicket(planningTicket(id)); err != nil {
			_ = runtime.Close()
			t.Fatal(err)
		}
	}
	batch, err := runtime.Plan("legacy-run", demandFixtureNow, policy.Version)
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 {
		_ = runtime.Close()
		t.Fatalf("legacy proposal count = %d; want 1", len(batch.Proposals))
	}
	if _, err := runtime.Reserve(batch.Proposals[0], "legacy-reservation", demandFixtureNow); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if _, err := runtime.Confirm(
		"legacy-reservation", "legacy-assignment", demandFixtureNow.Add(time.Second),
	); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if _, err := runtime.AcknowledgeAssignment(
		"legacy-assignment",
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "legacy-ack", Outcome: domain.AssignmentCompleted,
		},
		demandFixtureNow.Add(2*time.Second),
	); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	active := planningTicket("legacy-active")
	active.EnqueuedAt = demandFixtureNow.Add(3 * time.Second)
	if err := runtime.SubmitMatchTicket(active); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
