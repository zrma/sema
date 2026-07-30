package wireconformance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
)

const (
	// BaseURLEnvironment contains the absolute service URL.
	BaseURLEnvironment = "SEMA_TARGET_BASE_URL"
	// WriteTokenEnvironment contains a same-tenant full-lifecycle token.
	WriteTokenEnvironment = "SEMA_TARGET_WRITE_TOKEN"
	// ReadTokenEnvironment contains a same-tenant match-ticket read token.
	ReadTokenEnvironment = "SEMA_TARGET_READ_TOKEN"
	// OtherTenantTokenEnvironment contains a different-tenant read token.
	OtherTenantTokenEnvironment = "SEMA_TARGET_OTHER_TENANT_TOKEN"

	baseURLEnvironment          = BaseURLEnvironment
	writeTokenEnvironment       = WriteTokenEnvironment
	readTokenEnvironment        = ReadTokenEnvironment
	otherTenantTokenEnvironment = OtherTenantTokenEnvironment
	maximumResponseBytes        = 1 << 20
	compatibilityProgramName    = "sema-target-smoke"
	compatibilityReportSchema   = "sema.target-smoke.v1"
)

// Identity controls the command name, build version, and report schema while
// compatibility and standard commands share one conformance implementation.
type Identity struct {
	ProgramName  string
	Version      string
	ReportSchema string
	APIVersion   string
}

type config struct {
	baseURL          string
	writeToken       string
	readToken        string
	otherTenantToken string
	apiVersion       string
	timeout          time.Duration
	allowHTTP        bool
}

type report struct {
	Schema            string `json:"schema"`
	RunID             string `json:"run_id"`
	Health            bool   `json:"health"`
	Unauthenticated   bool   `json:"unauthenticated"`
	PermissionDenied  bool   `json:"permission_denied"`
	TenantIsolation   bool   `json:"tenant_isolation"`
	LifecycleComplete bool   `json:"lifecycle_complete"`
}

type responseEnvelope struct {
	APIVersion string          `json:"api_version"`
	Data       json.RawMessage `json:"data"`
	Error      *api.Failure    `json:"error"`
}

// ResponseError is a bounded wire failure that excludes response payloads and
// credentials while preserving status/code evidence for conformance checks.
type ResponseError struct {
	Method    string
	Path      string
	Status    int
	Code      string
	Retryable bool
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf(
		"%s %s returned status %d code %q",
		err.Method,
		err.Path,
		err.Status,
		err.Code,
	)
}

// Run executes the authenticated wire-conformance lifecycle.
func Run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	identity Identity,
) int {
	return runWithIdentity(
		ctx,
		args,
		lookupEnvironment,
		rand.Reader,
		stdout,
		stderr,
		time.Now().UTC(),
		identity,
	)
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	random io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	return runWithIdentity(
		ctx,
		args,
		lookupEnvironment,
		random,
		stdout,
		stderr,
		time.Now().UTC(),
		Identity{
			ProgramName:  compatibilityProgramName,
			Version:      "dev",
			ReportSchema: compatibilityReportSchema,
			APIVersion:   api.Version,
		},
	)
}

func runWithIdentity(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	random io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	now time.Time,
	identity Identity,
) int {
	if identity.ProgramName == "" {
		identity.ProgramName = compatibilityProgramName
	}
	if identity.Version == "" {
		identity.Version = "dev"
	}
	if identity.ReportSchema == "" {
		identity.ReportSchema = compatibilityReportSchema
	}
	if identity.APIVersion == "" {
		identity.APIVersion = api.Version
	}
	configuration, showVersion, err := parseConfigWithName(
		args,
		lookupEnvironment,
		stderr,
		identity.ProgramName,
		identity.APIVersion,
	)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "%s %s\n", identity.ProgramName, identity.Version)
		return 0
	}

	validationContext, cancel := context.WithTimeout(ctx, configuration.timeout)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	result, err := validateWithSchema(validationContext, configuration, client, random, now, identity.ReportSchema)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", identity.ProgramName, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintf(stderr, "%s: write report: %v\n", identity.ProgramName, err)
		return 1
	}
	return 0
}

