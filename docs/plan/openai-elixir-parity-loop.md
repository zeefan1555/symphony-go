# OpenAI Elixir parity loop

本文件记录当前 Go 仓对 `ref/elixir-symphony-example` 的差距吸收结果。原则是以 `SPEC.md` 为主合同，OpenAI Elixir 实现作为行为参考；只吸收能改善当前 Go runtime 且影响面可验证的能力。

## 已吸收

### Liquid prompt rendering

- 参考实现：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/prompt_builder.ex`
- Go 落点：`internal/service/workflow/workflow.go`
- 行为变化：
  - 使用真实 Liquid engine 渲染 workflow prompt，不再用手写正则替换少数字段。
  - 开启 strict variables，未知变量直接返回渲染错误。
  - 暴露 `issue.priority`、`issue.branch_name`、`issue.labels`、`issue.blocked_by`、`issue.created_at`、`issue.updated_at`。
  - 空 prompt 使用默认 Linear issue prompt。
- 验证：`internal/service/workflow/workflow_test.go`

### Dispatch stale revalidation

- 参考实现：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/orchestrator.ex`
- Go 落点：`internal/service/orchestrator/orchestrator.go`
- 行为变化：
  - poll 选出候选 issue 后，启动 runner 前再次读取 issue state。
  - 如果 issue 已消失、进入 terminal/inactive state，或 Todo 新增非终态 blocker，则跳过 dispatch。
  - Linear `FetchIssueStatesByIDs` 查询同步带回 blocker relation，避免 stale blocker 漏判。
- 验证：
  - `internal/service/orchestrator/orchestrator_test.go`
  - `internal/integration/linear/client_test.go`

## 已覆盖无需改动

### Codex turn result classification

- 参考实现：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/codex/app_server.ex`
- Go 落点：`internal/service/codex/runner.go`
- 当前行为：
  - `turn/completed` 作为成功 turn 返回。
  - `turn/failed` 和 `turn/cancelled` 返回包含原始 method 的错误，同时保留已启动 turn 的 session/thread/turn identity。
  - `turn/input_required`、approval request 和 MCP elicitation 在无人值守模式下 fail fast。
- 验证：`internal/service/codex/runner_test.go`

### Runtime token and rate-limit observability

- 参考实现：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/status_dashboard.ex`
- Go 落点：
  - `internal/runtime/observability/token.go`
  - `internal/runtime/observability/ratelimit.go`
  - `internal/tui/dashboard.go`
- 当前行为：Go runtime 已将 Codex token event、rate limit payload 和 TUI 展示拆入 observability 包和 TUI renderer，不需要直接移植 Elixir terminal renderer。
- 验证：
  - `internal/runtime/observability/token_test.go`
  - `internal/runtime/observability/ratelimit_test.go`
  - `internal/tui/dashboard_test.go`

## 暂不吸收

### Phoenix dashboard

Elixir 示例包含 Phoenix LiveView dashboard。当前 Go 仓已有 TUI、Hertz control surface 和 IDL 计划，`SPEC.md` 也把 rich web UI 定义为 non-goal，因此不直接移植 Phoenix 形态。后续如要增强 operator surface，应沿 `internal/transport/hertzserver` 和 `idl/` 继续，而不是引入新的 Web stack。

### SSH worker pool

Elixir 示例包含远程 worker host 选择和 SSH 启动能力。当前 Go 仓的核心目标仍是本地 issue worktree runner；远程 worker 会改变 workspace、sandbox、日志和调度容量语义，不能混入本轮小步 parity。若要做，应单独写 PRD，先定义 worker capacity、workspace root、Git metadata writable roots 和日志归属。

### Elixir Mix tasks

`mix specs.check`、`mix pr_body.check` 和 `mix workspace.before_remove` 是 Elixir 项目的工具入口。Go 仓已经有 `./test.sh`、`./build.sh`、`scripts/symphony_before_remove.sh` 和 repo-root skills，不直接迁移 Mix task 形态。

### Elixir-only config schema

Elixir 的 schema 同时服务 Phoenix、SSH worker 和 Mix task。Go 仓只补 `SPEC.md` 和当前 `WORKFLOW.md` 使用到的 typed config；未被 Go runtime 使用的 Elixir-only 字段不移植，避免制造未生效配置。
