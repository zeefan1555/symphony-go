---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Todo
    - In Progress
    - AI Review
    - Human Review
    - Merging
    - Rework
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 2000
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
agent:
  max_concurrent_agents: 1
  max_turns: 1
  review_policy:
    mode: auto
    on_ai_fail: rework
    expected_changed_files:
      - SMOKE.md
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=low app-server
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    writableRoots:
      - /Users/bytedance/symphony/go/.worktrees
      - /Users/bytedance/symphony/.git
    readOnlyAccess:
      type: fullAccess
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

处理 issue `{{ issue.identifier }}`。这是固定中文冒烟任务，只做下面几步。

1. 只改 worktree 根目录的 `SMOKE.md`。
2. 用 `date '+%Y-%m-%dT%H:%M:%S%z'` 取时间戳。
3. 追加一行：`中文冒烟测试时间戳：<timestamp>`。
4. 验证：
   - `rg -n '^中文冒烟测试时间戳：' SMOKE.md`
   - `git diff --check`
   - `git diff --name-only`
   - `git status --short`
5. 创建本地提交：`{{ issue.identifier }}: 中文冒烟测试时间戳`。

约束：全程中文；不创建 PR；不 push；不调用 Linear/MCP/apps；不更新 Linear comment 或状态；不要采集指标或改写实验文档；不要修改 `SMOKE.md` 以外的文件。提交后停止，后续 `AI Review`、merge、指标采集由 Go orchestrator 和外层 harness 处理。
