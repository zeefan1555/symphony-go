include "common.thrift"

namespace go control.http

service ControlPlane {
    common.ScaffoldStatus GetScaffold(1: common.Empty request) (api.get="/api/v1/scaffold")
}
