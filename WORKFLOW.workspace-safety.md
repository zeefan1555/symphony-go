---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Todo
    - In Progress
    - Human Review
    - Merging
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 2000
workspace:
  root: .worktrees-safety
hooks:
  after_create: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_after_create.sh" "$workspace"
    cd "$workspace"
    mkdir -p .symphony-safety
    pwd -P > .symphony-safety/after_create.cwd
  before_run: |
    mkdir -p .symphony-safety
    pwd -P > .symphony-safety/before_run.cwd
  after_run: |
    mkdir -p .symphony-safety
    pwd -P > .symphony-safety/after_run.cwd
  before_remove: |
    workspace="$(pwd -P)"
    go_root="$(cd "$workspace/../.." && pwd -P)"
    "$go_root/scripts/symphony_before_remove.sh" "$workspace"
agent:
  max_concurrent_agents: 1
  max_turns: 1
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=low app-server
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    writableRoots:
      - /Users/bytedance/symphony/go/.worktrees-safety
      - /Users/bytedance/symphony/.git
    readOnlyAccess:
      type: fullAccess
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

处理 issue `{{ issue.identifier }}`。这是 workspace safety 的真实场景验证，只做下面几步。

1. 运行 `pwd -P`，记为当前 workspace 路径。
2. 读取 `.symphony-safety/after_create.cwd` 和 `.symphony-safety/before_run.cwd`。
3. 验证这三个路径完全相同。
4. 新建或覆盖 `WORKSPACE_SAFETY_PROOF.md`，内容必须包含：
   - issue identifier
   - `pwd -P` 输出
   - `after_create.cwd` 内容
   - `before_run.cwd` 内容
   - 验证结论 `workspace safety proof: passed`
5. 运行：
   - `test "$(pwd -P)" = "$(cat .symphony-safety/after_create.cwd)"`
   - `test "$(pwd -P)" = "$(cat .symphony-safety/before_run.cwd)"`
   - `rg -n 'workspace safety proof: passed' WORKSPACE_SAFETY_PROOF.md`
   - `git diff --check`
   - `git diff --name-only`
   - `git status --short`
6. 创建本地提交：`{{ issue.identifier }}: workspace safety proof`。

约束：全程中文；不创建 PR；不 push；不调用 Linear/MCP/apps；不更新 Linear comment 或状态；不要采集指标或改写实验文档；不要修改 `WORKSPACE_SAFETY_PROOF.md` 以外的文件。提交后停止。
