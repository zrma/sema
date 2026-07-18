package targetapi

import (
	"net/http"
	"strconv"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/service"
)

func (server *server) putPolicy(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPoliciesWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	version, ok := policyVersion(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var policy api.MatchmakingPolicy
	if !decodeRequest(writer, request, &policy) {
		return
	}
	if policy.Version != version {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"path policy version and body policy version differ",
		))
		return
	}
	domainPolicy, err := api.ToDomainPolicy(policy)
	if err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "%v", err))
		return
	}
	result, err := server.policies.Put(
		request.Context(), principal.Tenant, domain.OperationID(operationID), domainPolicy,
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.PolicyMutation{
		Resource: policyResource(service.PolicyRecord{
			Policy: result.Policy, Fingerprint: result.Fingerprint,
			StorageVersion: result.StorageVersion,
		}),
		Replayed: result.Replayed,
	})
}

func (server *server) getPolicy(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPoliciesRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	version, ok := policyVersion(writer, request)
	if !ok {
		return
	}
	record, exists, err := server.policies.Get(request.Context(), principal.Tenant, version)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "policy was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, policyResource(record))
}

func (server *server) listPolicies(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionPoliciesRead)
	if !ok {
		return
	}
	query, ok := parseQuery(writer, request, "cursor", "limit")
	if !ok {
		return
	}
	limit := defaultPageSize
	if value := singleQuery(query, "limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			writeFailure(writer, domain.NewFailure(
				domain.FailureInvalidInput,
				"query parameter limit must be an integer between 1 and %d",
				maxPageSize,
			))
			return
		}
		limit = parsed
	}
	binding := cursorBinding{
		Tenant: principal.Tenant, ResourceKind: string(service.ResourcePolicy),
		Filter: "all", Order: "resource_id.asc",
	}
	var position cursorPosition
	if token := singleQuery(query, "cursor"); token != "" {
		var err error
		position, err = server.cursors.decode(token, binding)
		if err != nil {
			writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "invalid cursor"))
			return
		}
	}
	snapshot, err := server.policies.Snapshot(request.Context(), principal.Tenant)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if position.RepositoryVersion != 0 && position.RepositoryVersion != snapshot.RepositoryVersion {
		writeFailure(writer, domain.NewFailure(
			domain.FailureStaleSnapshot,
			"cursor snapshot changed; restart pagination",
		))
		return
	}
	items := make([]api.PolicyResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if record.Policy.Version <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, policyResource(record))
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RepositoryVersion,
			After:             items[len(items)-1].Policy.Version,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.PolicyPage{
		Items: items, RepositoryVersion: uint64(snapshot.RepositoryVersion), NextCursor: next,
	})
}

func policyResource(record service.PolicyRecord) api.PolicyResource {
	return api.PolicyResource{
		Policy: api.FromDomainPolicy(record.Policy), Fingerprint: string(record.Fingerprint),
		StorageVersion: uint64(record.StorageVersion),
	}
}

func policyVersion(writer http.ResponseWriter, request *http.Request) (string, bool) {
	version := request.PathValue("version")
	if !validIdentifier(version) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "policy version is invalid"))
		return "", false
	}
	return version, true
}
