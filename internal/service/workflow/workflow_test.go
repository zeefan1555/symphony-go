package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	issuemodel "symphony-go/internal/service/issue"
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
merge:
  target: release
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
	if loaded.Config.Merge.Target != "release" {
		t.Fatalf("merge target = %q", loaded.Config.Merge.Target)
	}
	attempt := 2
	rendered, err := Render(loaded.PromptTemplate, issuemodel.Issue{
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

func TestRepoWorkflowUsesPRMergeFlow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		".codex/skills/pr/SKILL.md",
		"PR merge flow",
		".codex/skills/pr/scripts/pr_merge_flow.sh",
		"不要在当前 sandbox 内直接把 issue worktree 分支合入 repo root 的 `main`",
		"Merging 快路径",
		"不要重新执行 AI Review",
		"先运行 `.codex/skills/pr/scripts/pr_merge_flow.sh`，再集中更新一次 workpad",
		"PR script 和远端 checks 是 `Merging` 阶段的质量门槛",
		"脚本前不要再执行 `linear auth whoami`",
		"脚本前不要读取完整历史 workpad",
		"如果 PR script 成功但 root `main` 没有同步到 `origin/main`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repo workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"git merge --no-ff <issue-worktree-branch>",
		"验证后 `git push origin main`",
		"`Merging` 阶段不走 PR land",
		"在 `Merging` 中不要创建 PR",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("repo workflow still contains local merge wording %q", forbidden)
		}
	}
}
