include "common.thrift"

namespace go scaffold.workflow

struct WorkflowLoadRequest {
    1: required string workflow_path
}

struct WorkflowSummary {
    1: required common.CapabilityBoundary boundary
    2: required string workflow_path
    3: required list<string> state_names
}

struct WorkflowRenderRequest {
    1: required string workflow_path
    2: required string issue_identifier
    3: required string issue_title
    4: required string issue_description
    5: required bool has_attempt
    6: required i32 attempt
}

struct WorkflowRenderResult {
    1: required common.CapabilityBoundary boundary
    2: required string prompt
}

service WorkflowScaffold {
    WorkflowSummary LoadWorkflow(1: required WorkflowLoadRequest request)
    WorkflowRenderResult RenderWorkflowPrompt(1: required WorkflowRenderRequest request)
}
