package hertzserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/service/codex"
	"symphony-go/internal/service/control"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workspace"
	"symphony-go/internal/transport/hertzserver"
)

type snapshotProvider struct {
	snapshot observability.Snapshot
}

type refreshSnapshotProvider struct {
	snapshot observability.Snapshot
	results  []bool
	err      error
	calls    int
}

type fakeCodexRunner struct {
	request codex.SessionRequest
	result  codex.SessionResult
	err     error
}

type fakeTracker struct {
	issues      []issuemodel.Issue
	issue       issuemodel.Issue
	updateID    string
	updateState string
}

func (p snapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func (p *refreshSnapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func (p *refreshSnapshotProvider) RequestRefresh(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.calls++
	if p.err != nil {
		return false, p.err
	}
	if len(p.results) == 0 {
		return false, nil
	}
	result := p.results[0]
	p.results = p.results[1:]
	return result, nil
}

func (f *fakeCodexRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return append([]issuemodel.Issue(nil), f.issues...), nil
}

func (f *fakeTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return append([]issuemodel.Issue(nil), f.issues...), nil
}

func (f *fakeTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	return f.issue, nil
}

func (f *fakeTracker) UpdateIssueState(_ context.Context, issueID, stateName string) error {
	f.updateID = issueID
	f.updateState = stateName
	return nil
}

func TestStateRouteReturnsEmptyRuntimeState(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: generatedAt,
		Running:     []observability.RunningEntry{},
		Retrying:    []observability.RetryEntry{},
		Polling:     observability.PollingStatus{IntervalMS: 30000},
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-state", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		State struct {
			GeneratedAt string `json:"generated_at"`
			Counts      struct {
				Running  int `json:"running"`
				Retrying int `json:"retrying"`
			} `json:"counts"`
			Running  []json.RawMessage `json:"running"`
			Retrying []json.RawMessage `json:"retrying"`
			Polling  struct {
				IntervalMS int `json:"interval_ms"`
			} `json:"polling"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.State.GeneratedAt != "2026-05-04T01:02:03Z" {
		t.Fatalf("generated_at = %q, want fixed timestamp", body.State.GeneratedAt)
	}
	if body.State.Counts.Running != 0 || body.State.Counts.Retrying != 0 {
		t.Fatalf("counts = %#v, want zero counts", body.State.Counts)
	}
	if body.State.Running == nil || len(body.State.Running) != 0 {
		t.Fatalf("running = %#v, want empty JSON array", body.State.Running)
	}
	if body.State.Retrying == nil || len(body.State.Retrying) != 0 {
		t.Fatalf("retrying = %#v, want empty JSON array", body.State.Retrying)
	}
	if body.State.Polling.IntervalMS != 30000 {
		t.Fatalf("polling.interval_ms = %d, want 30000", body.State.Polling.IntervalMS)
	}
}

func TestSpecAliasStateRouteReturnsRuntimeState(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC),
		Running:     []observability.RunningEntry{},
		Retrying:    []observability.RetryEntry{},
		Polling:     observability.PollingStatus{IntervalMS: 45000},
	}})
	baseURL := startTestServer(t, hertzserver.New(service))

	resp := get(t, baseURL, "/api/v1/state")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		State struct {
			Polling struct {
				IntervalMS int `json:"interval_ms"`
			} `json:"polling"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.State.Polling.IntervalMS != 45000 {
		t.Fatalf("polling.interval_ms = %d, want 45000", body.State.Polling.IntervalMS)
	}
}

func TestRootDashboardRouteReturnsHumanReadableRuntimeState(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: generatedAt,
		Counts:      observability.Counts{Running: 1, Retrying: 1},
		Running: []observability.RunningEntry{{
			IssueID:         "issue-id",
			IssueIdentifier: "ZEE-47",
			State:           "In Progress",
			AgentPhase:      "implementer",
			Stage:           "running_agent",
			SessionID:       "thread-1-turn-1",
			PID:             1234,
			TurnCount:       3,
			LastEvent:       "turn_completed",
			Tokens:          observability.TokenUsage{TotalTokens: 15},
		}},
		Retrying: []observability.RetryEntry{{
			IssueID:         "retry-id",
			IssueIdentifier: "ZEE-48",
			Attempt:         2,
			DueAt:           generatedAt.Add(45 * time.Second),
			Error:           "no available orchestrator slots",
		}},
		CodexTotals: observability.CodexTotals{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		Polling:     observability.PollingStatus{IntervalMS: 45000, NextPollInMS: 20000},
		LastError:   "last error",
	}})
	baseURL := startTestServer(t, hertzserver.New(service))

	resp := get(t, baseURL, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", contentType)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"SYMPHONY STATUS",
		"Agents: 1 running / 1 retrying",
		"Tokens: in 100 | out 50 | total 150",
		"Polling: interval=45000ms",
		"Last error: last error",
		"ZEE-47 state=In Progress stage=implementer/running_agent",
		"ZEE-48 attempt=2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard body missing %q:\n%s", want, body)
		}
	}
}

