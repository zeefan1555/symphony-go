package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	generated "github.com/zeefan1555/symphony-go/biz/model/workflow"
)

func TestAdapterExposesStandardWorkflowDiagnosticMethods(t *testing.T) {
	var _ interface {
		LoadWorkflow(context.Context, *generated.LoadWorkflowReq) (*generated.WorkflowSummary, error)
		RenderWorkflowPrompt(context.Context, *generated.RenderWorkflowPromptReq) (*generated.WorkflowRenderResult, error)
	} = (*Adapter)(nil)
}

func TestLoadWorkflowDelegatesToWorkflowLoader(t *testing.T) {
	path := writeWorkflow(t)
	adapter := NewAdapter()

	summary, err := adapter.LoadWorkflow(context.Background(), &generated.LoadWorkflowReq{WorkflowPath: path})
	if err != nil {
		t.Fatalf("LoadWorkflow() error = %v", err)
	}
	if summary.Boundary == nil {
		t.Fatal("summary boundary is nil")
	}
	if summary.Boundary.HandwrittenAdapter != "internal/workflow/scaffold" {
		t.Fatalf("adapter = %q", summary.Boundary.HandwrittenAdapter)
	}
	if summary.WorkflowPath != path {
		t.Fatalf("workflow path = %q, want %q", summary.WorkflowPath, path)
	}
	if strings.Join(summary.StateNames, ",") != "Todo,In Progress" {
		t.Fatalf("state names = %#v", summary.StateNames)
	}
}

func TestRenderWorkflowPromptDelegatesToWorkflowRender(t *testing.T) {
	path := writeWorkflow(t)
	adapter := NewAdapter()

	rendered, err := adapter.RenderWorkflowPrompt(context.Background(), &generated.RenderWorkflowPromptReq{
		WorkflowPath:     path,
		IssueIdentifier:  "ZEE-59",
		IssueTitle:       "Workflow tracer",
		IssueDescription: "Render through scaffold adapter.",
		HasAttempt:       true,
		Attempt:          2,
	})
	if err != nil {
		t.Fatalf("RenderWorkflowPrompt() error = %v", err)
	}
	for _, want := range []string{"ZEE-59", "Workflow tracer", "Render through scaffold adapter.", "attempt 2"} {
		if !strings.Contains(rendered.Prompt, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered.Prompt)
		}
	}
}

func writeWorkflow(t *testing.T) string {
	t.Helper()
	t.Setenv("LINEAR_API_KEY", "lin_test")
	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  project_slug: demo
  active_states:
    - Todo
    - In Progress
---
Issue {{ issue.identifier }}: {{ issue.title }}
Description: {{ issue.description }}
{% if attempt %}
attempt {{ attempt }}
{% endif %}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
