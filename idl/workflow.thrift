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
}

struct WorkflowRenderResult {
    1: common.CapabilityBoundary boundary;
    2: string prompt;
}