func TestSpecAliasIssueRouteUsesErrorEnvelope(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		Running:  []observability.RunningEntry{},
		Retrying: []observability.RetryEntry{},
	}})
	baseURL := startTestServer(t, hertzserver.New(service))

	resp := get(t, baseURL, "/api/v1/ZEE-404")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "issue_not_found" {
		t.Fatalf("error code = %q, want issue_not_found", body.Error.Code)
	}
}

func TestSpecAliasRefreshRouteQueuesRefresh(t *testing.T) {
	provider := &refreshSnapshotProvider{results: []bool{true}}
	baseURL := startTestServer(t, hertzserver.New(control.NewService(provider)))

	resp := postJSON(t, baseURL, "/api/v1/refresh", "{}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if provider.calls != 1 {
		t.Fatalf("refresh calls = %d, want one", provider.calls)
	}
}

func TestStateRouteReturnsRuntimeProjection(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	startedAt := generatedAt.Add(-2 * time.Minute)
	lastEventAt := generatedAt.Add(-30 * time.Second)
	dueAt := generatedAt.Add(45 * time.Second)
	lastPollAt := generatedAt.Add(-10 * time.Second)
	nextPollAt := generatedAt.Add(20 * time.Second)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: generatedAt,
		Counts:      observability.Counts{Running: 1, Retrying: 1},
		Running: []observability.RunningEntry{{
			IssueID:         "issue-id",
			IssueIdentifier: "ZEE-47",
			State:           "In Progress",
			AgentPhase:      "implementer",
			Stage:           "running_agent",
			WorkspacePath:   "/tmp/ZEE-47",
			SessionID:       "thread-1",
			PID:             1234,
			TurnCount:       3,
			LastEvent:       "turn_completed",
			LastMessage:     "done",
			StartedAt:       startedAt,
			LastEventAt:     lastEventAt,
			Tokens:          observability.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			RuntimeSeconds:  120.5,
		}},
		Retrying: []observability.RetryEntry{{
			IssueID:         "retry-id",
			IssueIdentifier: "ZEE-48",
			Attempt:         2,
			DueAt:           dueAt,
			Error:           "no available orchestrator slots",
			WorkspacePath:   "/tmp/ZEE-48",
		}},
		CodexTotals: observability.CodexTotals{
			InputTokens:    100,
			OutputTokens:   50,
			TotalTokens:    150,
			SecondsRunning: 300.25,
		},
		Polling: observability.PollingStatus{
			Checking:     true,
			LastPollAt:   lastPollAt,
			NextPollAt:   nextPollAt,
			NextPollInMS: 20000,
			IntervalMS:   30000,
		},
		RateLimits: map[string]any{"primary": map[string]any{"remaining": float64(42)}},
		LastError:  "last error",
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-state", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		State struct {
			Counts struct {
				Running  int `json:"running"`
				Retrying int `json:"retrying"`
			} `json:"counts"`
			Running []struct {
				IssueID         string `json:"issue_id"`
				IssueIdentifier string `json:"issue_identifier"`
				State           string `json:"state"`
				AgentPhase      string `json:"agent_phase"`
				Stage           string `json:"stage"`
				WorkspacePath   string `json:"workspace_path"`
				SessionID       string `json:"session_id"`
				PID             int    `json:"pid"`
				TurnCount       int    `json:"turn_count"`
				LastEvent       string `json:"last_event"`
				LastMessage     string `json:"last_message"`
				StartedAt       string `json:"started_at"`
				LastEventAt     string `json:"last_event_at"`
				Tokens          struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"tokens"`
				RuntimeSeconds float64 `json:"runtime_seconds"`
			} `json:"running"`
			Retrying []struct {
				IssueID         string `json:"issue_id"`
				IssueIdentifier string `json:"issue_identifier"`
				Attempt         int    `json:"attempt"`
				DueAt           string `json:"due_at"`
				Error           string `json:"error"`
				WorkspacePath   string `json:"workspace_path"`
			} `json:"retrying"`
			CodexTotals struct {
				InputTokens    int     `json:"input_tokens"`
				OutputTokens   int     `json:"output_tokens"`
				TotalTokens    int     `json:"total_tokens"`
				SecondsRunning float64 `json:"seconds_running"`
			} `json:"codex_totals"`
			Polling struct {
				Checking     bool   `json:"checking"`
				LastPollAt   string `json:"last_poll_at"`
				NextPollAt   string `json:"next_poll_at"`
				NextPollInMS int64  `json:"next_poll_in_ms"`
				IntervalMS   int    `json:"interval_ms"`
			} `json:"polling"`
			RateLimits map[string]map[string]float64 `json:"rate_limits"`
			LastError  string                        `json:"last_error"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	state := body.State
	if state.Counts.Running != 1 || state.Counts.Retrying != 1 {
		t.Fatalf("counts = %#v, want running=1 retrying=1", state.Counts)
	}
	if len(state.Running) != 1 {
		t.Fatalf("running len = %d, want 1", len(state.Running))
	}
	running := state.Running[0]
	if running.IssueIdentifier != "ZEE-47" || running.State != "In Progress" || running.WorkspacePath != "/tmp/ZEE-47" {
		t.Fatalf("running entry = %#v, want ZEE-47 projection", running)
	}
	if running.AgentPhase != "implementer" || running.Stage != "running_agent" {
		t.Fatalf("running phase/stage = %#v, want implementer/running_agent", running)
	}
	if running.SessionID != "thread-1" || running.PID != 1234 || running.TurnCount != 3 {
		t.Fatalf("running execution fields = %#v, want session/pid/turn count", running)
	}
	if running.LastEvent != "turn_completed" || running.LastMessage != "done" {
		t.Fatalf("running last event fields = %#v, want projected event", running)
	}
	if running.StartedAt != "2026-05-04T01:00:03Z" || running.LastEventAt != "2026-05-04T01:01:33Z" {
		t.Fatalf("running time fields = %#v, want RFC3339 timestamps", running)
	}
	if running.Tokens.TotalTokens != 15 || running.RuntimeSeconds != 120.5 {
		t.Fatalf("running metrics = %#v, want token/runtime projection", running)
	}

	if len(state.Retrying) != 1 {
		t.Fatalf("retrying len = %d, want 1", len(state.Retrying))
	}
	retry := state.Retrying[0]
	if retry.IssueIdentifier != "ZEE-48" || retry.Attempt != 2 || retry.DueAt != "2026-05-04T01:02:48Z" {
		t.Fatalf("retry entry = %#v, want retry projection", retry)
	}
	if retry.Error != "no available orchestrator slots" || retry.WorkspacePath != "/tmp/ZEE-48" {
		t.Fatalf("retry details = %#v, want error/workspace projection", retry)
	}
	if state.CodexTotals.TotalTokens != 150 || state.CodexTotals.SecondsRunning != 300.25 {
		t.Fatalf("codex totals = %#v, want projected totals", state.CodexTotals)
	}
	if !state.Polling.Checking || state.Polling.NextPollInMS != 20000 || state.Polling.IntervalMS != 30000 {
		t.Fatalf("polling = %#v, want projected polling state", state.Polling)
	}
	if state.Polling.LastPollAt != "2026-05-04T01:01:53Z" || state.Polling.NextPollAt != "2026-05-04T01:02:23Z" {
		t.Fatalf("polling timestamps = %#v, want RFC3339 timestamps", state.Polling)
	}
	if state.RateLimits["primary"]["remaining"] != 42 {
		t.Fatalf("rate limits = %#v, want primary remaining 42", state.RateLimits)
	}
	if state.LastError != "last error" {
		t.Fatalf("last_error = %q, want last error", state.LastError)
	}
}

func TestObservabilitySnapshotRouteReturnsStableProjection(t *testing.T) {
	generatedAt := time.Date(2026, 5, 6, 1, 2, 3, 0, time.UTC)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: generatedAt,
		Running: []observability.RunningEntry{{
			IssueID:         "issue-1",
			IssueIdentifier: "ZEE-101",
			State:           "In Progress",
		}},
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/observability/get-snapshot", "{}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Boundary struct {
			Name    string `json:"name"`
			Purpose string `json:"purpose"`
		} `json:"boundary"`
		State struct {
			GeneratedAt string `json:"generated_at"`
			Running     []struct {
				IssueIdentifier string `json:"issue_identifier"`
			} `json:"running"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Boundary.Name != "observability.snapshot" {
		t.Fatalf("boundary = %#v, want observability snapshot", body.Boundary)
	}
	if !strings.Contains(body.Boundary.Purpose, "SigNoz/OTLP owns historical metrics") {
		t.Fatalf("boundary purpose = %q, want SigNoz metrics boundary", body.Boundary.Purpose)
	}
	if body.State.GeneratedAt != "2026-05-06T01:02:03Z" || len(body.State.Running) != 1 || body.State.Running[0].IssueIdentifier != "ZEE-101" {
		t.Fatalf("state = %#v, want ZEE-101 runtime projection", body.State)
	}
}

func TestRuntimeSettingsRouteReturnsNonSecretSettings(t *testing.T) {
	service := control.NewServiceWithOptions(control.ServiceOptions{
		Config: runtimeconfig.Config{
			Tracker: runtimeconfig.TrackerConfig{
				Kind:           "linear",
				APIKey:         "lin_secret",
				ProjectSlug:    "demo",
				ActiveStates:   []string{"Todo"},
				TerminalStates: []string{"Done"},
			},
			Server:    runtimeconfig.ServerConfig{Port: 18080, PortSet: true},
			Polling:   runtimeconfig.PollingConfig{IntervalMS: 30000},
			Workspace: runtimeconfig.WorkspaceConfig{Root: "/tmp/workspaces"},
			Merge:     runtimeconfig.MergeConfig{Target: "main"},
			Agent:     runtimeconfig.AgentConfig{MaxConcurrentAgents: 2, MaxTurns: 4, MaxRetryBackoffMS: 60000},
			Codex:     runtimeconfig.CodexConfig{ThreadSandbox: "workspace-write", TurnTimeoutMS: 1000, ReadTimeoutMS: 2000, StallTimeoutMS: 3000},
		},
	})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/runtime/get-settings", "{}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Boundary struct {
			Name string `json:"name"`
		} `json:"boundary"`
		Settings struct {
			TrackerKind         string   `json:"tracker_kind"`
			TrackerProjectSlug  string   `json:"tracker_project_slug"`
			TrackerActiveStates []string `json:"tracker_active_states"`
			ServerPort          int      `json:"server_port"`
			MergeTarget         string   `json:"merge_target"`
			WorkspaceRoot       string   `json:"workspace_root"`
		} `json:"settings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Boundary.Name != "runtime.settings" {
		t.Fatalf("boundary = %#v, want runtime settings", body.Boundary)
	}
	if body.Settings.TrackerKind != "linear" || body.Settings.TrackerProjectSlug != "demo" || strings.Join(body.Settings.TrackerActiveStates, ",") != "Todo" {
		t.Fatalf("tracker settings = %#v, want non-secret tracker settings", body.Settings)
	}
	if body.Settings.ServerPort != 18080 || body.Settings.MergeTarget != "main" || body.Settings.WorkspaceRoot != "/tmp/workspaces" {
		t.Fatalf("runtime settings = %#v, want server/workspace/merge settings", body.Settings)
	}
}

func TestTrackerRoutesDelegateToControlService(t *testing.T) {
	priority := 2
	createdAt := time.Date(2026, 5, 6, 1, 2, 3, 0, time.UTC)
	tracker := &fakeTracker{
		issues: []issuemodel.Issue{{
			ID:         "issue-1",
			Identifier: "ZEE-101",
			Title:      "Expose tracker",
			Priority:   &priority,
			State:      "Todo",
			Labels:     []string{"api"},
			BlockedBy:  []issuemodel.BlockerRef{{ID: "issue-0", Identifier: "ZEE-100", State: "In Progress"}},
			CreatedAt:  &createdAt,
		}},
		issue: issuemodel.Issue{ID: "issue-2", Identifier: "ZEE-102", State: "Done"},
	}
	service := control.NewServiceWithOptions(control.ServiceOptions{Tracker: tracker})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	listResp := postJSON(t, baseURL, "/api/v1/tracker/list-issues", `{"state_names":["Todo"]}`)
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResp.StatusCode, http.StatusOK)
	}
	var listed struct {
		Boundary struct {
			Name string `json:"name"`
		} `json:"boundary"`
		Issues []struct {
			IssueIdentifier string `json:"issue_identifier"`
			Priority        int    `json:"priority"`
			BlockedBy       []struct {
				IssueIdentifier string `json:"issue_identifier"`
			} `json:"blocked_by"`
			CreatedAt string `json:"created_at"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listed.Boundary.Name != "tracker.issue" || len(listed.Issues) != 1 {
		t.Fatalf("listed = %#v, want one tracker issue", listed)
	}
	if listed.Issues[0].IssueIdentifier != "ZEE-101" || listed.Issues[0].Priority != 2 || len(listed.Issues[0].BlockedBy) != 1 || listed.Issues[0].CreatedAt != "2026-05-06T01:02:03Z" {
		t.Fatalf("listed issue = %#v, want projected tracker fields", listed.Issues[0])
	}

	getResp := postJSON(t, baseURL, "/api/v1/tracker/get-issue", `{"issue_identifier":"ZEE-102"}`)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResp.StatusCode, http.StatusOK)
	}
	var fetched struct {
		Issue struct {
			IssueIdentifier string `json:"issue_identifier"`
			State           string `json:"state"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.Issue.IssueIdentifier != "ZEE-102" || fetched.Issue.State != "Done" {
		t.Fatalf("fetched = %#v, want ZEE-102 Done", fetched.Issue)
	}

	updateResp := postJSON(t, baseURL, "/api/v1/tracker/update-issue-state", `{"issue_id":"issue-2","state_name":"Done"}`)
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateResp.StatusCode, http.StatusOK)
	}
	var updated struct {
		IssueID   string `json:"issue_id"`
		StateName string `json:"state_name"`
		Updated   bool   `json:"updated"`
	}
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if tracker.updateID != "issue-2" || tracker.updateState != "Done" || !updated.Updated {
		t.Fatalf("updated = %#v tracker id=%q state=%q", updated, tracker.updateID, tracker.updateState)
	}
}

