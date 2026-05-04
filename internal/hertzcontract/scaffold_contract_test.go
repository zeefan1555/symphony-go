package hertzcontract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalScaffoldIDLContract(t *testing.T) {
	repo := "../../"

	for _, path := range []string{
		"idl/scaffold/orchestrator.thrift",
		"idl/scaffold/workspace.thrift",
		"idl/scaffold/codex_session.thrift",
		"idl/scaffold/workflow.thrift",
	} {
		text := readFile(t, filepath.Join(repo, path))
		if !strings.Contains(text, "namespace go scaffold.") {
			t.Fatalf("%s must declare a scaffold Go namespace", path)
		}
		if strings.Contains(text, "api.") {
			t.Fatalf("%s is an internal scaffold IDL and must not expose Hertz route annotations", path)
		}
	}
}

func TestInternalScaffoldGenerationEntry(t *testing.T) {
	repo := "../../"

	makefile := readFile(t, filepath.Join(repo, "Makefile"))
	if !strings.Contains(makefile, "hertz-scaffold-generate") {
		t.Fatalf("Makefile must expose hertz-scaffold-generate")
	}

	script := readFile(t, filepath.Join(repo, "scripts/hertz_scaffold_generate.sh"))
	for _, want := range []string{
		"hz model",
		"idl/scaffold/orchestrator.thrift",
		"idl/scaffold/workspace.thrift",
		"idl/scaffold/codex_session.thrift",
		"idl/scaffold/workflow.thrift",
		"internal/generated/hertz/scaffold",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scaffold generation script missing %q", want)
		}
	}
}

func TestOrchestratorScaffoldIDLDefinesGeneratedServiceEntry(t *testing.T) {
	text := readFile(t, "../../idl/scaffold/orchestrator.thrift")

	for _, want := range []string{
		"service OrchestratorScaffold",
		"ProjectIssueRun",
		"IssueRunProjectionRequest",
		"IssueRunProjection",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("orchestrator scaffold IDL missing %q", want)
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

func TestWorkspaceScaffoldIDLDefinesGeneratedServiceEntry(t *testing.T) {
	text := readFile(t, "../../idl/scaffold/workspace.thrift")

	for _, want := range []string{
		"service WorkspaceScaffold",
		"ResolveWorkspacePath",
		"ValidateWorkspacePath",
		"PrepareWorkspace",
		"CleanupWorkspace",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("workspace scaffold IDL missing %q", want)
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

func TestCodexSessionScaffoldIDLDefinesGeneratedServiceEntry(t *testing.T) {
	text := readFile(t, "../../idl/scaffold/codex_session.thrift")

	for _, want := range []string{
		"service CodexSessionScaffold",
		"RunTurn",
		"CodexTurnRequest",
		"CodexTurnSummary",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("codex session scaffold IDL missing %q", want)
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
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("codex session scaffold IDL exposes protocol detail %q", forbidden)
		}
	}
}

func TestCodexSessionScaffoldIsNotExternalControlRoute(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "biz/router/api/main.go"))

	for _, forbidden := range []string{
		"CodexSessionScaffold",
		"RunTurn",
		"codexsession",
		"codex",
	} {
		if strings.Contains(strings.ToLower(controlRoute), strings.ToLower(forbidden)) {
			t.Fatalf("external control route exposes internal codex session scaffold %q", forbidden)
		}
	}
}

func TestWorkflowScaffoldIDLDefinesGeneratedServiceEntry(t *testing.T) {
	text := readFile(t, "../../idl/scaffold/workflow.thrift")

	for _, want := range []string{
		"service WorkflowScaffold",
		"LoadWorkflow",
		"RenderWorkflowPrompt",
		"WorkflowLoadRequest",
		"WorkflowSummary",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow scaffold IDL missing %q", want)
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
		"`idl/scaffold/orchestrator.thrift`",
		"`idl/scaffold/workspace.thrift`",
		"`idl/scaffold/codex_session.thrift`",
		"`idl/scaffold/workflow.thrift`",
		"`internal/generated/hertz/scaffold/`",
		"`scripts/hertz_scaffold_generate.sh`",
		"`make hertz-scaffold-generate`",
		"内部架构脚手架 IDL",
		"控制面 IDL",
		"手写 adapter/service",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("internal scaffold doc missing %q", want)
		}
	}
}

func TestGeneratedHertzScaffoldStaysGeneratedOnly(t *testing.T) {
	repo := "../../"
	checkScript := readFile(t, filepath.Join(repo, "scripts/check_generated_hertz_boundary.sh"))
	if !strings.Contains(checkScript, "internal/generated/hertz") {
		t.Fatalf("generated boundary check must inspect internal/generated/hertz")
	}

	root := filepath.Join(repo, "internal/generated/hertz/scaffold")
	foundGeneratedGo := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		foundGeneratedGo = true
		content := readFile(t, path)
		if !strings.HasPrefix(content, "// Code generated ") {
			t.Fatalf("%s must stay generated-only", path)
		}
		for _, forbidden := range []string{
			"internal/orchestrator",
			"internal/workspace",
			"internal/codex",
			"internal/workflow",
			"internal/issuetracker",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s imports %s; generated scaffold must not own business logic", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated scaffold: %v", err)
	}
	if !foundGeneratedGo {
		t.Fatalf("internal/generated/hertz/scaffold must contain generated Go files")
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
		"internal/issuetracker",
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
