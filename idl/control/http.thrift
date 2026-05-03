include "common.thrift"

namespace go control.http

struct IssueRequest {
    1: required string issue_identifier (api.path="issue_identifier")
}

service ControlPlane {
    common.ScaffoldStatus GetScaffold(1: common.Empty request) (api.get="/api/v1/scaffold")
    common.RuntimeState GetState(1: common.Empty request) (api.get="/api/v1/state")
    common.IssueDetail GetIssue(1: IssueRequest request) (api.get="/api/v1/:issue_identifier")
}
