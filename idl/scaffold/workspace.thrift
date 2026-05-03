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

struct WorkspacePathValidationRequest {
    1: required string workspace_path
}

struct WorkspacePathValidation {
    1: required common.CapabilityBoundary boundary
    2: required string workspace_path
    3: required bool contained_in_root
}

struct WorkspaceCleanupRequest {
    1: required string workspace_path
}

struct WorkspaceCleanupResult {
    1: required common.CapabilityBoundary boundary
    2: required string workspace_path
    3: required bool removed
    4: required bool contained_in_root
}

service WorkspaceScaffold {
    WorkspacePreparation ResolveWorkspacePath(1: required WorkspacePrepareRequest request)
    WorkspacePathValidation ValidateWorkspacePath(1: required WorkspacePathValidationRequest request)
    WorkspacePreparation PrepareWorkspace(1: required WorkspacePrepareRequest request)
    WorkspaceCleanupResult CleanupWorkspace(1: required WorkspaceCleanupRequest request)
}
