namespace go api

include "common.thrift"
include "control.thrift"

service SymphonyAPI {
    control.GetScaffoldResp GetScaffold(1: control.GetScaffoldReq req) (api.post="/api/v1/control/get-scaffold")
    control.GetStateResp GetState(1: control.GetStateReq req) (api.post="/api/v1/control/get-state")
    control.RefreshResp Refresh(1: control.RefreshReq req) (api.post="/api/v1/control/refresh")
    control.GetIssueResp GetIssue(1: control.GetIssueReq req) (api.post="/api/v1/control/get-issue")
}
