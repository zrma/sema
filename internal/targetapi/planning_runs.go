package targetapi

import (
	"net/http"
	"strconv"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/service"
)

func (server *server) postPlanningRun(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPlanningRunsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	runID, ok := planningRunID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command api.PlanningRunRequest
	if !decodeRequest(writer, request, &command) {
		return
	}
	if !validIdentifier(command.PolicyVersion) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "policy version is invalid"))
		return
	}
	result, err := server.planningRuns.Execute(
		request.Context(), principal.Tenant, domain.OperationID(operationID), runID, command.PolicyVersion,
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.PlanningRunMutation{
		Resource: planningRunResource(result.Run), Replayed: result.Replayed,
	})
}

func (server *server) getPlanningRun(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPlanningRunsRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	runID, ok := planningRunID(writer, request)
	if !ok {
		return
	}
	run, exists, err := server.planningRuns.Get(request.Context(), principal.Tenant, runID)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "planning run was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, planningRunResource(run))
}

func (server *server) listPlanningRunProposals(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPlanningRunsRead)
	if !ok {
		return
	}
	runID, ok := planningRunID(writer, request)
	if !ok {
		return
	}
	query, ok := parseQuery(writer, request, "cursor", "limit")
	if !ok {
		return
	}
	limit, ok := planningPageLimit(writer, query)
	if !ok {
		return
	}
	binding := cursorBinding{
		Tenant: principal.Tenant, ResourceKind: string(service.ResourceProposal),
		Filter: "run=" + runID, Order: "proposal_id.asc",
	}
	position, ok := decodePlanningCursor(writer, server.cursors, query, binding)
	if !ok {
		return
	}
	snapshot, err := server.planningRuns.Proposals(request.Context(), principal.Tenant, runID)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if position.RepositoryVersion != 0 && position.RepositoryVersion != snapshot.RunStorageVersion {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "cursor belongs to another planning result"))
		return
	}
	items := make([]api.ProposalResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if string(record.Proposal.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, api.ProposalResource{
			RunID: record.RunID, Proposal: api.FromDomainProposal(record.Proposal),
			StorageVersion: uint64(record.StorageVersion),
		})
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RunStorageVersion,
			After:             items[len(items)-1].Proposal.ID,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.ProposalPage{
		Items: items, RunStorageVersion: uint64(snapshot.RunStorageVersion), NextCursor: next,
	})
}

func (server *server) listPlanningRunUnmatched(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPlanningRunsRead)
	if !ok {
		return
	}
	runID, ok := planningRunID(writer, request)
	if !ok {
		return
	}
	query, ok := parseQuery(writer, request, "cursor", "limit")
	if !ok {
		return
	}
	limit, ok := planningPageLimit(writer, query)
	if !ok {
		return
	}
	binding := cursorBinding{
		Tenant: principal.Tenant, ResourceKind: string(service.ResourcePlanningUnmatched),
		Filter: "run=" + runID, Order: "ticket_id.asc",
	}
	position, ok := decodePlanningCursor(writer, server.cursors, query, binding)
	if !ok {
		return
	}
	snapshot, err := server.planningRuns.Unmatched(request.Context(), principal.Tenant, runID)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if position.RepositoryVersion != 0 && position.RepositoryVersion != snapshot.RunStorageVersion {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "cursor belongs to another planning result"))
		return
	}
	items := make([]api.UnmatchedResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if string(record.Unmatched.Ticket.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, api.UnmatchedResource{
			RunID: record.RunID, Unmatched: api.FromDomainUnmatched(record.Unmatched),
			StorageVersion: uint64(record.StorageVersion),
		})
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RunStorageVersion,
			After:             items[len(items)-1].Unmatched.Ticket.ID,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.UnmatchedPage{
		Items: items, RunStorageVersion: uint64(snapshot.RunStorageVersion), NextCursor: next,
	})
}

func planningRunResource(run service.PlanningRunRecord) api.PlanningRunResource {
	return api.PlanningRunResource{
		ID: run.ID, SnapshotID: string(run.SnapshotID), PolicyVersion: run.PolicyVersion,
		PolicyFingerprint:       string(run.PolicyFingerprint),
		SourceRepositoryVersion: uint64(run.SourceRepositoryVersion),
		CapturedAt:              run.CapturedAt, CompletedAt: optionalAPITime(run.CompletedAt), Status: string(run.Status),
		ProposalCount: run.ProposalCount, UnmatchedCount: run.UnmatchedCount,
		BudgetExhausted: run.BudgetExhausted, Evidence: api.FromDomainBatchEvidence(run.Evidence),
		StorageVersion: uint64(run.StorageVersion),
	}
}

func optionalAPITime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	canonical := value.UTC()
	return &canonical
}

func planningRunID(writer http.ResponseWriter, request *http.Request) (string, bool) {
	runID := request.PathValue("run_id")
	if !validIdentifier(runID) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "planning run ID is invalid"))
		return "", false
	}
	return runID, true
}

func planningPageLimit(writer http.ResponseWriter, query map[string][]string) (int, bool) {
	limit := defaultPageSize
	if value := singleQuery(query, "limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			writeFailure(writer, domain.NewFailure(
				domain.FailureInvalidInput,
				"query parameter limit must be an integer between 1 and %d",
				maxPageSize,
			))
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func decodePlanningCursor(
	writer http.ResponseWriter,
	codec cursorCodec,
	query map[string][]string,
	binding cursorBinding,
) (cursorPosition, bool) {
	token := singleQuery(query, "cursor")
	if token == "" {
		return cursorPosition{}, true
	}
	position, err := codec.decode(token, binding)
	if err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "invalid cursor"))
		return cursorPosition{}, false
	}
	return position, true
}
