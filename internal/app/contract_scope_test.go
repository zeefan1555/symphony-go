package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractScopeIsDocumentedAndRuntimeAssemblyStaysLayered(t *testing.T) {
	repo := filepath.Join("..", "..")
	scopeDoc := readText(t, filepath.Join(repo, "docs", "contract-scope.md"))
	runSource := readText(t, "run.go")

	for _, want := range []string{
		"long-running automation service",
		"`WORKFLOW.md`",
		"internal/service/workflow",
		"internal/runtime/config",
		"internal/integration/linear",
		"internal/service/orchestrator",
		"internal/service/workspace",
		"internal/service/codex",
		"internal/runtime/logging",
		"internal/runtime/observability",
		"terminal TUI and loopback HTTP control plane are operator surfaces",
		"not a rich web UI or multi-tenant control plane",
	} {
		if !strings.Contains(scopeDoc, want) {
			t.Fatalf("contract scope doc missing %q", want)
		}
	}

	for _, want := range []string{
		"workflow.NewReloader(",
		"linear.New(",
		"workspace.New(",
		"codex.New(",
		"orchestrator.New(",
		"logging.New(",
		"hertzserver.New(",
		"renderTUI(",
	} {
		if !strings.Contains(runSource, want) {
			t.Fatalf("runtime assembly missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"CreateComment",
		"UpsertWorkpad",
		"commentCreate",
		"gh pr",
	} {
		if strings.Contains(runSource, forbidden) {
			t.Fatalf("runtime assembly should not own workflow ticket/PR business logic %q", forbidden)
		}
	}
}

func TestRuntimePolicyDocumentsSecurityAndOperationalSafety(t *testing.T) {
	repo := filepath.Join("..", "..")
	policy := readText(t, filepath.Join(repo, "docs", "runtime-policy.md"))

	for _, want := range []string{
		"high-trust local automation runner",
		"does not claim to provide a strong security sandbox",
		"Operators should tighten these fields",
		"Workflow config supports explicit `$VAR` indirection",
		"must not include API tokens or resolved secret values",
		"Errors may name the missing configuration field or expected environment variable",
		"Workspace hooks are trusted shell scripts",
		"per-issue workspace as cwd",
		"configured hook timeout",
		"shortened previews",
		"untrusted tracker data",
		"reduce the available credentials, tool surface, filesystem writable roots, and network access",
		"The injected `linear_graphql` tool uses the configured Linear credential",
	} {
		if !strings.Contains(policy, want) {
			t.Fatalf("runtime policy missing %q", want)
		}
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
