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
		"internal/service/codex/scaffold",
		"internal/service/orchestrator/scaffold",
		"internal/service/workflow/scaffold",
		"internal/service/workspace/scaffold",
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
	mainIDL := readFile(t, "../../idl/main.proto")
	orchestratorIDL := readFile(t, "../../idl/orchestrator.proto")

	for _, want := range []string{
		"service SymphonyAPI",
		"ProjectIssueRun",
		"ProjectIssueRunReq",
		`(api.post) = "/api/v1/orchestrator/project-issue-run"`,
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
	controlRoute := readFile(t, filepath.Join(repo, "gen/hertz/router/api/main.go"))

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
	mainIDL := readFile(t, "../../idl/main.proto")
	workspaceIDL := readFile(t, "../../idl/workspace.proto")

	for _, want := range []string{
		"ResolveWorkspacePath",
		"ValidateWorkspacePath",
		"PrepareWorkspace",
		"CleanupWorkspace",
		`(api.post) = "/api/v1/workspace/resolve"`,
		`(api.post) = "/api/v1/workspace/validate"`,
		`(api.post) = "/api/v1/workspace/prepare"`,
		`(api.post) = "/api/v1/workspace/cleanup"`,
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
	controlRoute := readFile(t, filepath.Join(repo, "gen/hertz/router/api/main.go"))

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
	mainIDL := readFile(t, "../../idl/main.proto")
	codexIDL := readFile(t, "../../idl/codex_session.proto")

	for _, want := range []string{
		"RunTurn",
		`(api.post) = "/api/v1/codex-session/run-turn"`,
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
	controlRoute := readFile(t, filepath.Join(repo, "gen/hertz/router/api/main.go"))

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
	mainIDL := readFile(t, "../../idl/main.proto")
	workflowIDL := readFile(t, "../../idl/workflow.proto")

	for _, want := range []string{
		"LoadWorkflow",
		"RenderWorkflowPrompt",
		`(api.post) = "/api/v1/workflow/load"`,
		`(api.post) = "/api/v1/workflow/render-prompt"`,
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
	controlRoute := readFile(t, filepath.Join(repo, "gen/hertz/router/api/main.go"))

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

func TestHertzGenerationArchitectureDecisionDocumentsGenHertzShell(t *testing.T) {
	adr := readFile(t, "../../docs/adr/0003-gen-hertz-control-plane-shell.md")
	contextDoc := readFile(t, "../../CONTEXT.md")
	controlDoc := readFile(t, "../../docs/control-plane-hertz-idl.md")
	scaffoldDoc := readFile(t, "../../docs/internal-scaffold-hertz-idl.md")

	for _, want := range []string{
		"`gen/hertz/...`",
		"`internal/transport/hertzbinding`",
		"`internal/service/control`",
		"旧 scaffold",
	} {
		if !strings.Contains(adr, want) {
			t.Fatalf("gen/hertz ADR missing %q", want)
		}
	}
	if strings.Contains(adr, "长期目标为 `biz") {
		t.Fatalf("gen/hertz ADR must not describe biz as the long-term generated shell")
	}

	for name, text := range map[string]string{
		"CONTEXT.md":                     contextDoc,
		"control-plane-hertz-idl.md":     controlDoc,
		"internal-scaffold-hertz-idl.md": scaffoldDoc,
	} {
		if !strings.Contains(text, "`gen/hertz/...`") {
			t.Fatalf("%s must document gen/hertz as the generated shell", name)
		}
		if strings.Contains(text, "标准 Hertz 根目录 `biz/...`") {
			t.Fatalf("%s must not describe biz as the standard long-term generated shell", name)
		}
	}
}

func TestRepositoryUsesLocalModulePath(t *testing.T) {
	goMod := readFile(t, "../../go.mod")
	if !strings.Contains(goMod, "module symphony-go\n") {
		t.Fatalf("go.mod must use local module path symphony-go")
	}
	remoteModule := "github.com/zeefan1555/" + "symphony-go"
	if strings.Contains(goMod, remoteModule) {
		t.Fatalf("go.mod must not use the remote GitHub module path")
	}
}

func TestInternalPackagesDoNotKeepThinDocFiles(t *testing.T) {
	repo := "../../"
	var docFiles []string
	err := filepath.WalkDir(filepath.Join(repo, "internal"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "doc.go" {
			return nil
		}
		docFiles = append(docFiles, strings.TrimPrefix(path, repo))
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	if len(docFiles) > 0 {
		t.Fatalf("thin internal doc.go files must be removed: %v", docFiles)
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
		"gen/hertz/handler",
		"gen/hertz/model",
		"gen/hertz/router",
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