func TestIssueRouteReturnsRunningDetail(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		Running: []observability.RunningEntry{{
			IssueID:         "issue-id",
			IssueIdentifier: "ZEE-48",
			State:           "In Progress",
			WorkspacePath:   "/tmp/ZEE-48",
			SessionID:       "thread-1",
			PID:             1234,
			TurnCount:       3,
			LastEvent:       "turn_completed",
			LastMessage:     "done",
			StartedAt:       generatedAt.Add(-2 * time.Minute),
			LastEventAt:     generatedAt.Add(-30 * time.Second),
			Tokens:          observability.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			RuntimeSeconds:  120.5,
		}},
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-issue", `{"issue_identifier":"ZEE-48"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Issue struct {
			IssueID         string `json:"issue_id"`
			IssueIdentifier string `json:"issue_identifier"`
			Status          string `json:"status"`
			Running         struct {
				SessionID      string  `json:"session_id"`
				TurnCount      int     `json:"turn_count"`
				LastEvent      string  `json:"last_event"`
				StartedAt      string  `json:"started_at"`
				RuntimeSeconds float64 `json:"runtime_seconds"`
				Tokens         struct {
					TotalTokens int `json:"total_tokens"`
				} `json:"tokens"`
			} `json:"running"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Issue.IssueID != "issue-id" || body.Issue.IssueIdentifier != "ZEE-48" || body.Issue.Status != "running" {
		t.Fatalf("issue detail = %#v, want running ZEE-48", body.Issue)
	}
	if body.Issue.Running.SessionID != "thread-1" || body.Issue.Running.TurnCount != 3 {
		t.Fatalf("running detail = %#v, want session and turn count", body.Issue.Running)
	}
	if body.Issue.Running.StartedAt != "2026-05-04T01:00:03Z" || body.Issue.Running.Tokens.TotalTokens != 15 {
		t.Fatalf("running metrics = %#v, want timestamp and tokens", body.Issue.Running)
	}
}

func TestIssueRouteReturnsRetryingDetail(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		Retrying: []observability.RetryEntry{{
			IssueID:         "retry-id",
			IssueIdentifier: "ZEE-49",
			Attempt:         2,
			DueAt:           generatedAt.Add(45 * time.Second),
			Error:           "rate limited",
			WorkspacePath:   "/tmp/ZEE-49",
		}},
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-issue", `{"issue_identifier":"ZEE-49"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Issue struct {
			IssueIdentifier string `json:"issue_identifier"`
			Status          string `json:"status"`
			Retry           struct {
				Attempt       int    `json:"attempt"`
				DueAt         string `json:"due_at"`
				Error         string `json:"error"`
				WorkspacePath string `json:"workspace_path"`
			} `json:"retry"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Issue.IssueIdentifier != "ZEE-49" || body.Issue.Status != "retrying" {
		t.Fatalf("issue detail = %#v, want retrying ZEE-49", body.Issue)
	}
	if body.Issue.Retry.Attempt != 2 || body.Issue.Retry.DueAt != "2026-05-04T01:02:48Z" || body.Issue.Retry.Error != "rate limited" {
		t.Fatalf("retry detail = %#v, want retry projection", body.Issue.Retry)
	}
}

func TestIssueRouteReturnsErrorEnvelope(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-issue", `{"issue_identifier":"ZEE-404"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "issue_not_found" || body.Error.Message == "" {
		t.Fatalf("error envelope = %#v, want issue_not_found", body.Error)
	}
}

func TestIssueRouteReturnsInvalidIdentifierEnvelope(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-issue", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "invalid_issue_identifier" || body.Error.Message == "" {
		t.Fatalf("error envelope = %#v, want invalid_issue_identifier", body.Error)
	}
}

func TestRefreshRouteQueuesPoll(t *testing.T) {
	provider := &refreshSnapshotProvider{results: []bool{true}}
	service := control.NewService(provider)
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/refresh", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	var body struct {
		Accepted bool   `json:"accepted"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Accepted || body.Status != "queued" {
		t.Fatalf("refresh response = %#v, want queued accepted result", body)
	}
	if provider.calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", provider.calls)
	}
}

func TestRefreshRouteReturnsAlreadyPending(t *testing.T) {
	provider := &refreshSnapshotProvider{results: []bool{false}}
	service := control.NewService(provider)
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/refresh", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	var body struct {
		Accepted bool   `json:"accepted"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Accepted || body.Status != "already_pending" {
		t.Fatalf("refresh response = %#v, want already pending accepted result", body)
	}
}

func TestRefreshRouteReturnsErrorEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		service    *control.Service
		wantStatus int
		wantCode   string
	}{
		{
			name:       "refresh unavailable",
			service:    control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()}),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "refresh_unavailable",
		},
		{
			name:       "refresh failed",
			service:    control.NewService(&refreshSnapshotProvider{err: errors.New("poll queue closed")}),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "refresh_failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := hertzserver.New(tt.service)
			baseURL := startTestServer(t, server)

			resp := postJSON(t, baseURL, "/api/v1/control/refresh", "{}")
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Error.Code != tt.wantCode || body.Error.Message == "" {
				t.Fatalf("error envelope = %#v, want code %q", body.Error, tt.wantCode)
			}
		})
	}
}