func parseConfig(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
) (config, bool, error) {
	return parseConfigWithName(args, lookupEnvironment, stderr, compatibilityProgramName, api.Version)
}

func parseConfigWithName(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
	programName string,
	defaultAPIVersion string,
) (config, bool, error) {
	configuration := config{timeout: 45 * time.Second, apiVersion: defaultAPIVersion}
	if value, exists := lookupEnvironment(baseURLEnvironment); exists {
		configuration.baseURL = value
	}
	flags := flag.NewFlagSet(programName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configuration.baseURL, "base-url", configuration.baseURL, "target service base URL")
	flags.StringVar(
		&configuration.apiVersion,
		"api-version",
		configuration.apiVersion,
		"service wire version: v1 or v0alpha2",
	)
	flags.DurationVar(&configuration.timeout, "timeout", configuration.timeout, "whole validation timeout")
	flags.BoolVar(&configuration.allowHTTP, "allow-http", false, "allow plaintext HTTP for isolated local tests")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s [flags]\n", programName)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return config{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		return configuration, true, nil
	}

	configuration.writeToken, _ = lookupEnvironment(writeTokenEnvironment)
	configuration.readToken, _ = lookupEnvironment(readTokenEnvironment)
	configuration.otherTenantToken, _ = lookupEnvironment(otherTenantTokenEnvironment)
	if err := validateConfig(configuration); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return config{}, false, err
	}
	return configuration, false, nil
}

