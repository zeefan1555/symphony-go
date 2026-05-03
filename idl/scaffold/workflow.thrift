include "common.thrift"

namespace go scaffold.workflow

struct WorkflowLoadRequest {
    1: required string workflow_path
}

struct WorkflowSummary {
    1: required common.CapabilityBoundary boundary
    2: required string workflow_path
    3: required list<string> state_names
}

