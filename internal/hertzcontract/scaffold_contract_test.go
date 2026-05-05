package hertzcontract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOldInternalGeneratedScaffoldChainIsRetired(t *testing.T) {
	repo := "../../"

	makefile := readFile(t, filepath.Join(repo, "Makefile"))
	if strings.Contains(makefile, "hertz-scaffold-generate") {
		t.Fatalf("Makefile must not expose retired hertz-scaffold-generate target")
	}

	for _, retiredPath := range []string{
		"scripts/hertz_scaffold_generate.sh",
		"internal/generated",
		"idl/scaffold",
	} {
		if _, err := os.Stat(filepath.Join(repo, retiredPath)); err == nil {
			t.Fatalf("retired scaffold path still exists: %s", retiredPath)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", retiredPath, err)
		}
	}
}

func TestStandardHertzIDLDefinesOrchestratorDiagnosticEntry(t *testing.T) {
	mainIDL := readFile(t, "../../idl/main.thrift")
	orchestratorIDL := readFile(t, "../../idl/orchestrator.thrift")

	for _, want := range []string{
		"service SymphonyAPI",
		"ProjectIssueRun",
		"ProjectIssueRunReq",
		`api.post="/api/v1/orchestrator/project-issue-run"`,
	} {
		if !strings.Contains(mainIDL, want) {
			t.Fatalf("standard Hertz main IDL missing orchestrator entry %q", want)
		}
	}
	for _, want := range []string{
		"ProjectIssueRunReq",
		"IssueRunProjection",
	} {
		if !strings.Contains(orchestratorIDL, want) {
			t.Fatalf("orchestrator domain IDL missing %q", want)
		}
	}
}

func TestOrchestratorScaffoldIsExposedAsDiagnosticRoute(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "biz/router/api/main.go"))

	for _, want := range []string{
		`_v1.Group("/orchestrator"`,
		`POST("/project-issue-run"`,
		"ProjectIssueRun",
	} {
		if !strings.Contains(controlRoute, want) {
			t.Fatalf("diagnostic route missing orchestrator scaffold route %q", want)
		}
	}
	if strings.Contains(controlRoute, "OrchestratorScaffold") {
		t.Fatalf("diagnostic route must expose the action, not the generated scaffold service name")
	}
}

func TestStandardHertzIDLDefinesWorkspaceDiagnosticEntries(t *testing.T) {
	mainIDL := readFile(t, "../../idl/main.thrift")
	workspaceIDL := readFile(t, "../../idl/workspace.thrift")

	for _, want := range []string{
		"ResolveWorkspacePath",
		"ValidateWorkspacePath",
		"PrepareWorkspace",
		"CleanupWorkspace",
		`api.post="/api/v1/workspace/resolve"`,
		`api.post="/api/v1/workspace/validate"`,
		`api.post="/api/v1/workspace/prepare"`,
		`api.post="/api/v1/workspace/cleanup"`,
	} {
		if !strings.Contains(mainIDL, want) {
			t.Fatalf("standard Hertz main IDL missing workspace entry %q", want)
		}
	}
	for _, want := range []string{
		"ResolveWorkspacePathReq",
		"ValidateWorkspacePathReq",
		"PrepareWorkspaceReq",
		"CleanupWorkspaceReq",
	} {
		if !strings.Contains(workspaceIDL, want) {
			t.Fatalf("workspace domain IDL missing %q", want)
		}
	}
}

func TestWorkspaceScaffoldIsExposedAsDiagnosticRoutes(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "biz/router/api/main.go"))

	for _, want := range []string{
		`_v1.Group("/workspace"`,
		`POST("/resolve"`,
		`POST("/validate"`,
		`POST("/prepare"`,
		`POST("/cleanup"`,
		"ResolveWorkspacePath",
		"ValidateWorkspacePath",
		"PrepareWorkspace",
		"CleanupWorkspace",
	} {
		if !strings.Contains(controlRoute, want) {
			t.Fatalf("diagnostic route missing workspace scaffold route %q", want)
		}
	}
	if strings.Contains(controlRoute, "WorkspaceScaffold") {
		t.Fatalf("diagnostic route must expose actions, not the generated scaffold service name")
	}
}

