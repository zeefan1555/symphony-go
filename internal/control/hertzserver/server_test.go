package hertzserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/control/hertzserver"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/service/control"
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

	resp, err := http.Get(baseURL + "/api/v1/state")
	if err != nil {
		t.Fatalf("GET /api/v1/state: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.GeneratedAt != "2026-05-04T01:02:03Z" {
		t.Fatalf("generated_at = %q, want fixed timestamp", body.GeneratedAt)
	}
	if body.Counts.Running != 0 || body.Counts.Retrying != 0 {
		t.Fatalf("counts = %#v, want zero counts", body.Counts)
	}
	if body.Running == nil || len(body.Running) != 0 {
		t.Fatalf("running = %#v, want empty JSON array", body.Running)
	}
	if body.Retrying == nil || len(body.Retrying) != 0 {
		t.Fatalf("retrying = %#v, want empty JSON array", body.Retrying)
	}
	if body.Polling.IntervalMS != 30000 {
		t.Fatalf("polling.interval_ms = %d, want 30000", body.Polling.IntervalMS)
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
		LastError: "last error",
	}})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp, err := http.Get(baseURL + "/api/v1/state")
	if err != nil {
		t.Fatalf("GET /api/v1/state: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Counts struct {
			Running  int `json:"running"`
			Retrying int `json:"retrying"`
		} `json:"counts"`
		Running []struct {
			IssueID         string `json:"issue_id"`
			IssueIdentifier string `json:"issue_identifier"`
			State           string `json:"state"`
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
		LastError string `json:"last_error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Counts.Running != 1 || body.Counts.Retrying != 1 {
		t.Fatalf("counts = %#v, want running=1 retrying=1", body.Counts)
	}
	if len(body.Running) != 1 {
		t.Fatalf("running len = %d, want 1", len(body.Running))
	}
	running := body.Running[0]
	if running.IssueIdentifier != "ZEE-47" || running.State != "In Progress" || running.WorkspacePath != "/tmp/ZEE-47" {
		t.Fatalf("running entry = %#v, want ZEE-47 projection", running)
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

	if len(body.Retrying) != 1 {
		t.Fatalf("retrying len = %d, want 1", len(body.Retrying))
	}
	retry := body.Retrying[0]
	if retry.IssueIdentifier != "ZEE-48" || retry.Attempt != 2 || retry.DueAt != "2026-05-04T01:02:48Z" {
		t.Fatalf("retry entry = %#v, want retry projection", retry)
	}
	if retry.Error != "no available orchestrator slots" || retry.WorkspacePath != "/tmp/ZEE-48" {
		t.Fatalf("retry details = %#v, want error/workspace projection", retry)
	}
	if body.CodexTotals.TotalTokens != 150 || body.CodexTotals.SecondsRunning != 300.25 {
		t.Fatalf("codex totals = %#v, want projected totals", body.CodexTotals)
	}
	if !body.Polling.Checking || body.Polling.NextPollInMS != 20000 || body.Polling.IntervalMS != 30000 {
		t.Fatalf("polling = %#v, want projected polling state", body.Polling)
	}
	if body.Polling.LastPollAt != "2026-05-04T01:01:53Z" || body.Polling.NextPollAt != "2026-05-04T01:02:23Z" {
		t.Fatalf("polling timestamps = %#v, want RFC3339 timestamps", body.Polling)
	}
	if body.LastError != "last error" {
		t.Fatalf("last_error = %q, want last error", body.LastError)
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

	resp, err := http.Get(baseURL + "/api/v1/ZEE-48")
	if err != nil {
		t.Fatalf("GET /api/v1/ZEE-48: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.IssueID != "issue-id" || body.IssueIdentifier != "ZEE-48" || body.Status != "running" {
		t.Fatalf("issue detail = %#v, want running ZEE-48", body)
	}
	if body.Running.SessionID != "thread-1" || body.Running.TurnCount != 3 {
		t.Fatalf("running detail = %#v, want session and turn count", body.Running)
	}
	if body.Running.StartedAt != "2026-05-04T01:00:03Z" || body.Running.Tokens.TotalTokens != 15 {
		t.Fatalf("running metrics = %#v, want timestamp and tokens", body.Running)
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

	resp, err := http.Get(baseURL + "/api/v1/ZEE-49")
	if err != nil {
		t.Fatalf("GET /api/v1/ZEE-49: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		IssueIdentifier string `json:"issue_identifier"`
		Status          string `json:"status"`
		Retry           struct {
			Attempt       int    `json:"attempt"`
			DueAt         string `json:"due_at"`
			Error         string `json:"error"`
			WorkspacePath string `json:"workspace_path"`
		} `json:"retry"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.IssueIdentifier != "ZEE-49" || body.Status != "retrying" {
		t.Fatalf("issue detail = %#v, want retrying ZEE-49", body)
	}
	if body.Retry.Attempt != 2 || body.Retry.DueAt != "2026-05-04T01:02:48Z" || body.Retry.Error != "rate limited" {
		t.Fatalf("retry detail = %#v, want retry projection", body.Retry)
	}
}

func TestIssueRouteReturnsErrorEnvelope(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	resp, err := http.Get(baseURL + "/api/v1/ZEE-404")
	if err != nil {
		t.Fatalf("GET /api/v1/ZEE-404: %v", err)
	}
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

	resp, err := http.Get(baseURL + "/api/v1/%20")
	if err != nil {
		t.Fatalf("GET /api/v1/%%20: %v", err)
	}
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

	resp, err := http.Post(baseURL+"/api/v1/refresh", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/v1/refresh: %v", err)
	}
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

	resp, err := http.Post(baseURL+"/api/v1/refresh", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/v1/refresh: %v", err)
	}
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
		method     string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "unsupported method",
			service:    control.NewService(&refreshSnapshotProvider{}),
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "unsupported_method",
		},
		{
			name:       "refresh unavailable",
			service:    control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()}),
			method:     http.MethodPost,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "refresh_unavailable",
		},
		{
			name:       "refresh failed",
			service:    control.NewService(&refreshSnapshotProvider{err: errors.New("poll queue closed")}),
			method:     http.MethodPost,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "refresh_failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := hertzserver.New(tt.service)
			baseURL := startTestServer(t, server)

			req, err := http.NewRequest(tt.method, baseURL+"/api/v1/refresh", strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s /api/v1/refresh: %v", tt.method, err)
			}
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

func TestScaffoldRouteCallsAuthoredControlService(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)
	baseURL := startTestServer(t, server)

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/scaffold", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/scaffold: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("response status = %q, want ok", body.Status)
	}
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
