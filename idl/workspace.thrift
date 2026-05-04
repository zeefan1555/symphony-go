namespace go workspace

include "common.thrift"

struct ResolveWorkspacePathReq {
    1: required string issue_identifier (api.body="issue_identifier");
}

struct ResolveWorkspacePathResp {
    1: WorkspacePreparation preparation;
}

struct ValidateWorkspacePathReq {
    1: required string workspace_path (api.body="workspace_path");
}

struct ValidateWorkspacePathResp {
    1: WorkspacePathValidation validation;
}

struct PrepareWorkspaceReq {
    1: required string issue_identifier (api.body="issue_identifier");
}

struct PrepareWorkspaceResp {
    1: WorkspacePreparation preparation;
}

struct CleanupWorkspaceReq {
    1: required string workspace_path (api.body="workspace_path");
}

struct CleanupWorkspaceResp {
    1: WorkspaceCleanupResult result;
}

struct WorkspacePreparation {
    1: common.CapabilityBoundary boundary;
    2: string workspace_path;
    3: bool contained_in_root;
}

struct WorkspacePathValidation {
    1: common.CapabilityBoundary boundary;
    2: string workspace_path;
    3: bool contained_in_root;
}

struct WorkspaceCleanupResult {
    1: common.CapabilityBoundary boundary;
    2: string workspace_path;
    3: bool removed;
    4: bool contained_in_root;
}
