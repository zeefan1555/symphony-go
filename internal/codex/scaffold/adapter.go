package scaffold

import (
	"context"

	corecodex "github.com/zeefan1555/symphony-go/internal/codex"
	generated "github.com/zeefan1555/symphony-go/internal/generated/hertz/scaffold/codexsession"
	"github.com/zeefan1555/symphony-go/internal/generated/hertz/scaffold/model"
	"github.com/zeefan1555/symphony-go/internal/types"
)

type Runner interface {
	RunSession(context.Context, corecodex.SessionRequest, func(corecodex.Event)) (corecodex.SessionResult, error)
}

type Adapter struct {
	runner Runner
}

func NewAdapter(runner Runner) *Adapter {
	return &Adapter{runner: runner}
}

func (a *Adapter) RunTurn(ctx context.Context, request *generated.CodexTurnRequest) (*generated.CodexTurnSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sessionRequest := corecodex.SessionRequest{
		WorkspacePath: request.GetWorkspacePath(),
		Issue:         types.Issue{Identifier: request.GetIssueIdentifier()},
		Prompts: []corecodex.TurnPrompt{{
			Text: request.GetPromptText(),
		}},
	}
	result, err := a.runner.RunSession(ctx, sessionRequest, nil)
	if err != nil {
		return nil, err
	}
	return &generated.CodexTurnSummary{
		Boundary: &model.CapabilityBoundary{
			Name:               "codex_session.turn",
			Purpose:            "Run a single Codex turn through the handwritten Codex runner without exposing app-server protocol details.",
			HandwrittenAdapter: "internal/codex/scaffold",
		},
		SessionID: result.SessionID,
		TurnCount: int32(len(result.Turns)),
	}, nil
}