func TestRetiredScaffoldRouteIsNotRegistered(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/control/get-scaffold", "{}")
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("retired scaffold route returned %d; route must not be registered", resp.StatusCode)
	}
}

func TestOrchestratorRouteReturnsIssueRunProjection(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.Snapshot{
		Running: []observability.RunningEntry{{
			IssueIdentifier: "ZEE-56",
			State:           "In Progress",
		}},
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/orchestrator/project-issue-run", `{"issue_identifier":"ZEE-56"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Projection struct {
			Boundary struct {
				Name               string `json:"name"`
				HandwrittenAdapter string `json:"handwritten_adapter"`
			} `json:"boundary"`
			IssueIdentifier string `json:"issue_identifier"`
			RuntimeState    string `json:"runtime_state"`
		} `json:"projection"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Projection.Boundary.Name != "orchestrator.issue_run_projection" {
		t.Fatalf("boundary name = %q, want orchestrator issue-run projection", body.Projection.Boundary.Name)
	}
	wantAdapter := "internal/service/control"
	if body.Projection.Boundary.HandwrittenAdapter != wantAdapter {
		t.Fatalf("adapter = %q, want control service", body.Projection.Boundary.HandwrittenAdapter)
	}
	if body.Projection.IssueIdentifier != "ZEE-56" || body.Projection.RuntimeState != "running" {
		t.Fatalf("projection = %#v, want running ZEE-56", body.Projection)
	}
}

func TestOrchestratorRouteReturnsIssueFlowTrunk(t *testing.T) {
	service := control.NewService(nil)
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/orchestrator/get-issue-flow", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Flow struct {
			Boundary struct {
				Name string `json:"name"`
			} `json:"boundary"`
			Name  string `json:"name"`
			Steps []struct {
				Name          string `json:"name"`
				CoreInterface string `json:"core_interface"`
			} `json:"steps"`
			Transitions []struct {
				From            string `json:"from"`
				To              string `json:"to"`
				FailureHandling string `json:"failure_handling"`
			} `json:"transitions"`
			FailurePolicy []string `json:"failure_policy"`
		} `json:"flow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Flow.Boundary.Name != "orchestrator.issue_flow" || body.Flow.Name != "issue-flow-trunk" {
		t.Fatalf("flow header = %#v, want orchestrator issue-flow trunk", body.Flow)
	}
	wantSteps := []string{"Blocked", "Todo", "In Progress", "AI Review", "Merging", "Done"}
	if len(body.Flow.Steps) != len(wantSteps) {
		t.Fatalf("steps = %#v, want trunk steps", body.Flow.Steps)
	}
	for i, want := range wantSteps {
		if body.Flow.Steps[i].Name != want || body.Flow.Steps[i].CoreInterface == "" {
			t.Fatalf("step[%d] = %#v, want %s with core interface", i, body.Flow.Steps[i], want)
		}
	}
	if len(body.Flow.Transitions) != len(wantSteps)-1 || body.Flow.Transitions[0].From != "Blocked" || body.Flow.Transitions[0].To != "Todo" {
		t.Fatalf("transitions = %#v, want trunk transitions", body.Flow.Transitions)
	}
	if body.Flow.Transitions[0].FailureHandling == "" || len(body.Flow.FailurePolicy) == 0 {
		t.Fatalf("flow missing failure handling: %#v", body.Flow)
	}
}

