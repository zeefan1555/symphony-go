include "common.thrift"

namespace go scaffold.orchestrator

struct IssueRunProjectionRequest {
    1: required string issue_identifier
}

struct IssueRunProjection {
    1: required common.CapabilityBoundary boundary
    2: required string issue_identifier
    3: required string runtime_state
}

service OrchestratorScaffold {
    IssueRunProjection ProjectIssueRun(1: required IssueRunProjectionRequest request)
}
