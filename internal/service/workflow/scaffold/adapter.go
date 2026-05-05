package scaffold

import (
	"context"

	"github.com/zeefan1555/symphony-go/gen/hertz/model/common"
	generated "github.com/zeefan1555/symphony-go/gen/hertz/model/workflow"
	issuemodel "github.com/zeefan1555/symphony-go/internal/service/issue"
	coreworkflow "github.com/zeefan1555/symphony-go/internal/service/workflow"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) LoadWorkflow(ctx context.Context, request *generated.LoadWorkflowReq) (*generated.WorkflowSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := coreworkflow.Load(request.GetWorkflowPath())
	if err != nil {
		return nil, err
	}
	return &generated.WorkflowSummary{
		Boundary:     workflowBoundary(),
		WorkflowPath: request.GetWorkflowPath(),
		StateNames:   append([]string(nil), loaded.Config.Tracker.ActiveStates...),
	}, nil
}

func (a *Adapter) RenderWorkflowPrompt(ctx context.Context, request *generated.RenderWorkflowPromptReq) (*generated.WorkflowRenderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := coreworkflow.Load(request.GetWorkflowPath())
	if err != nil {
		return nil, err
	}
	var attempt *int
	if request.GetHasAttempt() {
		value := int(request.GetAttempt())
		attempt = &value
	}
	prompt, err := coreworkflow.Render(loaded.PromptTemplate, issuemodel.Issue{
		Identifier:  request.GetIssueIdentifier(),
		Title:       request.GetIssueTitle(),
		Description: request.GetIssueDescription(),
	}, attempt)
	if err != nil {
		return nil, err
	}
	return &generated.WorkflowRenderResult{
		Boundary: workflowBoundary(),
		Prompt:   prompt,
	}, nil
}

func workflowBoundary() *common.CapabilityBoundary {
	return &common.CapabilityBoundary{
		Name:               "workflow.load_render",
		Purpose:            "Load workflow configuration and render prompts through the handwritten workflow package.",
		HandwrittenAdapter: "internal/service/workflow/scaffold",
	}
}