func validateConfig(configuration config) error {
	if configuration.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if configuration.apiVersion != "v1" && configuration.apiVersion != api.Version {
		return fmt.Errorf("api version must be v1 or %s", api.Version)
	}
	parsed, err := url.Parse(configuration.baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("base URL must be an absolute URL without credentials, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("base URL must not include a path")
	}
	if parsed.Scheme != "https" && !(configuration.allowHTTP && parsed.Scheme == "http") {
		return fmt.Errorf("base URL must use HTTPS; -allow-http is only for isolated local tests")
	}
	for name, token := range map[string]string{
		writeTokenEnvironment:       configuration.writeToken,
		readTokenEnvironment:        configuration.readToken,
		otherTenantTokenEnvironment: configuration.otherTenantToken,
	} {
		if token == "" || token != strings.TrimSpace(token) || len(token) > 16<<10 {
			return fmt.Errorf("%s must contain one bounded token without surrounding whitespace", name)
		}
	}
	if configuration.writeToken == configuration.readToken ||
		configuration.writeToken == configuration.otherTenantToken ||
		configuration.readToken == configuration.otherTenantToken {
		return fmt.Errorf("write, read-only, and other-tenant tokens must be distinct")
	}
	return nil
}

func validate(
	ctx context.Context,
	configuration config,
	client *http.Client,
	random io.Reader,
	now time.Time,
) (report, error) {
	return validateWithSchema(ctx, configuration, client, random, now, compatibilityReportSchema)
}

func validateWithSchema(
	ctx context.Context,
	configuration config,
	client *http.Client,
	random io.Reader,
	now time.Time,
	reportSchema string,
) (report, error) {
	runID, err := randomID(random)
	if err != nil {
		return report{}, err
	}
	result := report{Schema: reportSchema, RunID: runID}
	baseURL := strings.TrimSuffix(configuration.baseURL, "/")
	apiPrefix := "/" + configuration.apiVersion

	if err := expectHealth(ctx, client, baseURL+"/livez"); err != nil {
		return report{}, fmt.Errorf("liveness: %w", err)
	}
	if err := expectHealth(ctx, client, baseURL+"/readyz"); err != nil {
		return report{}, fmt.Errorf("readiness: %w", err)
	}
	result.Health = true

	firstTicketPath := apiPrefix + "/match-tickets/" + runID + "-ticket-1"
	if err := expectFailure(
		ctx, client, baseURL, "", http.MethodGet, firstTicketPath, nil,
		http.StatusUnauthorized, "Unauthenticated", configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("unauthenticated boundary: %w", err)
	}
	result.Unauthenticated = true

	firstTicket := matchTicket(runID+"-ticket-1", now, 1490)
	if err := expectFailure(
		ctx, client, baseURL, configuration.readToken, http.MethodPut, firstTicketPath, firstTicket,
		http.StatusForbidden, "PermissionDenied", configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("permission boundary: %w", err)
	}
	result.PermissionDenied = true

	for index, skill := range []int{1490, 1510, 1495, 1505} {
		id := fmt.Sprintf("%s-ticket-%d", runID, index+1)
		if _, err := requestDataForVersion[api.MatchTicketMutation](
			ctx, client, baseURL, configuration.writeToken,
			fmt.Sprintf("%s-put-ticket-%d", runID, index+1),
			http.MethodPut, apiPrefix+"/match-tickets/"+id, matchTicket(id, now, skill),
			configuration.apiVersion,
		); err != nil {
			return report{}, fmt.Errorf("create match ticket %d: %w", index+1, err)
		}
	}

	if err := expectFailure(
		ctx, client, baseURL, configuration.otherTenantToken, http.MethodGet, firstTicketPath, nil,
		http.StatusNotFound, "NotFound", configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("tenant isolation: %w", err)
	}
	result.TenantIsolation = true

	policyVersion := runID + "-policy"
	policy := api.MatchmakingPolicy{
		Version: policyVersion, TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 100,
		MaxProposals: 1, MaxSearchNodes: 100_000,
		RelaxationSteps: []api.RelaxationStep{{AfterWaitMillis: 0, MaxTeamSkillGap: 100}},
	}
	if _, err := requestDataForVersion[api.PolicyMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-put-policy",
		http.MethodPut, apiPrefix+"/policies/"+policyVersion, policy, configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("register policy: %w", err)
	}

	planningRunID := runID + "-planning"
	planning, err := requestDataForVersion[api.PlanningRunMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-plan",
		http.MethodPost, apiPrefix+"/planning-runs/"+planningRunID,
		api.PlanningRunRequest{PolicyVersion: policyVersion}, configuration.apiVersion,
	)
	if err != nil {
		return report{}, fmt.Errorf("execute planning run: %w", err)
	}
	if planning.Resource.Status != "completed" || planning.Resource.ProposalCount != 1 {
		return report{}, fmt.Errorf("planning run did not produce exactly one completed proposal")
	}
	proposals, err := requestDataForVersion[api.ProposalPage](
		ctx, client, baseURL, configuration.writeToken, "", http.MethodGet,
		apiPrefix+"/planning-runs/"+planningRunID+"/proposals", nil, configuration.apiVersion,
	)
	if err != nil {
		return report{}, fmt.Errorf("read proposal: %w", err)
	}
	if len(proposals.Items) != 1 {
		return report{}, fmt.Errorf("planning result contains %d proposals; want 1", len(proposals.Items))
	}

	reservationID := runID + "-reservation"
	if _, err := requestDataForVersion[api.ReservationMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-reserve",
		http.MethodPost, apiPrefix+"/reservations/"+reservationID,
		api.ReservationRequest{ProposalID: proposals.Items[0].Proposal.ID}, configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("reserve proposal: %w", err)
	}

	assignmentID := runID + "-assignment"
	if _, err := requestDataForVersion[api.AssignmentMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-confirm",
		http.MethodPost, apiPrefix+"/reservations/"+reservationID+"/confirm",
		api.ConfirmReservationRequest{AssignmentID: assignmentID}, configuration.apiVersion,
	); err != nil {
		return report{}, fmt.Errorf("confirm reservation: %w", err)
	}
	completed, err := requestDataForVersion[api.AssignmentMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-acknowledge",
		http.MethodPost, apiPrefix+"/assignments/"+assignmentID+"/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"}, configuration.apiVersion,
	)
	if err != nil {
		return report{}, fmt.Errorf("acknowledge assignment: %w", err)
	}
	if completed.Resource.Assignment.Status != "completed" {
		return report{}, fmt.Errorf("assignment status is %q; want completed", completed.Resource.Assignment.Status)
	}
	result.LifecycleComplete = true
	return result, nil
}

