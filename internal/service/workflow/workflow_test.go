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

func TestRepoWorkflowUsesElixirStyleAIReviewLandFlow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"active_states:\n    - Todo\n    - In Progress\n    - AI Review\n    - Merging\n    - Rework",
		"本 workflow 使用 `AI Review` 作为示例 workflow 里 `Human Review` 的自动审核位置",
		"`push`：实现阶段创建或更新 PR，并把 PR 链接回 Linear",
		"`land`：当 issue 进入 `Merging` 时",
		"PR feedback sweep protocol",
		"每一条 actionable reviewer comment",
		"使用 `push` skill push branch，创建或更新 PR",
		"当 issue 处于 `AI Review`，reviewer agent 审查 issue、workpad、PR",
		"打开并遵守 `.codex/skills/land/SKILL.md`",
		"不要直接调用 `gh pr merge`",
		"PR checks 绿色，branch 已 push，PR 已链接到 issue",
		"使用 `linear_graphql`，不要使用 Linear MCP/app 工具",
		"读取 issue、team states、comments：使用 `linear_graphql` query",
		"更新 issue 状态：先读取 team states 拿到目标 `stateId`，再使用 `linear_graphql` 的 `issueUpdate` mutation",
		"创建或更新 `## Codex Workpad`：使用 `linear_graphql` 的 `commentCreate` / `commentUpdate` mutation",
		"不调用 Linear MCP/app issue/comment 工具作为兜底",
		"通过 `linear_graphql` issue update mutation 将状态更新为 `In Progress`",
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
		".codex/skills/pr/SKILL.md",
		".codex/skills/pr/scripts/pr_merge_flow.sh",
		"Merging 快路径",
		"PR script 和远端 checks 是 `Merging` 阶段的质量门槛",
		"使用 Linear MCP/app 工具，不要使用 Linear CLI",
		"不要使用 `linear` CLI 或 `linear_graphql` 作为兜底",
		"通过 Linear MCP/app issue 更新工具将状态更新为 `In Progress`",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("repo workflow still contains local merge wording %q", forbidden)
		}
	}
}
