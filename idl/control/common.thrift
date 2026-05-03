namespace go control.model

struct Empty {
}

struct ScaffoldStatus {
    1: required string status
}

struct RuntimeCounts {
    1: required i32 running
    2: required i32 retrying
}

struct TokenUsage {
    1: required i32 input_tokens
    2: required i32 output_tokens
    3: required i32 total_tokens
}

struct IssueRun {
    1: required string issue_id
    2: required string issue_identifier
    3: required string state
    4: optional string workspace_path
    5: optional string session_id
    6: optional i32 pid
    7: required i32 turn_count
    8: optional string last_event
    9: optional string last_message
    10: optional string started_at
    11: optional string last_event_at
    12: required TokenUsage tokens
    13: required double runtime_seconds
}

struct RetryRun {
    1: required string issue_id
    2: required string issue_identifier
    3: required i32 attempt
    4: required string due_at
    5: optional string error
    6: optional string workspace_path
}

struct CodexTotals {
    1: required i32 input_tokens
    2: required i32 output_tokens
    3: required i32 total_tokens
    4: required double seconds_running
}

struct PollingStatus {
    1: required bool checking
    2: optional string next_poll_at
    3: required i64 next_poll_in_ms
    4: required i32 interval_ms
    5: optional string last_poll_at
}

struct RuntimeState {
    1: required string generated_at
    2: required RuntimeCounts counts
    3: required list<IssueRun> running
    4: required list<RetryRun> retrying
    5: required CodexTotals codex_totals
    6: required PollingStatus polling
    7: optional string last_error
}

struct IssueDetail {
    1: required string issue_id
    2: required string issue_identifier
    3: required string status
    4: optional IssueRun running
    5: optional RetryRun retry
}

struct ErrorDetail {
    1: required string code
    2: required string message
}

struct ErrorEnvelope {
    1: required ErrorDetail error
}
