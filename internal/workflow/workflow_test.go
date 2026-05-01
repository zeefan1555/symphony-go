package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeefan1555/symphony-go/internal/types"
)

func TestLoadAndRenderChineseWorkflow(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  project_slug: "demo"
  active_states:
    - Todo
workspace:
  root: .worktrees
---
你正在处理 {{ issue.identifier }}：{{ issue.title }}
{% if attempt %}
第 {{ attempt }} 次续跑
{% endif %}
描述：{{ issue.description }}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.Tracker.ProjectSlug != "demo" {
		t.Fatalf("project slug = %q", loaded.Config.Tracker.ProjectSlug)
	}
	if loaded.Config.Codex.StallTimeoutMS != 300000 {
		t.Fatalf("stall timeout = %d", loaded.Config.Codex.StallTimeoutMS)
	}
	attempt := 2
	rendered, err := Render(loaded.PromptTemplate, types.Issue{
		Identifier:  "ZEE-中文",
		Title:       "中文标题",
		Description: "中文描述",
	}, &attempt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ZEE-中文", "中文标题", "第 2 次续跑", "中文描述"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered)
		}
	}
}

func TestLoadPreservesExplicitCodexStallTimeout(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	for _, tc := range []struct {
		name string
		raw  int
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "WORKFLOW.md")
			content := fmt.Sprintf(`---
tracker:
  kind: linear
  project_slug: "demo"
codex:
  stall_timeout_ms: %d
---
prompt
`, tc.raw)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			loaded, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Config.Codex.StallTimeoutMS != tc.raw {
				t.Fatalf("stall timeout = %d, want %d", loaded.Config.Codex.StallTimeoutMS, tc.raw)
			}
		})
	}
}

func TestLoadDefaultsNullCodexStallTimeout(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  project_slug: "demo"
codex:
  stall_timeout_ms: null
---
prompt
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.Codex.StallTimeoutMS != 300000 {
		t.Fatalf("stall timeout = %d, want default 300000", loaded.Config.Codex.StallTimeoutMS)
	}
}
