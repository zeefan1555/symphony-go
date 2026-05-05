namespace go workflow

include "common.thrift"

struct LoadWorkflowReq {
    1: required string workflow_path (api.body="workflow_path");
}

struct LoadWorkflowResp {
    1: WorkflowSummary summary;
}

struct RenderWorkflowPromptReq {
    1: required string workflow_path (api.body="workflow_path");
    2: required string issue_identifier (api.body="issue_identifier");
    3: required string issue_title (api.body="issue_title");
    4: required string issue_description (api.body="issue_description");
    5: required bool has_attempt (api.body="has_attempt");
    6: required i32 attempt (api.body="attempt");
}

struct RenderWorkflowPromptResp {
    1: WorkflowRenderResult result;
}

struct WorkflowSummary {
    1: common.CapabilityBoundary boundary;
    2: string workflow_path;
    3: list<string> state_names;
    4: WorkflowIssueFlow issue_flow;
}

struct WorkflowIssueFlow {
    1: list<string> active_states;
    2: list<string> terminal_states;
    3: WorkflowReviewPolicy review_policy;
    4: list<WorkflowPhaseRoute> phase_routes;
    5: list<WorkflowStateTransition> transitions;
    6: list<WorkflowDispatchRule> dispatch_rules;
    7: bool single_agent_session;
    8: list<WorkflowStageFlow> stage_flows;
}

struct WorkflowReviewPolicy {
    1: string mode;
    2: bool allow_manual_ai_review;
    3: string on_ai_fail;
}

struct WorkflowPhaseRoute {
    1: string state;
    2: string phase;
    3: string behavior;
}

struct WorkflowStateTransition {
    1: string from_state;
    2: string to_state;
    3: string owner;
    4: string trigger;
    5: string condition;
}

struct WorkflowDispatchRule {
    1: string state;
    2: string decision;
    3: string reason;
}

struct WorkflowStageFlow {
    1: string state;
    2: string stage;
    3: string session_policy;
    4: string entry_condition;
    5: string action;
    6: string exit_condition;
    7: list<string> next_states;
}

struct WorkflowRenderResult {
    1: common.CapabilityBoundary boundary;
    2: string prompt;
}
