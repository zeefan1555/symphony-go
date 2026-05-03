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

func TestOrchestratorScaffoldIsNotExternalControlRoute(t *testing.T) {
	repo := "../../"
	controlRoute := readFile(t, filepath.Join(repo, "internal/generated/hertz/control/router/control/http/http.go"))

	for _, forbidden := range []string{
		"ProjectIssueRun",
		"OrchestratorScaffold",
		"orchestrator",
	} {
		if strings.Contains(controlRoute, forbidden) {
			t.Fatalf("external control route exposes internal orchestrator scaffold %q", forbidden)
		}
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
			"internal/linear",
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