func TestWorkspaceRoutesDelegateToControlService(t *testing.T) {
	root := t.TempDir()
	manager := workspace.New(root, runtimeconfig.HooksConfig{})
	service := control.NewServiceWithWorkspace(snapshotProvider{snapshot: observability.NewSnapshot()}, manager)
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resolveResp := postJSON(t, baseURL, "/api/v1/workspace/resolve", `{"issue_identifier":"../ZEE/unsafe"}`)
	defer resolveResp.Body.Close()
	if resolveResp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d, want %d", resolveResp.StatusCode, http.StatusOK)
	}
	var resolved struct {
		Preparation struct {
			Boundary struct {
				Name string `json:"name"`
			} `json:"boundary"`
			WorkspacePath   string `json:"workspace_path"`
			ContainedInRoot bool   `json:"contained_in_root"`
		} `json:"preparation"`
	}
	if err := json.NewDecoder(resolveResp.Body).Decode(&resolved); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	wantPath := filepath.Join(root, workspace.SafeIdentifier("../ZEE/unsafe"))
	if resolved.Preparation.WorkspacePath != wantPath || !resolved.Preparation.ContainedInRoot {
		t.Fatalf("resolved preparation = %#v, want contained path %q", resolved.Preparation, wantPath)
	}
	if resolved.Preparation.Boundary.Name != "workspace.lifecycle" {
		t.Fatalf("workspace boundary = %q, want lifecycle", resolved.Preparation.Boundary.Name)
	}
	if _, err := os.Stat(resolved.Preparation.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("resolve should not create workspace, stat err=%v", err)
	}

	prepareResp := postJSON(t, baseURL, "/api/v1/workspace/prepare", `{"issue_identifier":"../ZEE/unsafe"}`)
	defer prepareResp.Body.Close()
	if prepareResp.StatusCode != http.StatusOK {
		t.Fatalf("prepare status = %d, want %d", prepareResp.StatusCode, http.StatusOK)
	}
	var prepared struct {
		Preparation struct {
			WorkspacePath   string `json:"workspace_path"`
			ContainedInRoot bool   `json:"contained_in_root"`
		} `json:"preparation"`
	}
	if err := json.NewDecoder(prepareResp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode prepare response: %v", err)
	}
	if prepared.Preparation.WorkspacePath != wantPath || !prepared.Preparation.ContainedInRoot {
		t.Fatalf("prepared workspace = %#v, want contained path %q", prepared.Preparation, wantPath)
	}
	if info, err := os.Stat(prepared.Preparation.WorkspacePath); err != nil || !info.IsDir() {
		t.Fatalf("prepare should create workspace directory, info=%v err=%v", info, err)
	}

	validateResp := postJSON(t, baseURL, "/api/v1/workspace/validate", `{"workspace_path":"`+prepared.Preparation.WorkspacePath+`"}`)
	defer validateResp.Body.Close()
	if validateResp.StatusCode != http.StatusOK {
		t.Fatalf("validate status = %d, want %d", validateResp.StatusCode, http.StatusOK)
	}
	var validated struct {
		Validation struct {
			WorkspacePath   string `json:"workspace_path"`
			ContainedInRoot bool   `json:"contained_in_root"`
		} `json:"validation"`
	}
	if err := json.NewDecoder(validateResp.Body).Decode(&validated); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if validated.Validation.WorkspacePath != prepared.Preparation.WorkspacePath || !validated.Validation.ContainedInRoot {
		t.Fatalf("validation = %#v, want contained prepared workspace", validated.Validation)
	}

	outsidePath := filepath.Join(filepath.Dir(root), "outside")
	invalidResp := postJSON(t, baseURL, "/api/v1/workspace/validate", `{"workspace_path":"`+outsidePath+`"}`)
	defer invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusOK {
		t.Fatalf("invalid validate status = %d, want %d", invalidResp.StatusCode, http.StatusOK)
	}
	var invalid struct {
		Validation struct {
			WorkspacePath   string `json:"workspace_path"`
			ContainedInRoot bool   `json:"contained_in_root"`
		} `json:"validation"`
	}
	if err := json.NewDecoder(invalidResp.Body).Decode(&invalid); err != nil {
		t.Fatalf("decode invalid validate response: %v", err)
	}
	if invalid.Validation.WorkspacePath != outsidePath || invalid.Validation.ContainedInRoot {
		t.Fatalf("invalid validation = %#v, want escaped path marked outside root", invalid.Validation)
	}

	cleanupResp := postJSON(t, baseURL, "/api/v1/workspace/cleanup", `{"workspace_path":"`+prepared.Preparation.WorkspacePath+`"}`)
	defer cleanupResp.Body.Close()
	if cleanupResp.StatusCode != http.StatusOK {
		t.Fatalf("cleanup status = %d, want %d", cleanupResp.StatusCode, http.StatusOK)
	}
	var cleanup struct {
		Result struct {
			WorkspacePath   string `json:"workspace_path"`
			Removed         bool   `json:"removed"`
			ContainedInRoot bool   `json:"contained_in_root"`
		} `json:"result"`
	}
	if err := json.NewDecoder(cleanupResp.Body).Decode(&cleanup); err != nil {
		t.Fatalf("decode cleanup response: %v", err)
	}
	if cleanup.Result.WorkspacePath != prepared.Preparation.WorkspacePath || !cleanup.Result.Removed || !cleanup.Result.ContainedInRoot {
		t.Fatalf("cleanup result = %#v, want removed prepared workspace", cleanup.Result)
	}
	if _, err := os.Stat(prepared.Preparation.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove workspace, stat err=%v", err)
	}
}

