namespace go control
include "common.thrift"

struct GetScaffoldReq {
}

struct GetScaffoldResp {
    1: required string status
}

struct GetStateReq {
}

struct GetStateResp {
    1: required common.RuntimeState state
}

struct RefreshReq {
}

struct RefreshResp {
    1: required bool accepted
    2: required string status
}

struct GetIssueReq {
    1: required string issue_identifier (api.body="issue_identifier")
}

struct GetIssueResp {
    1: required common.IssueDetail issue
}
