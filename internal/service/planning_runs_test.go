package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

func TestPlanningRunPersistsImmutableSnapshotAndResults(t *testing.T) {
	owner := repository.NewMemory()
	seedPlanningDemand(t, owner, "tenant-a", 4)
	runs, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runs.Execute(context.Background(), "tenant-a", "execute-run-a", "run-a", "policy-plan")
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Run.Status != service.PlanningRunCompleted ||
		created.Run.ProposalCount != 1 || created.Run.UnmatchedCount != 0 {
		t.Fatalf("created planning run = %#v", created)
	}
	proposals, err := runs.Proposals(context.Background(), "tenant-a", "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.Records) != 1 || len(proposals.Records[0].Proposal.Tickets) != 4 {
		t.Fatalf("planning proposals = %#v", proposals)
	}

	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.Cancel(context.Background(), "tenant-a", "cancel-after-plan", "ticket-0", 1); err != nil {
		t.Fatal(err)
	}
	replayed, err := runs.Execute(context.Background(), "tenant-a", "execute-run-a", "run-a", "policy-plan")
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Run.StorageVersion != created.Run.StorageVersion {
		t.Fatalf("historical planning replay = %#v; created=%#v", replayed, created)
	}
	afterMutation, err := runs.Proposals(context.Background(), "tenant-a", "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterMutation.Records) != 1 ||
		afterMutation.Records[0].Proposal.ID != proposals.Records[0].Proposal.ID {
		t.Fatalf("planning result changed after demand mutation: %#v", afterMutation)
	}
	if _, err := runs.Execute(
		context.Background(), "tenant-a", "another-run-a-command", "run-a", "policy-plan",
	); failureCode(err) != domain.FailureInvalidRevision {
		t.Fatalf("duplicate run identity = %v; want %s", err, domain.FailureInvalidRevision)
	}
}

func TestPlanningRunReleasesRepositoryWhileMatcherComputes(t *testing.T) {
	owner := repository.NewMemory()
	seedPlanningDemand(t, owner, "tenant-a", 4)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runs, err := service.NewPlanningRuns(
		owner,
		func() time.Time { return demandFixtureNow },
		func(snapshot domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
			once.Do(func() { close(started) })
			<-release
			return planner.Plan(snapshot)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, executeErr := runs.Execute(
			context.Background(), "tenant-a", "execute-slow-run", "slow-run", "policy-plan",
		)
		result <- executeErr
	}()
	<-started
	pending, exists, err := runs.Get(context.Background(), "tenant-a", "slow-run")
	if err != nil || !exists || pending.Status != service.PlanningRunPlanning {
		t.Fatalf("pending planning run = %#v exists=%t err=%v", pending, exists, err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.Put(
		context.Background(), "tenant-a", "late-ticket-create", planningTicket("ticket-late"),
	); err != nil {
		t.Fatalf("queue ingress while matcher computes: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	proposals, err := runs.Proposals(context.Background(), "tenant-a", "slow-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range proposals.Records {
		for _, ticket := range record.Proposal.Tickets {
			if ticket.ID == "ticket-late" {
				t.Fatal("late queue ingress leaked into immutable planning snapshot")
			}
		}
	}
}

func TestPlanningRunResumesCapturedSnapshotAfterPlannerInterruption(t *testing.T) {
	owner := repository.NewMemory()
	seedPlanningDemand(t, owner, "tenant-a", 4)
	interrupted, err := service.NewPlanningRuns(
		owner,
		func() time.Time { return demandFixtureNow },
		func(domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
			return domain.ProposalBatch{}, errors.New("injected planner interruption")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := interrupted.Execute(
		context.Background(), "tenant-a", "execute-resumable-run", "resumable-run", "policy-plan",
	); err == nil {
		t.Fatal("interrupted planner unexpectedly completed")
	}
	pending, exists, err := interrupted.Get(context.Background(), "tenant-a", "resumable-run")
	if err != nil || !exists || pending.Status != service.PlanningRunPlanning {
		t.Fatalf("captured interrupted run = %#v exists=%t err=%v", pending, exists, err)
	}
	resumed, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow.Add(time.Second) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resumed.Execute(
		context.Background(), "tenant-a", "execute-resumable-run", "resumable-run", "policy-plan",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed || result.Run.Status != service.PlanningRunCompleted || result.Run.ProposalCount != 1 {
		t.Fatalf("resumed planning run = %#v", result)
	}
}

func TestPlanningRunRejectsResultOutsideCapturedSnapshot(t *testing.T) {
	owner := repository.NewMemory()
	seedPlanningDemand(t, owner, "tenant-a", 4)
	runs, err := service.NewPlanningRuns(
		owner,
		func() time.Time { return demandFixtureNow },
		func(snapshot domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
			batch, err := planner.Plan(snapshot)
			if err != nil {
				return domain.ProposalBatch{}, err
			}
			batch.Proposals[0].Tickets[0].Revision++
			batch.Proposals[0].Teams[0].Tickets[0].Revision++
			return batch, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Execute(
		context.Background(), "tenant-a", "execute-invalid-run", "invalid-run", "policy-plan",
	); err == nil {
		t.Fatal("planning result outside captured snapshot was accepted")
	}
	run, exists, err := runs.Get(context.Background(), "tenant-a", "invalid-run")
	if err != nil || !exists || run.Status != service.PlanningRunPlanning {
		t.Fatalf("invalid result run = %#v exists=%t err=%v", run, exists, err)
	}
	if _, err := runs.Proposals(context.Background(), "tenant-a", "invalid-run"); failureCode(err) != domain.FailureInvalidTransition {
		t.Fatalf("invalid result proposal page = %v; want %s", err, domain.FailureInvalidTransition)
	}
}

func TestConcurrentPlanningRetryConvergesOnOneCompletedResult(t *testing.T) {
	owner := repository.NewMemory()
	seedPlanningDemand(t, owner, "tenant-a", 4)
	runs, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan service.PlanningRunMutation, 2)
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, executeErr := runs.Execute(
				context.Background(), "tenant-a", "execute-concurrent-run", "concurrent-run", "policy-plan",
			)
			results <- result
			errors <- executeErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent planning retry: %v", err)
		}
	}
	versions := make(map[repository.Version]bool)
	for result := range results {
		if result.Run.Status != service.PlanningRunCompleted {
			t.Fatalf("concurrent planning result = %#v", result)
		}
		versions[result.Run.StorageVersion] = true
	}
	if len(versions) != 1 {
		t.Fatalf("concurrent planning versions = %#v; want one completion", versions)
	}
	proposals, err := runs.Proposals(context.Background(), "tenant-a", "concurrent-run")
	if err != nil || len(proposals.Records) != 1 {
		t.Fatalf("concurrent planning proposals = %#v err=%v", proposals, err)
	}
}

func seedPlanningDemand(t *testing.T, owner repository.Repository, scope string, ticketCount int) {
	t.Helper()
	policies, err := service.NewPolicies(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	policy := servicePolicy("policy-plan")
	policy.RoleRequirements = nil
	policy.RelaxationSteps = []domain.RelaxationStep{{AfterWait: 0, MaxTeamSkillGap: 100}}
	if _, err := policies.Put(context.Background(), scope, "register-plan-policy", policy); err != nil {
		t.Fatal(err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < ticketCount; index++ {
		id := domain.TicketID(fmt.Sprintf("ticket-%d", index))
		if _, err := tickets.Put(
			context.Background(), scope, domain.OperationID("create-"+id), planningTicket(id),
		); err != nil {
			t.Fatal(err)
		}
	}
}

func planningTicket(id domain.TicketID) domain.MatchTicket {
	return domain.MatchTicket{
		ID: id, Revision: 1, EnqueuedAt: demandFixtureNow.Add(-time.Second),
		Players: []domain.Player{{ID: domain.PlayerID("player-" + id), Skill: 1500, LatencyMillis: 30}},
	}
}