func TestStandardHertzIDLDefinesCodexSessionDiagnosticEntry(t *testing.T) {
	mainIDL := readFile(t, "../../idl/main.thrift")
	codexIDL := readFile(t, "../../idl/codex_session.thrift")

	for _, want := range []string{
		"RunTurn",
		`api.post="/api/v1/codex-session/run-turn"`,
	} {
		if !strings.Contains(mainIDL, want) {
			t.Fatalf("standard Hertz main IDL missing codex session entry %q", want)
		}
	}
	for _, want := range []string{
		"RunTurnReq",
		"RunTurnResp",
		"CodexTurnSummary",
	} {
		if !strings.Contains(codexIDL, want) {
			t.Fatalf("codex session domain IDL missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"callback",
		"func",
		"app-server",
		"jsonrpc",
		"thread/start",
		"turn/start",
	} {
		if strings.Contains(strings.ToLower(codexIDL), forbidden) {
			t.Fatalf("codex session domain IDL exposes protocol detail %q", forbidden)
		}
	}
}

func TestCodexSessionScaffoldIsExposedAsDiagnosticRoute(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "biz/router/api/main.go"))

	for _, want := range []string{
		`_v1.Group("/codex-session"`,
		`POST("/run-turn"`,
		"RunTurn",
	} {
		if !strings.Contains(controlRoute, want) {
			t.Fatalf("diagnostic route missing codex session scaffold route %q", want)
		}
	}
	if strings.Contains(controlRoute, "CodexSessionScaffold") {
		t.Fatalf("diagnostic route must expose the action, not the generated scaffold service name")
	}
}

func TestStandardHertzIDLDefinesWorkflowDiagnosticEntries(t *testing.T) {
	mainIDL := readFile(t, "../../idl/main.thrift")
	workflowIDL := readFile(t, "../../idl/workflow.thrift")

	for _, want := range []string{
		"LoadWorkflow",
		"RenderWorkflowPrompt",
		`api.post="/api/v1/workflow/load"`,
		`api.post="/api/v1/workflow/render-prompt"`,
	} {
		if !strings.Contains(mainIDL, want) {
			t.Fatalf("standard Hertz main IDL missing workflow entry %q", want)
		}
	}
	for _, want := range []string{
		"LoadWorkflowReq",
		"RenderWorkflowPromptReq",
		"WorkflowSummary",
	} {
		if !strings.Contains(workflowIDL, want) {
			t.Fatalf("workflow domain IDL missing %q", want)
		}
	}
}

func TestWorkflowScaffoldIsExposedAsDiagnosticRoutes(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "biz/router/api/main.go"))

	for _, want := range []string{
		`_v1.Group("/workflow"`,
		`POST("/load"`,
		`POST("/render-prompt"`,
		"LoadWorkflow",
		"RenderWorkflowPrompt",
	} {
		if !strings.Contains(controlRoute, want) {
			t.Fatalf("diagnostic route missing workflow scaffold route %q", want)
		}
	}
	if strings.Contains(controlRoute, "WorkflowScaffold") {
		t.Fatalf("diagnostic route must expose actions, not the generated scaffold service name")
	}
}

func TestInternalScaffoldDocumentation(t *testing.T) {
	doc := readFile(t, "../../docs/internal-scaffold-hertz-idl.md")

	for _, want := range []string{
		"迁移期契约",
		"标准 Hertz 根目录 `biz/...`",
		"已退役遗留",
		"新增业务逻辑必须落到",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("internal scaffold doc missing %q", want)
		}
	}
}

func TestGeneratedHertzBoundaryCheckOnlyCoversAuthoritativeGeneratedShell(t *testing.T) {
	repo := "../../"
	checkScript := readFile(t, filepath.Join(repo, "scripts/check_generated_hertz_boundary.sh"))
	if strings.Contains(checkScript, "internal/generated") {
		t.Fatalf("generated boundary check must not inspect retired internal/generated tree")
	}
}

func TestGeneratedHertzBoundaryCheckCoversBizAndServiceBoundaries(t *testing.T) {
	repo := "../../"
	makefile := readFile(t, filepath.Join(repo, "Makefile"))
	if !strings.Contains(makefile, "check-generated-hertz-boundary") ||
		!strings.Contains(makefile, "scripts/check_generated_hertz_boundary.sh") {
		t.Fatalf("Makefile must expose the generated Hertz boundary check")
	}

	checkScript := readFile(t, filepath.Join(repo, "scripts/check_generated_hertz_boundary.sh"))
	for _, want := range []string{
		"biz/handler",
		"biz/model",
		"biz/router",
		"internal/service",
		"github.com/cloudwego/hertz/pkg/app",
		"app\\.RequestContext",
		"internal/integration",
	} {
		if !strings.Contains(checkScript, want) {
			t.Fatalf("generated Hertz boundary check missing %q", want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
