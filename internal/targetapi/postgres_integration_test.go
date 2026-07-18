package targetapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/zrma/sema/internal/api/v0alpha2"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var targetSchemaSequence atomic.Uint64

func TestTargetAPIPostgresComposition(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip(postgresTestDSNEnvironment + " is not set")
	}
	schema := fmt.Sprintf("sema_target_api_test_%d", targetSchemaSequence.Add(1))
	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(
		context.Background(),
		"CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(
			context.Background(),
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		)
	})

	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgresrepository.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	owner, err := postgresrepository.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	handler := newTestHandler(t, owner)
	created := requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "postgres-create", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-postgres", matchTicket("ticket-postgres", 1), http.StatusOK,
	)
	polled := requestData[api.MatchTicketResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polled.Ticket, created.Resource.Ticket) || polled.StorageVersion != created.Resource.StorageVersion {
		t.Fatalf("PostgreSQL poll = %#v; created=%#v", polled, created)
	}
	createdBackfill := requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "postgres-backfill-create", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-postgres",
		backfillTicket("backfill-postgres", "session-postgres", 1, 7), http.StatusOK,
	)
	polledBackfill := requestData[api.BackfillTicketResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets/backfill-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polledBackfill.Ticket, createdBackfill.Resource.Ticket) ||
		polledBackfill.StorageVersion != createdBackfill.Resource.StorageVersion {
		t.Fatalf("PostgreSQL backfill poll = %#v; created=%#v", polledBackfill, createdBackfill)
	}
	createdPolicy := requestData[api.PolicyMutation](
		t, handler, "tenant-a", "postgres-policy-create", http.MethodPut,
		"/v0alpha2/policies/policy-postgres", matchmakingPolicy("policy-postgres"), http.StatusOK,
	)
	polledPolicy := requestData[api.PolicyResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/policies/policy-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polledPolicy.Policy, createdPolicy.Resource.Policy) ||
		polledPolicy.Fingerprint != createdPolicy.Resource.Fingerprint ||
		polledPolicy.StorageVersion != createdPolicy.Resource.StorageVersion {
		t.Fatalf("PostgreSQL policy poll = %#v; created=%#v", polledPolicy, createdPolicy)
	}
	planningPolicy := matchmakingPolicy("policy-postgres-planning")
	planningPolicy.RoleRequirements = nil
	requestData[api.PolicyMutation](
		t, handler, "tenant-a", "postgres-planning-policy-create", http.MethodPut,
		"/v0alpha2/policies/policy-postgres-planning", planningPolicy, http.StatusOK,
	)
	for index := 1; index < 4; index++ {
		id := fmt.Sprintf("ticket-postgres-%d", index)
		requestData[api.MatchTicketMutation](
			t, handler, "tenant-a", "postgres-create-"+id, http.MethodPut,
			"/v0alpha2/match-tickets/"+id, matchTicket(id, 1), http.StatusOK,
		)
	}
	createdRun := requestData[api.PlanningRunMutation](
		t, handler, "tenant-a", "postgres-planning-run", http.MethodPost,
		"/v0alpha2/planning-runs/postgres-run",
		api.PlanningRunRequest{PolicyVersion: "policy-postgres-planning"}, http.StatusOK,
	)
	proposals := requestData[api.ProposalPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/planning-runs/postgres-run/proposals", nil, http.StatusOK,
	)
	if createdRun.Resource.Status != "completed" ||
		len(proposals.Items) != createdRun.Resource.ProposalCount {
		t.Fatalf("PostgreSQL planning run = %#v proposals=%#v", createdRun, proposals)
	}
	if len(proposals.Items) == 0 {
		t.Fatal("PostgreSQL planning run returned no reservable proposal")
	}
	createdReservation := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "postgres-reservation-create", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres",
		api.ReservationRequest{ProposalID: proposals.Items[0].Proposal.ID}, http.StatusOK,
	)
	polledReservation := requestData[api.ReservationResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/reservations/reservation-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polledReservation.Reservation, createdReservation.Resource.Reservation) ||
		polledReservation.StorageVersion != createdReservation.Resource.StorageVersion {
		t.Fatalf("PostgreSQL reservation poll = %#v; created=%#v", polledReservation, createdReservation)
	}
	requestData[api.ReservationMutation](
		t, handler, "tenant-a", "postgres-reservation-cancel", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres/cancel", struct{}{}, http.StatusOK,
	)
	replayedReservation := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "postgres-reservation-create", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres",
		api.ReservationRequest{ProposalID: proposals.Items[0].Proposal.ID}, http.StatusOK,
	)
	if !replayedReservation.Replayed ||
		!reflect.DeepEqual(replayedReservation.Resource, createdReservation.Resource) {
		t.Fatalf("PostgreSQL reservation replay = %#v; created=%#v", replayedReservation, createdReservation)
	}
	requestData[api.ReservationMutation](
		t, handler, "tenant-a", "postgres-reservation-for-assignment", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres-assignment",
		api.ReservationRequest{ProposalID: proposals.Items[0].Proposal.ID}, http.StatusOK,
	)
	confirmedAssignment := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "postgres-assignment-confirm", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres-assignment/confirm",
		api.ConfirmReservationRequest{AssignmentID: "assignment-postgres"}, http.StatusOK,
	)
	polledAssignment := requestData[api.AssignmentResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/assignments/assignment-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polledAssignment.Assignment, confirmedAssignment.Resource.Assignment) ||
		polledAssignment.StorageVersion != confirmedAssignment.Resource.StorageVersion {
		t.Fatalf("PostgreSQL assignment poll = %#v; confirmed=%#v", polledAssignment, confirmedAssignment)
	}
	acknowledgment := api.AcknowledgeAssignmentRequest{Outcome: "completed"}
	if target := confirmedAssignment.Resource.Assignment.Backfill; target != nil {
		acknowledgment.SessionID = target.SessionID
		acknowledgment.ExpectedRosterVersion = target.RosterVersion
		acknowledgment.ResultingRosterVersion = target.RosterVersion + 1
	}
	completedAssignment := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "postgres-assignment-ack", http.MethodPost,
		"/v0alpha2/assignments/assignment-postgres/acknowledgments",
		acknowledgment, http.StatusOK,
	)
	if completedAssignment.Resource.Assignment.Status != "completed" {
		t.Fatalf("PostgreSQL completed assignment = %#v", completedAssignment)
	}
	replayedAssignment := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "postgres-assignment-confirm", http.MethodPost,
		"/v0alpha2/reservations/reservation-postgres-assignment/confirm",
		api.ConfirmReservationRequest{AssignmentID: "assignment-postgres"}, http.StatusOK,
	)
	if !replayedAssignment.Replayed ||
		!reflect.DeepEqual(replayedAssignment.Resource, confirmedAssignment.Resource) {
		t.Fatalf("PostgreSQL assignment replay = %#v; confirmed=%#v", replayedAssignment, confirmedAssignment)
	}
}
