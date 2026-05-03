include "common.thrift"

namespace go scaffold.workspace

struct WorkspacePrepareRequest {
    1: required string issue_identifier
    2: required string repo_root
}

struct WorkspacePreparation {
    1: required common.CapabilityBoundary boundary
    2: required string workspace_path
    3: required bool contained_in_root
}