func TestWorkflowRoutesDelegateToControlService(t *testing.T) {
	workflowPath := writeWorkflowFile(t)
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	loadResp := postJSON(t, baseURL, "/api/v1/workflow/load", `{"workflow_path":"`+workflowPath+`"}`)
	defer loadResp.Body.Close()
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status = %d, want %d", loadResp.StatusCode, http.StatusOK)
	}
	var loaded struct {
		Summary struct {
			Boundary struct {
				Name string `json:"name"`
			} `json:"boundary"`
			WorkflowPath string   `json:"workflow_path"`
			StateNames   []string `json:"state_names"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(loadResp.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode load response: %v", err)
	}
	if loaded.Summary.Boundary.Name != "workflow.load_render" {
		t.Fatalf("workflow boundary = %q, want load_render", loaded.Summary.Boundary.Name)
	}
	if loaded.Summary.WorkflowPath != workflowPath || strings.Join(loaded.Summary.StateNames, ",") != "Todo,In Progress" {
		t.Fatalf("summary = %#v, want workflow path and active states", loaded.Summary)
	}

	renderResp := postJSON(t, baseURL, "/api/v1/workflow/render-prompt", `{"workflow_path":"`+workflowPath+`","issue_identifier":"ZEE-59","issue_title":"Workflow tracer","issue_description":"Render through HTTP route.","has_attempt":true,"attempt":2}`)
	defer renderResp.Body.Close()
	if renderResp.StatusCode != http.StatusOK {
		t.Fatalf("render status = %d, want %d", renderResp.StatusCode, http.StatusOK)
	}
	var rendered struct {
		Result struct {
			Prompt string `json:"prompt"`
		} `json:"result"`
	}
	if err := json.NewDecoder(renderResp.Body).Decode(&rendered); err != nil {
		t.Fatalf("decode render response: %v", err)
	}
	for _, want := range []string{"ZEE-59", "Workflow tracer", "Render through HTTP route.", "attempt 2"} {
		if !strings.Contains(rendered.Result.Prompt, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered.Result.Prompt)
		}
	}
}

func TestCodexSessionRouteDelegatesToControlService(t *testing.T) {
	runner := &fakeCodexRunner{result: codex.SessionResult{
		SessionID: "session-1",
		ThreadID:  "thread-1",
		Turns: []codex.Result{{
			SessionID: "session-1",
			ThreadID:  "thread-1",
			TurnID:    "turn-1",
		}},
	}}
	service := control.NewServiceWithCodexRunner(snapshotProvider{snapshot: observability.NewSnapshot()}, runner)
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp := postJSON(t, baseURL, "/api/v1/codex-session/run-turn", `{"issue_identifier":"ZEE-58","prompt_name":"implementation","workspace_path":"/tmp/workspace","prompt_text":"Implement the requested slice."}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Summary struct {
			Boundary struct {
				Name               string `json:"name"`
				HandwrittenAdapter string `json:"handwritten_adapter"`
			} `json:"boundary"`
			SessionID string `json:"session_id"`
			TurnCount int32  `json:"turn_count"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if runner.request.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("workspace path = %q", runner.request.WorkspacePath)
	}
	if runner.request.Issue.Identifier != "ZEE-58" {
		t.Fatalf("issue identifier = %q", runner.request.Issue.Identifier)
	}
	if len(runner.request.Prompts) != 1 || runner.request.Prompts[0].Text != "Implement the requested slice." {
		t.Fatalf("prompts = %#v", runner.request.Prompts)
	}
	if body.Summary.Boundary.Name != "codex_session.turn" || body.Summary.Boundary.HandwrittenAdapter != "internal/service/control" {
		t.Fatalf("boundary = %#v, want codex session turn boundary", body.Summary.Boundary)
	}
	if body.Summary.SessionID != "session-1" || body.Summary.TurnCount != 1 {
		t.Fatalf("summary = %#v, want session-1 turn count 1", body.Summary)
	}
}

func postJSON(t *testing.T, baseURL, path, body string) *http.Response {
	t.Helper()

	resp, err := http.Post(baseURL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func get(t *testing.T, baseURL, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func startTestServer(t *testing.T, server *hertzserver.Server) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
		_ = listener.Close()
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get("http://" + listener.Addr().String() + "/ping")
		if err == nil {
			_ = resp.Body.Close()
			return "http://" + listener.Addr().String()
		}
		select {
		case serveErr := <-errCh:
			t.Fatalf("server exited early: %v", serveErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeWorkflowFile(t *testing.T) string {
	t.Helper()
	t.Setenv("LINEAR_API_KEY", "lin_test")
	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: demo
  active_states:
    - Todo
    - In Progress
---
Issue {{ issue.identifier }}: {{ issue.title }}
Description: {{ issue.description }}
{% if attempt %}
attempt {{ attempt }}
{% endif %}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
