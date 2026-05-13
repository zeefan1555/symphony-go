package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	issuemodel "symphony-go/internal/service/issue"
)

func TestLoadAndRenderChineseWorkflow(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
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

func TestRenderSupportsLiquidControlFlowAndIssueMetadata(t *testing.T) {
	priority := 2
	createdAt := time.Date(2026, 2, 26, 18, 6, 48, 0, time.FixedZone("SGT", 8*60*60))
	updatedAt := time.Date(2026, 2, 26, 18, 7, 3, 0, time.UTC)
	issue := issuemodel.Issue{
		ID:          "issue-1",
		Identifier:  "ZEE-101",
		Title:       "Render parity",
		Description: "Render through Liquid.",
		Priority:    &priority,
		State:       "Todo",
		BranchName:  "zeefan/render-parity",
		URL:         "https://linear.app/demo/issue/ZEE-101",
		Labels:      []string{"backend", "workflow"},
		BlockedBy:   []issuemodel.BlockerRef{{ID: "blocker-1", Identifier: "ZEE-100", State: "Done"}},
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
	attempt := 3
	template := `Ticket {{ issue.identifier }} priority={{ issue.priority }} branch={{ issue.branch_name }}
{% if issue.description %}{{ issue.description }}{% else %}No description{% endif %}
{% for label in issue.labels %}[{{ label }}]{% endfor %}
{% for blocker in issue.blocked_by %}blocked_by={{ blocker.identifier }}:{{ blocker.state }}{% endfor %}
created={{ issue.created_at }} updated={{ issue.updated_at }}
{% if attempt %}attempt={{ attempt }}{% endif %}`

	rendered, err := Render(template, issue, &attempt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Ticket ZEE-101 priority=2 branch=zeefan/render-parity",
		"Render through Liquid.",
		"[backend][workflow]",
		"blocked_by=ZEE-100:Done",
		"created=2026-02-26T10:06:48Z",
		"updated=2026-02-26T18:07:03Z",
		"attempt=3",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderUsesStrictVariables(t *testing.T) {
	_, err := Render("Work on {{ missing.ticket_id }}", issuemodel.Issue{Identifier: "ZEE-1"}, nil)
	if err == nil {
		t.Fatal("expected strict variable error")
	}
	if Code(err) != ErrTemplateRender {
		t.Fatalf("code = %q, want %s", Code(err), ErrTemplateRender)
	}
}

func TestRenderSurfacesInvalidTemplateWithContext(t *testing.T) {
	_, err := Render("{% if issue.identifier %}", issuemodel.Issue{Identifier: "ZEE-1"}, nil)
	if err == nil {
		t.Fatal("expected template parse error")
	}
	if Code(err) != ErrTemplateParse || !strings.Contains(err.Error(), "{% if issue.identifier %}") {
		t.Fatalf("error = %v, want template_parse_error with template context", err)
	}
}

func TestLoadReturnsTypedErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content *string
		want    string
	}{
		{name: "missing", want: ErrMissingWorkflowFile},
		{name: "parse", content: strPtr("---\ntracker: [\n---\nprompt\n"), want: ErrWorkflowParse},
		{name: "shape", content: strPtr("---\n- tracker\n---\nprompt\n"), want: ErrWorkflowFrontMatterShape},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "WORKFLOW.md")
			if tc.content != nil {
				if err := os.WriteFile(path, []byte(*tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected typed workflow error")
			}
			if Code(err) != tc.want {
				t.Fatalf("code = %q, want %s; err=%v", Code(err), tc.want, err)
			}
		})
	}
}

func TestRenderUsesDefaultPromptForBlankTemplate(t *testing.T) {
	rendered, err := Render(" \n\t", issuemodel.Issue{
		Identifier: "ZEE-DEFAULT",
		Title:      "Default prompt",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Identifier: ZEE-DEFAULT", "Title: Default prompt", "No description provided."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("default prompt missing %q:\n%s", want, rendered)
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
  api_key: $LINEAR_API_KEY
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
  api_key: $LINEAR_API_KEY
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

func strPtr(value string) *string {
	return &value
}

func TestRepoWorkflowUsesMergingFlow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "workflows", "WORKFLOW-symphony-go.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"active_states:\n    - Todo\n    - In Progress\n    - AI Review\n    - Pushing\n    - Merging\n    - Rework",
		"workspace:\n  mode: static_cwd\n  cwd: ..",
		"只在当前 repo root 中工作。不要为 issue 创建 git worktree、scratch checkout、临时 clone 或 PR 分支。",
		"`merge.target` 是本 workflow 的目标开发分支",
		"所有实现、验证和 commit 都在配置的目标分支上完成；PR 创建、检查和合并由 `Merging` 阶段统一处理。",
		"同步目标分支时只做 fast-forward",
		"`pr`：`Merging` 阶段走 PR skill 快路径，创建或更新 PR、等待检查并完成 merge。",
		"只有进入 `Merging` 阶段后才打开并遵守 `.codex/skills/pr/SKILL.md`",
		"默认路径是 `Todo -> In Progress -> AI Review -> Merging -> Done`。",
		"AI Review`，然后结束当前 turn，等待框架下发 `AI Review` continuation prompt",
		"review 通过后，框架将 issue 推进到 `Merging`",
		"`Merging`：AI Review 已通过；同一个 issue agent 走 PR skill 快路径、写 merge evidence，并以 `Merge: PASS` 结束",
		"`Pushing`：保留给不走 PR 的直接推送收口场景；不是默认路径。",
		"一个独立逻辑改动对应一个 commit；多类改动要拆成多个清晰 commit。",
		"## Step 3：AI Review 与 Merging handling",
		"如果 review 通过，最终回复以 `Review: PASS` 开头；框架会把 issue 推进到 `Merging`，并下发下一轮 `Merging` continuation prompt",
		"移动到 `AI Review` 的同一个实现 turn 不要输出 `Review: PASS` 或 `Merge: PASS`",
		"使用 PR skill 快路径：确认 `pr_merge_flow.sh` 可执行，准备 PR title/body，运行脚本，等待 checks，并完成 squash merge / repo-root sync。",
		"最终回复以 `Merge: PASS` 开头，包含 PR URL、merge commit、root status 和验证摘要；框架会把 issue 移动到 `Done`。",
		"## 移动到 Done 前的完成门槛",
		"issue 已处于 `Merging`。",
		"后续 continuation prompt 只用于阶段续航",
		"正常简单任务不要为每个小命令更新 workpad",
		"使用 `commit` skill 提交到当前目标分支",
		"只有框架下发 `AI Review` continuation prompt 后，当前 issue agent 才执行 AI Review",
		"本地 diff、commit range 和验证证据足以支持 AI Review",
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
		"默认路径是 `Todo -> In Progress -> AI Review -> Pushing -> Done`。",
		"review 通过后，不进入 PR 或 Merging 流程",
		"`Pushing`：AI Review 已通过；同一个 issue agent",
		"## Step 3：AI Review 与 Pushing handling",
		"issue 已处于 `Pushing`。",
		"PR metadata 必须完整，包括 `symphony` label",
		"每一条 actionable reviewer comment",
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
		"do not use `status` as a shell variable name",
		"`script_status` or `pr_status`",
	} {
		if !strings.Contains(prText, want) {
			t.Fatalf("pr skill missing %q", want)
		}
	}
	if strings.Contains(prText, "status=$?") {
		t.Fatal("pr skill still demonstrates assigning to zsh readonly status variable")
	}

	runRaw, err := os.ReadFile(filepath.Join("..", "..", "..", ".codex", "skills", "symphony-issue-run", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	runText := string(runRaw)
	for _, want := range []string{
		"./bin/symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md --once --no-tui --issue \"$ISSUE\" --merge-target main",
		"`Todo -> In Progress -> AI Review -> Merging -> Done`",
		"Merging uses the PR skill fast path, records merge evidence, and reports `Merge: PASS`",
		"`Pushing` is only for explicitly direct-push issues",
		"Issue work happens in the repo root on the configured target branch",
	} {
		if !strings.Contains(runText, want) {
			t.Fatalf("symphony issue run skill missing %q", want)
		}
	}
	for _, forbidden := range []string{
		".codex/skills/land/SKILL.md",
		"`AI Review` 前必须创建/更新 PR",
		"Use `run-once` only for diagnosis",
		"`Todo -> In Progress -> AI Review -> Pushing -> Done`",
		"Pushing pushes the target branch",
	} {
		if strings.Contains(runText, forbidden) {
			t.Fatalf("symphony issue run skill still contains %q", forbidden)
		}
	}
}
