namespace go orchestrator
include "common.thrift"

struct ProjectIssueRunReq {
    1: required string issue_identifier (api.body="issue_identifier")
}

struct IssueRunProjection {
    1: required common.CapabilityBoundary boundary
    2: required string issue_identifier
    3: required string runtime_state
}

struct ProjectIssueRunResp {
    1: required IssueRunProjection projection
}
