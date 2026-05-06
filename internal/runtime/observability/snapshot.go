package observability

import "time"

type Snapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Counts      Counts         `json:"counts"`
	Running     []RunningEntry `json:"running"`
	Retrying    []RetryEntry   `json:"retrying"`
	CodexTotals CodexTotals    `json:"codex_totals"`
	RateLimits  any            `json:"rate_limits"`
	Polling     PollingStatus  `json:"polling"`
	LastError   string         `json:"last_error,omitempty"`
}

type Counts struct {
	Running  int `json:"running"`
	Retrying int `json:"retrying"`
}

type RunningEntry struct {
	IssueID         string     `json:"issue_id"`
	IssueIdentifier string     `json:"issue_identifier"`
	State           string     `json:"state"`
	AgentPhase      string     `json:"agent_phase,omitempty"`
	Stage           string     `json:"stage,omitempty"`
	WorkspacePath   string     `json:"workspace_path,omitempty"`
	Attempt         int        `json:"attempt,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	ThreadID        string     `json:"thread_id,omitempty"`
	TurnID          string     `json:"turn_id,omitempty"`
	PID             int        `json:"pid,omitempty"`
	TurnCount       int        `json:"turn_count"`
	LastEvent       string     `json:"last_event,omitempty"`
	LastMessage     string     `json:"last_message,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	LastEventAt     time.Time  `json:"last_event_at,omitempty"`
	Tokens          TokenUsage `json:"tokens"`
	RuntimeSeconds  float64    `json:"runtime_seconds"`
}

type RetryEntry struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	Attempt         int       `json:"attempt"`
	DueAt           time.Time `json:"due_at"`
	Error           string    `json:"error,omitempty"`
	WorkspacePath   string    `json:"workspace_path,omitempty"`
}

type CodexTotals struct {
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	SecondsRunning float64 `json:"seconds_running"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type PollingStatus struct {
	Checking     bool      `json:"checking"`
	NextPollAt   time.Time `json:"next_poll_at,omitempty"`
	NextPollInMS int64     `json:"next_poll_in_ms"`
	IntervalMS   int       `json:"interval_ms"`
	LastPollAt   time.Time `json:"last_poll_at,omitempty"`
}

func NewSnapshot() Snapshot {
	return Snapshot{
		GeneratedAt: time.Now(),
		Running:     []RunningEntry{},
		Retrying:    []RetryEntry{},
	}
}

func (s Snapshot) TotalRuntimeSeconds(now time.Time) float64 {
	total := s.CodexTotals.SecondsRunning
	for _, entry := range s.Running {
		if !entry.StartedAt.IsZero() {
			total += now.Sub(entry.StartedAt).Seconds()
		}
	}
	return total
}