func randomID(source io.Reader) (string, error) {
	raw := make([]byte, 8)
	if _, err := io.ReadFull(source, raw); err != nil {
		return "", fmt.Errorf("generate run identity: %w", err)
	}
	return "e2e-" + hex.EncodeToString(raw), nil
}

func matchTicket(id string, now time.Time, skill int) api.MatchTicket {
	return api.MatchTicket{
		ID: id, Revision: 1, EnqueuedAt: now.Add(-5 * time.Second),
		Players: []api.Player{{
			ID: "player-" + id, Skill: skill, Role: "flex", LatencyMillis: 30,
		}},
	}
}

func expectHealth(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10)); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d; want 200", response.StatusCode)
	}
	return nil
}

func requestDataForVersion[T any](
	ctx context.Context,
	client *http.Client,
	baseURL string,
	token string,
	operationID string,
	method string,
	path string,
	body any,
	expectedVersion string,
) (T, error) {
	var zero T
	response, err := performRequest(ctx, client, baseURL, token, operationID, method, path, body)
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()
	envelope, err := decodeEnvelope(response.Body)
	if err != nil {
		return zero, err
	}
	if response.StatusCode != http.StatusOK || envelope.Error != nil {
		code := ""
		retryable := false
		if envelope.Error != nil {
			code = envelope.Error.Code
			retryable = envelope.Error.Retryable
		}
		return zero, &ResponseError{
			Method: method, Path: path, Status: response.StatusCode, Code: code, Retryable: retryable,
		}
	}
	if envelope.APIVersion != expectedVersion {
		return zero, fmt.Errorf("%s %s returned API version %q", method, path, envelope.APIVersion)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return zero, fmt.Errorf("decode %s %s response data: %w", method, path, err)
	}
	return data, nil
}

// RequestData executes one strict v0alpha2 request and decodes its data
// envelope. It never includes token or raw response payload in returned errors.
func RequestData[T any](
	ctx context.Context,
	client *http.Client,
	baseURL string,
	token string,
	operationID string,
	method string,
	path string,
	body any,
) (T, error) {
	return requestDataForVersion[T](
		ctx,
		client,
		baseURL,
		token,
		operationID,
		method,
		path,
		body,
		api.Version,
	)
}

func expectFailure(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	token string,
	method string,
	path string,
	body any,
	wantStatus int,
	wantCode string,
	expectedVersion string,
) error {
	response, err := performRequest(ctx, client, baseURL, token, "", method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	envelope, err := decodeEnvelope(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != wantStatus || envelope.APIVersion != expectedVersion ||
		envelope.Error == nil || envelope.Error.Code != wantCode {
		code := ""
		if envelope.Error != nil {
			code = envelope.Error.Code
		}
		return fmt.Errorf(
			"%s %s returned status %d code %q; want %d %q",
			method, path, response.StatusCode, code, wantStatus, wantCode,
		)
	}
	return nil
}

func performRequest(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	token string,
	operationID string,
	method string,
	path string,
	body any,
) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode %s %s request: %w", method, path, err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, payload)
	if err != nil {
		return nil, fmt.Errorf("create %s %s request: %w", method, path, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if operationID != "" {
		request.Header.Set("Idempotency-Key", operationID)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s %s request failed: %w", method, path, err)
	}
	return response, nil
}

func decodeEnvelope(reader io.Reader) (responseEnvelope, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maximumResponseBytes+1))
	if err != nil {
		return responseEnvelope{}, fmt.Errorf("read response envelope: %w", err)
	}
	if len(payload) > maximumResponseBytes {
		return responseEnvelope{}, fmt.Errorf("response envelope exceeds %d bytes", maximumResponseBytes)
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return responseEnvelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	return envelope, nil
}
