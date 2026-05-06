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

func TestRepoWorkflowUsesAIReviewPRSkillFastPath(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"active_states:\n    - Todo\n    - In Progress\n    - AI Review\n    - Merging\n    - Rework",
		"本 workflow 使用 `AI Review` 作为示例 workflow 里 `Human Review` 的自动审核位置",
		"`pr`：当 issue 进入 `Merging` 时",
		"AI Review`，并在同一个 session 中继续审查 issue、workpad、本地 diff、commit range 和验证证据",
		"`Merging` 阶段走 `pr` skill 快路径",
		"打开并遵守 `.codex/skills/pr/SKILL.md`",
		".codex/skills/pr/scripts/pr_merge_flow.sh",
		"Merging 快路径",
		"不要在脚本前重新展开实现、review 或完整历史 workpad",
		"PR script 和远端 checks 是 `Merging` 阶段的质量门槛",
		"test -x .codex/skills/pr/scripts/pr_merge_flow.sh",
		"正常路径不要 `sed`/展开读取完整脚本内容",
		"Merge: PASS",
		"agent 不直接移动 issue 到 `Done`",
		"issue worktree agent 不负责写 repo-root `main` checkout",
		"repo-root `main` checkout sync 由 orchestrator/operator 在 repo-root context 收尾",
		"不要从 issue worktree agent 执行 `git -C /Users/bytedance/symphony-go pull --ff-only origin main`",
		"`origin/main` 已 fetch 到该 merge commit",
		"每一条 actionable reviewer comment",
		"不要在移动到 `AI Review` 前提前创建 PR 或重复 sweep",
		"使用 `commit` skill 提交当前 issue worktree 分支",
		"当 issue 处于 `AI Review`，同一个 issue agent 审查 issue、workpad、本地 diff",
		"不要直接调用 `gh pr merge`",
		"本地 diff、commit range 和验证证据足以支持 AI Review",
		"PR metadata 必须完整，包括 `symphony` label",
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
		"`land`：当 issue 进入 `Merging` 时",
		"打开并遵守 `.codex/skills/land/SKILL.md`",
		"只允许执行 `land` skill",
		"完成 acceptance、validation、workpad、PR 创建/更新和 PR checks 后，移动到 `AI Review`",
		"PR checks 绿色，branch 已 push，PR 已链接到 issue",
		"repo-root sync 完成",
		"由 PR skill 统一负责 push、PR 创建/更新、feedback sweep、checks、squash merge 和 repo-root sync",
		"merge 完成后，更新 workpad merge evidence，并移动 issue 到 `Done`",
		"才移动 issue 到 `Done`",
		"使用 Linear MCP/app 工具，不要使用 Linear CLI",
		"不要使用 `linear` CLI 或 `linear_graphql` 作为兜底",
		"通过 Linear MCP/app issue 更新工具将状态更新为 `In Progress`",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("repo workflow still contains local merge wording %q", forbidden)
		}
	}
}

func TestRepoSkillsDocumentFastPullAndMergePassContract(t *testing.T) {
	pullRaw, err := os.ReadFile(filepath.Join("..", "..", "..", ".codex", "skills", "pull", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	pullText := string(pullRaw)
	for _, want := range []string{
		`test "$(git config --get rerere.enabled)" = true || git config rerere.enabled true`,
		`git show-ref --verify --quiet "refs/remotes/origin/$branch"`,
		`remote branch origin/$branch not found; skip feature-branch pull`,
	} {
		if !strings.Contains(pullText, want) {
			t.Fatalf("pull skill missing %q", want)
		}
	}
	if strings.Contains(pullText, "`git pull --ff-only origin $(git branch --show-current)`") {
		t.Fatal("pull skill still documents unconditional feature branch pull")
	}

	prRaw, err := os.ReadFile(filepath.Join("..", "..", "..", ".codex", "skills", "pr", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	prText := string(prRaw)
	for _, want := range []string{
		"`Merge: PASS`",
		"Do not `sed` or otherwise expand the",
		"finish with a `Merge: PASS` message instead of moving the",
		"Final response must start with `Merge: PASS`",
	} {
		if !strings.Contains(prText, want) {
			t.Fatalf("pr skill missing %q", want)
		}
	}

	runRaw, err := os.ReadFile(filepath.Join("..", "..", "..", ".codex", "skills", "symphony-issue-run", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	runText := string(runRaw)
	for _, want := range []string{
		"./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue \"$ISSUE\" --merge-target main",
		"PR creation waits for `Merging`",
		"Agent reports `Merge: PASS`; orchestrator then moves the issue to `Done`",
	} {
		if !strings.Contains(runText, want) {
			t.Fatalf("symphony issue run skill missing %q", want)
		}
	}
	for _, forbidden := range []string{
		".codex/skills/land/SKILL.md",
		"`AI Review` 前必须创建/更新 PR",
		"Use `run-once` only for diagnosis",
	} {
		if strings.Contains(runText, forbidden) {
			t.Fatalf("symphony issue run skill still contains %q", forbidden)
		}
	}
}
