include "common.thrift"

namespace go scaffold.codexsession

struct CodexTurnRequest {
    1: required string issue_identifier
    2: required string prompt_name
    3: required string workspace_path
    4: required string prompt_text
}

struct CodexTurnSummary {
    1: required common.CapabilityBoundary boundary
    2: required string session_id
    3: required i32 turn_count
}

service CodexSessionScaffold {
    CodexTurnSummary RunTurn(1: required CodexTurnRequest request)
}
