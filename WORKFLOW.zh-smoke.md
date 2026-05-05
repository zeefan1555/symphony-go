---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Todo
    - In Progress
    - AI Review
    - Merging
    - Rework
    - Human Review
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 5000
workspace:
  root: .worktrees
hooks:
  after_create: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_after_create.sh" "$workspace"
  before_remove: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_before_remove.sh" "$workspace"
merge:
  target: main
agent:
  max_concurrent_agents: 1
  max_turns: 12
  review_policy:
    mode: auto
    allow_manual_ai_review: false
    on_ai_fail: rework
    expected_changed_files:
      - docs/smoke/pr-merge-fast-path-smoke.md
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=high app-server
  read_timeout_ms: 60000
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    networkAccess: true
---

你正在处理 Linear ticket `{{ issue.identifier }}`。

这是 Symphony Go 的中文冒烟 workflow。目标不是实现新功能，而是用一个极小、可重复的文档变更验证完整自动流程：

`Todo -> In Progress -> AI Review -> Merging -> Done -> 自动清理 issue worktree`

## 固定任务

只允许修改 `docs/smoke/pr-merge-fast-path-smoke.md`。

在文件末尾追加一组新的冒烟记录，格式保持为三行 bullet：

```markdown
- Timestamp: <当前本地时间，使用 ISO/RFC3339 风格>
- Issue: {{ issue.identifier }}
- Note: PR merge fast path smoke; the change was made in the issue worktree and merged through the PR flow.
```

不要修改 `SMOKE.md`，不要修改业务代码、IDL、脚本、测试或其他文档。

## Worktree 规则

- 实现、验证、commit、AI Review 和 Merging 准备都必须在当前 issue worktree 中完成。
- 不要在 root checkout 里直接编辑 `docs/smoke/pr-merge-fast-path-smoke.md`。
- issue worktree 分支必须创建本地 commit，再进入 `AI Review`。
- `Merging` 阶段使用 `.codex/skills/pr/SKILL.md` 的 PR merge flow；PR 合并完成后再把 issue 移动到 `Done`。
- `Done` 后由 orchestrator 自动清理 `.worktrees/{{ issue.identifier }}`，不要手动删除 worktree。

## 状态路由

- `Todo`：立即移动到 `In Progress`，创建或复用唯一 `## Codex Workpad` comment。
- `In Progress`：执行固定任务、验证并 commit。
- `AI Review`：在同一个 issue session 中审查 diff、commit 和验证证据。通过时给出 `Review: pass` 结论，让 orchestrator 继续到 `Merging`；失败时移动到 `Rework` 并写清发现。
- `Rework`：只修复 review 发现的问题，重新验证并回到 `AI Review`。
- `Merging`：执行 PR merge flow，确认 PR merged、root checkout sync 成功后移动到 `Done`。
- `Human Review`：只用于真实外部 blocker，例如缺少 auth、权限、secret 或必要工具。默认成功路径不能停在 `Human Review`。
- `Done`：终态，不再继续处理。

## 必选验证

完成固定任务后必须运行：

```bash
rg -n "Issue: {{ issue.identifier }}" docs/smoke/pr-merge-fast-path-smoke.md
git diff --check
git status --short --branch
```

创建 commit 后记录：

- issue worktree 分支名
- commit short SHA
- changed files，必须只有 `docs/smoke/pr-merge-fast-path-smoke.md`
- 上述验证命令和结果

## Workpad 要求

所有对 Linear 可见的 workpad、状态说明、review 结论、PR 标题/正文和最终回复都使用中文。命令、路径、错误原文和代码标识符可以保留英文。

`## Codex Workpad` 至少包含：

- 当前环境戳：`<host>:<abs-worktree>@<short-sha>`
- 计划 checklist
- Acceptance Criteria
- Validation
- Notes
- PR URL 和 merge 结果
- worktree 自动清理前的最终状态说明

## 完成标准

- `docs/smoke/pr-merge-fast-path-smoke.md` 追加了当前 issue 的新时间戳记录。
- `changed_files` 只有 `docs/smoke/pr-merge-fast-path-smoke.md`。
- `rg -n "Issue: {{ issue.identifier }}" docs/smoke/pr-merge-fast-path-smoke.md` 通过。
- `git diff --check` 通过。
- issue worktree 分支有本地 commit。
- AI Review 在同一个 issue session 中通过。
- Merging 创建或更新 PR，并完成 merge。
- issue 最终状态是 `Done`，不是 `Human Review`。
