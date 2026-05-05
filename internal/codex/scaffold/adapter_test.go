package scaffold

import (
	"context"
	"testing"

	generated "github.com/zeefan1555/symphony-go/biz/model/codexsession"
	corecodex "github.com/zeefan1555/symphony-go/internal/codex"
)

func TestAdapterExposesStandardCodexSessionDiagnosticMethod(t *testing.T) {
	var _ interface {
		RunTurn(context.Context, *generated.RunTurnReq) (*generated.CodexTurnSummary, error)
	} = (*Adapter)(nil)
}

func TestRunTurnDelegatesToRunner(t *testing.T) {
	runner := &fakeRunner{result: corecodex.SessionResult{
		SessionID: "session-1",
		ThreadID:  "thread-1",
		Turns: []corecodex.Result{{
			SessionID: "session-1",
			ThreadID:  "thread-1",
			TurnID:    "turn-1",
		}},
	}}
	adapter := NewAdapter(runner)

	summary, err := adapter.RunTurn(context.Background(), &generated.RunTurnReq{
		IssueIdentifier: "ZEE-58",
		WorkspacePath:   "/tmp/workspace",
		PromptName:      "implementation",
		PromptText:      "Implement the requested slice.",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if runner.request.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("workspace path = %q", runner.request.WorkspacePath)
	}
	if runner.request.Issue.Identifier != "ZEE-58" {
		t.Fatalf("issue identifier = %q", runner.request.Issue.Identifier)
	}
	if len(runner.request.Prompts) != 1 || runner.request.Prompts[0].Text != "Implement the requested slice." {
		t.Fatalf("prompts = %#v", runner.request.Prompts)
	}
	if runner.request.AfterTurn != nil {
		t.Fatal("adapter must not expose callback-style continuation in IDL")
	}
	if summary.Boundary == nil {
		t.Fatal("summary boundary is nil")
	}
	if summary.Boundary.HandwrittenAdapter != "internal/codex/scaffold" {
		t.Fatalf("adapter = %q", summary.Boundary.HandwrittenAdapter)
	}
	if summary.SessionID != "session-1" || summary.TurnCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

type fakeRunner struct {
	request corecodex.SessionRequest
	result  corecodex.SessionResult
	err     error
}

func (f *fakeRunner) RunSession(ctx context.Context, request corecodex.SessionRequest, onEvent func(corecodex.Event)) (corecodex.SessionResult, error) {
	f.request = request
	return f.result, f.err
}
