namespace go codexsession

include "common.thrift"

struct RunTurnReq {
    1: required string issue_identifier (api.body="issue_identifier");
    2: required string prompt_name (api.body="prompt_name");
    3: required string workspace_path (api.body="workspace_path");
    4: required string prompt_text (api.body="prompt_text");
}

struct RunTurnResp {
    1: CodexTurnSummary summary;
}

struct CodexTurnSummary {
    1: common.CapabilityBoundary boundary;
    2: string session_id;
    3: i32 turn_count;
}
