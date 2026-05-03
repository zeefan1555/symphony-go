package hertzserver_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/control"
	"github.com/zeefan1555/symphony-go/internal/control/hertzserver"
	"github.com/zeefan1555/symphony-go/internal/observability"
)

type snapshotProvider struct {
	snapshot observability.Snapshot
}

func (p snapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
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
