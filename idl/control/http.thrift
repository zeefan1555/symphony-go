include "common.thrift"

namespace go control.http

struct IssueRequest {
    1: required string issue_identifier (api.path="issue_identifier")
}

service ControlPlane {
    common.ScaffoldStatus GetScaffold(1: common.Empty request) (api.get="/api/v1/scaffold")
    common.RuntimeState GetState(1: common.Empty request) (api.get="/api/v1/state")
    common.ErrorEnvelope GetRefreshUnsupported(1: common.Empty request) (api.get="/api/v1/refresh")
    common.RefreshResult PostRefresh(1: common.Empty request) (api.post="/api/v1/refresh")
    common.IssueDetail GetIssue(1: IssueRequest request) (api.get="/api/v1/:issue_identifier")
}
