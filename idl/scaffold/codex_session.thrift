include "common.thrift"

namespace go scaffold.codexsession

struct CodexTurnRequest {
    1: required string issue_identifier
    2: required string prompt_name
}

struct CodexTurnSummary {
    1: required common.CapabilityBoundary boundary
    2: required string session_id
    3: required i32 turn_count
}

