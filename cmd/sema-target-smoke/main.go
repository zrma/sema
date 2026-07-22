package main

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
	"os"
	"strings"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
)

var version = "dev"

const (
	baseURLEnvironment          = "SEMA_TARGET_BASE_URL"
	writeTokenEnvironment       = "SEMA_TARGET_WRITE_TOKEN"
	readTokenEnvironment        = "SEMA_TARGET_READ_TOKEN"
	otherTenantTokenEnvironment = "SEMA_TARGET_OTHER_TENANT_TOKEN"
	maximumResponseBytes        = 1 << 20
)

type config struct {
	baseURL          string
	writeToken       string
	readToken        string
	otherTenantToken string
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

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.LookupEnv, rand.Reader, os.Stdout, os.Stderr))
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	random io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	configuration, showVersion, err := parseConfig(args, lookupEnvironment, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-target-smoke %s\n", version)
		return 0
	}

	validationContext, cancel := context.WithTimeout(ctx, configuration.timeout)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	result, err := validate(validationContext, configuration, client, random, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(stderr, "sema-target-smoke: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintf(stderr, "sema-target-smoke: write report: %v\n", err)
		return 1
	}
	return 0
}

func parseConfig(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
) (config, bool, error) {
	configuration := config{timeout: 45 * time.Second}
	if value, exists := lookupEnvironment(baseURLEnvironment); exists {
		configuration.baseURL = value
	}
	flags := flag.NewFlagSet("sema-target-smoke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configuration.baseURL, "base-url", configuration.baseURL, "target service base URL")
	flags.DurationVar(&configuration.timeout, "timeout", configuration.timeout, "whole validation timeout")
	flags.BoolVar(&configuration.allowHTTP, "allow-http", false, "allow plaintext HTTP for isolated local tests")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-target-smoke [flags]")
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
		fmt.Fprintf(stderr, "sema-target-smoke: %v\n", err)
		return config{}, false, err
	}
	return configuration, false, nil
}

func validateConfig(configuration config) error {
	if configuration.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
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
	runID, err := randomID(random)
	if err != nil {
		return report{}, err
	}
	result := report{Schema: "sema.target-smoke.v1", RunID: runID}
	baseURL := strings.TrimSuffix(configuration.baseURL, "/")

	if err := expectHealth(ctx, client, baseURL+"/livez"); err != nil {
		return report{}, fmt.Errorf("liveness: %w", err)
	}
	if err := expectHealth(ctx, client, baseURL+"/readyz"); err != nil {
		return report{}, fmt.Errorf("readiness: %w", err)
	}
	result.Health = true

	firstTicketPath := "/v0alpha2/match-tickets/" + runID + "-ticket-1"
	if err := expectFailure(
		ctx, client, baseURL, "", http.MethodGet, firstTicketPath, nil,
		http.StatusUnauthorized, "Unauthenticated",
	); err != nil {
		return report{}, fmt.Errorf("unauthenticated boundary: %w", err)
	}
	result.Unauthenticated = true

	firstTicket := matchTicket(runID+"-ticket-1", now, 1490)
	if err := expectFailure(
		ctx, client, baseURL, configuration.readToken, http.MethodPut, firstTicketPath, firstTicket,
		http.StatusForbidden, "PermissionDenied",
	); err != nil {
		return report{}, fmt.Errorf("permission boundary: %w", err)
	}
	result.PermissionDenied = true

	for index, skill := range []int{1490, 1510, 1495, 1505} {
		id := fmt.Sprintf("%s-ticket-%d", runID, index+1)
		if _, err := requestData[api.MatchTicketMutation](
			ctx, client, baseURL, configuration.writeToken,
			fmt.Sprintf("%s-put-ticket-%d", runID, index+1),
			http.MethodPut, "/v0alpha2/match-tickets/"+id, matchTicket(id, now, skill),
		); err != nil {
			return report{}, fmt.Errorf("create match ticket %d: %w", index+1, err)
		}
	}

	if err := expectFailure(
		ctx, client, baseURL, configuration.otherTenantToken, http.MethodGet, firstTicketPath, nil,
		http.StatusNotFound, "NotFound",
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
	if _, err := requestData[api.PolicyMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-put-policy",
		http.MethodPut, "/v0alpha2/policies/"+policyVersion, policy,
	); err != nil {
		return report{}, fmt.Errorf("register policy: %w", err)
	}

	planningRunID := runID + "-planning"
	planning, err := requestData[api.PlanningRunMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-plan",
		http.MethodPost, "/v0alpha2/planning-runs/"+planningRunID,
		api.PlanningRunRequest{PolicyVersion: policyVersion},
	)
	if err != nil {
		return report{}, fmt.Errorf("execute planning run: %w", err)
	}
	if planning.Resource.Status != "completed" || planning.Resource.ProposalCount != 1 {
		return report{}, fmt.Errorf("planning run did not produce exactly one completed proposal")
	}
	proposals, err := requestData[api.ProposalPage](
		ctx, client, baseURL, configuration.writeToken, "", http.MethodGet,
		"/v0alpha2/planning-runs/"+planningRunID+"/proposals", nil,
	)
	if err != nil {
		return report{}, fmt.Errorf("read proposal: %w", err)
	}
	if len(proposals.Items) != 1 {
		return report{}, fmt.Errorf("planning result contains %d proposals; want 1", len(proposals.Items))
	}

	reservationID := runID + "-reservation"
	if _, err := requestData[api.ReservationMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-reserve",
		http.MethodPost, "/v0alpha2/reservations/"+reservationID,
		api.ReservationRequest{ProposalID: proposals.Items[0].Proposal.ID},
	); err != nil {
		return report{}, fmt.Errorf("reserve proposal: %w", err)
	}

	assignmentID := runID + "-assignment"
	if _, err := requestData[api.AssignmentMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-confirm",
		http.MethodPost, "/v0alpha2/reservations/"+reservationID+"/confirm",
		api.ConfirmReservationRequest{AssignmentID: assignmentID},
	); err != nil {
		return report{}, fmt.Errorf("confirm reservation: %w", err)
	}
	completed, err := requestData[api.AssignmentMutation](
		ctx, client, baseURL, configuration.writeToken, runID+"-acknowledge",
		http.MethodPost, "/v0alpha2/assignments/"+assignmentID+"/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"},
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

func requestData[T any](
	ctx context.Context,
	client *http.Client,
	baseURL string,
	token string,
	operationID string,
	method string,
	path string,
	body any,
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
		if envelope.Error != nil {
			code = envelope.Error.Code
		}
		return zero, fmt.Errorf("%s %s returned status %d code %q", method, path, response.StatusCode, code)
	}
	if envelope.APIVersion != api.Version {
		return zero, fmt.Errorf("%s %s returned API version %q", method, path, envelope.APIVersion)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return zero, fmt.Errorf("decode %s %s response data: %w", method, path, err)
	}
	return data, nil
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
	if response.StatusCode != wantStatus || envelope.APIVersion != api.Version ||
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
