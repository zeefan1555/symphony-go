# Symphony Issue Run Optimization Log

本文件记录 `symphony-issue-run` 流程每次保留下来的优化点。每条记录必须能回答：
这次卡在哪里、证据是什么、改了 Skill / Workflow / 代码的哪一层、以及怎么验证。

## 2026-05-06 18:57 +08 - ZEE-103 follow-up

- Trigger: ZEE-103 真实 issue smoke 虽已从 ZEE-102 的约 11 分 34 秒降到约 5 分 39 秒，但简单任务仍明显慢；用户要求按卡点继续优化。
- Evidence:
  - `.symphony/logs/run-20260506-183535.human.log` 显示 `Merging` 到 PR script start 仍有约 38 秒，主要用于读取/展开 `pr` skill 和脚本、重复检查，而脚本本身约 25 秒。
  - 同一轮中 `git pull --ff-only origin symphony-go/ZEE-103` 因远端 issue branch 尚不存在失败，浪费约 3.2 秒，并制造无效解释。
  - ZEE-103 issue 已进入 `Done` 且 PR 已 merge，但唯一 workpad 没写入最终 merge evidence，说明 agent 直接 Done 与 worktree cleanup 存在收尾竞态。
  - 这轮 `run` 未带 `--once`，issue terminal 后仍持续轮询，监督会话需要手动 kill 进程。
- Optimization:
  - Skill 层：`pull` 只在 `origin/<issue-branch>` 已存在时同步远端 feature branch，且只在 rerere 配置缺失时写 git config。
  - Workflow/Skill 层：`Merging` 快路径只要求打开 `pr` skill、`test -x` 脚本、准备 PR title/body 并立即执行脚本；正常路径禁止展开完整脚本、重复 review 或重复读取 workflow 历史。
  - 代码层：新增 `Merge: PASS` agent/orchestrator 文本契约。agent 在 `Merging` 脚本成功后先更新 workpad，再用 `Merge: PASS` 最终回复；orchestrator 识别后负责把 issue 移到 `Done` 并触发 terminal cleanup。
  - Skill 层：`symphony-issue-run` 的单 issue smoke 默认命令改为 `./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue <ISSUE> --merge-target main`。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/pr/SKILL.md`
  - `.codex/skills/pull/SKILL.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `internal/service/orchestrator/agent_session.go`
  - `internal/service/orchestrator/orchestrator_test.go`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`./test.sh ./internal/service/orchestrator ./internal/service/workflow`
  - 通过：`./build.sh`
  - 通过：`bash -n .codex/skills/pr/scripts/pr_merge_flow.sh`
  - 通过：`git diff --check`
- Follow-up: 变更验证通过后再新建真实 smoke issue，观察总耗时是否低于 5 分钟、`Merging` 到 script start 是否低于 15 秒、workpad merge evidence 是否完整、`--once` 是否自动退出。

## 2026-05-06 18:25 +08 - ZEE-102 follow-up

- Trigger: 用户指出简单 smoke 不应该跑到 10 分钟，并提醒此前已经写过 `pr` skill，`AI Review` 完成后应该直接使用该 skill。
- Evidence:
  - `.symphony/logs/run-20260506-180424.human.log` 显示 ZEE-102 从 `18:04:24` 到 `18:15:58`，总耗时约 11 分 34 秒。
  - 同一日志中 82 个可见 shell command 累计耗时约 69.6 秒，worktree hook 约 1.17 秒，主要耗时来自 agent 在命令之间的上下文读取、workpad 更新、PR 前置 sweep、自审和状态流转。
  - `Merging` 里执行 `.codex/skills/land/scripts/land_pr_flow.sh`，但用户期望的是直接走 `.codex/skills/pr/SKILL.md` 的 PR merge flow。
  - ZEE-102 还证明 root checkout sync 不应由 issue worktree agent 承担：PR 已 merge 且 `origin/main` 可见后，child 仍可能因 repo-root `main` 文件写入权限卡在 root pull/unlink。
- Optimization:
  - Workflow 层：实现阶段收敛到本地 validation + commit + workpad handoff；不再要求移动到 `AI Review` 前提前创建 PR、跑完整 PR feedback sweep 或等待 remote checks。
  - Workflow 层：`AI Review` 只审本地 diff、commit range、workpad 和验证证据。
  - Workflow 层：review 通过后进入 `Merging`，直接打开 `.codex/skills/pr/SKILL.md`，优先运行 `.codex/skills/pr/scripts/pr_merge_flow.sh`，由 PR skill 统一负责 push、PR 创建/更新、feedback sweep、checks 和 squash merge。
  - Skill/代码层：`pr_merge_flow.sh` 不再 switch/pull/restore repo-root `main` checkout；Merging turn 不再把 repo-root main checkout 加入 writable roots。root sync 由 orchestrator/operator 在 repo-root context 中处理。
  - 测试层：更新 repo workflow contract，禁止 `land` skill 文案回归，并锁住 `Merging 快路径`。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/pr/SKILL.md`
  - `.codex/skills/pr/scripts/pr_merge_flow.sh`
  - `internal/service/codex/runner.go`
  - `internal/service/codex/runner_test.go`
  - `internal/service/workflow/workflow_test.go`
  - `lesson.md`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`./test.sh ./internal/service/workflow ./internal/service/codex`
  - 通过：`./build.sh`
  - 通过：`bash -n .codex/skills/pr/scripts/pr_merge_flow.sh`
  - 通过：`git diff --check`

## 2026-05-06 16:53 +08 - ZEE-97

- Trigger: 用户要求跑一个简单中文冒烟任务，观察从创建 issue 到全自动处理完成的大致耗时和瓶颈。
- Evidence:
  - Linear issue: `ZEE-97`，创建于 `2026-05-06 16:50:35 +0800`，初始状态 `Todo`，后因外部 auth blocker 被外层移动到 `Human Review`。
  - listener: PID `46701`，启动于 `2026-05-06 16:51:09 +0800`，daemon log `.symphony/logs/ZEE-97-20260506-165109.out`。
  - human log: `.symphony/logs/run-20260506-165110.human.log`，首行在 `16:51:11` 记录 `startup_cleanup_fetch_failed`，随后每 5 秒记录一次 `poll_error`。
  - 错误原文为 `Linear GraphQL status 401` / `Authentication required, not authenticated`。
  - 外层用同一个 `LINEAR_API_KEY` 调 `query { viewer { id name } }` 复现 `http=401`。
  - `.worktrees/ZEE-97` 未创建，说明瓶颈发生在 listener 读取 active issue 之前，尚未进入 child agent、实现、AI Review 或 Merging。
- Optimization:
  - 本轮不修改代码；将环境 blocker 记录为 follow-up issue `ZEE-98`。
  - 决策：框架应在 startup cleanup / poll loop 前做一次 Linear auth health check。无效 token 应输出单次清晰 blocker，而不是进入重复 `poll_error` 循环。
- Files:
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`make build`
  - 通过：`curl` 使用当前 `LINEAR_API_KEY` 复现 `http=401`
  - 通过：`git worktree list --porcelain | rg 'ZEE-97|\.worktrees/ZEE-97'` 无输出
  - 通过：`git diff --check`
- Follow-up: `ZEE-98` 优化 listener 启动前 Linear auth 预检与 401 blocker 输出。

## 2026-05-06 15:56 +08 - ZEE-96

- Trigger: 用户要求跑一轮真实 smoke，验证 child session 现在能否使用 `linear_graphql` dynamic tool。
- Evidence:
  - Linear issue: `ZEE-96`，创建于 `Todo`，listener 记录 `Todo -> In Progress`，并创建 `.worktrees/ZEE-96` / branch `symphony-go/ZEE-96`。
  - daemon log: `.symphony/logs/ZEE-96-20260506-155356.out`。
  - human log: `.symphony/logs/run-20260506-155357.human.log`。
  - JSONL log: `.symphony/logs/run-20260506-155357.jsonl`。
  - child 通过 `linear_graphql` query 读取当前 issue、comments 和 team states，并拿到 `Done` stateId。
  - JSONL 记录 `item/tool/call`，`tool` 为 `linear_graphql`，其中 `commentCreate` 成功创建 Workpad comment `92ed404b-10c5-45e8-8b8b-335a1a802e85`，正文包含 `linear_graphql probe success`。
  - JSONL 记录第二次 `item/tool/call`，`tool` 为 `linear_graphql`，其中 `issueUpdate` 成功把 `ZEE-96` 移到 `Done`。
  - Linear 复读确认 `ZEE-96` 当前状态为 `Done`；worktree cleanup 已完成，`.worktrees/ZEE-96` 不再存在。
- Optimization:
  - 本轮证明 Go runner 的 child dynamic tool 注入已生效：child 不再卡在缺少 `linear_graphql`，并能用同一工具完成 Linear query、comment mutation 和 issue state mutation。
  - 本轮是诊断型 smoke，刻意不修改仓库文件、不创建 commit/PR、不进入 AI Review/Merging；目的只验证 child `linear_graphql` 能力。
- Files:
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`make build`
  - 通过：ZEE-96 listener smoke 到 `Done`
- Follow-up: 后续完整 workflow smoke 应再验证常规实现任务能在 `linear_graphql` 可用的前提下完成 PR handoff、`AI Review` 和 `Merging` land。

## 2026-05-06 15:48 +08 - ZEE-95

- Trigger: 用户要求参考 `ref/elixir-symphony-example` 的代码，确认它如何让 child session 使用 GraphQL，并把 Go runner 改到同样可用。
- Evidence:
  - Elixir 示例在 `ref/elixir-symphony-example/elixir/lib/symphony_elixir/codex/dynamic_tool.ex` 中定义 `linear_graphql` tool spec、参数归一化和 GraphQL 执行。
  - Elixir 示例在 `ref/elixir-symphony-example/elixir/lib/symphony_elixir/codex/app_server.ex` 的 `thread/start` 里传入 `DynamicTool.tool_specs()`，并处理 `item/tool/call` 后返回 tool result。
  - Go runner 原先在 `internal/service/codex/runner.go` 的 `thread/start` 中固定发送空 `dynamicTools`，所以 ZEE-94 child 只能看到缺失的 `linear_graphql`。
- Optimization:
  - 代码层：新增 Go 版 dynamic tool executor，广告 `linear_graphql`，支持 raw query string 或 `{query, variables}` 输入，并把 GraphQL top-level `errors` 作为 `success=false` 的 tool output 保留给模型。
  - Runner 层：`thread/start` 使用 executor 的 tool specs；`awaitTurn` 处理 `item/tool/call` 并把结果回写 app-server，避免 tool call 悬挂。
  - App/reload 层：从 workflow tracker config 创建 Linear GraphQL client，并注入到 Codex runner；workflow reload 时同步重建带新 tracker config 的 runner。
  - Linear client 层：拆出 `GraphQLRaw`，让动态工具可以保留 GraphQL `errors` response，同时保持原有 typed `GraphQL` 调用对 errors 返回 error。
- Files:
  - `internal/service/codex/dynamic_tool.go`
  - `internal/service/codex/runner.go`
  - `internal/service/codex/dynamic_tool_test.go`
  - `internal/service/codex/runner_test.go`
  - `internal/integration/linear/client.go`
  - `internal/app/run.go`
  - `internal/service/orchestrator/orchestrator.go`
  - `docs/plan/go-symphony-v3-core.md`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`./test.sh ./internal/service/codex`
  - 通过：`./test.sh ./internal/service/codex ./internal/integration/linear ./internal/service/orchestrator ./internal/app`
  - 通过：`./test.sh ./internal/service/workflow`
  - 通过：`git diff --check`
  - 通过：`make build`
- Follow-up: 重建 binary 后重跑 ZEE-94/ZEE-95 同类 smoke，确认 child 日志出现 `item/tool/call` 的 `linear_graphql` 成功返回，并继续推进到 PR handoff / `AI Review`。

## 2026-05-06 15:29 +08 - ZEE-94

- Trigger: 用户要求使用 `symphony-issue-run` 跑一轮真实 Linear 冒烟，验证 Elixir-style `AI Review` + `land` workflow 在当前 Go runner 下能否全自动跑通。
- Evidence:
  - Linear issue: `ZEE-94`，创建于 `Todo`，listener 使用有效 Linear GraphQL key 后记录 `Todo -> In Progress`，并创建 `.worktrees/ZEE-94` / branch `symphony-go/ZEE-94`。
  - 首次 listener 日志 `.symphony/logs/ZEE-94-20260506-152337.out` 证明当前 shell 的 `LINEAR_API_KEY` 无效，GraphQL 返回 `401 Authentication required`。
  - 有效 key 重跑日志 `.symphony/logs/ZEE-94-20260506-152435.out`、human log `.symphony/logs/run-20260506-152435.human.log` 和 JSONL `.symphony/logs/run-20260506-152435.jsonl` 证明 listener/tracker 路径可用，child Codex session 已启动。
  - child 两轮都执行 `command -v linear_graphql || true` 且无输出，随后按 workflow 记录 blocker；它没有使用 Linear MCP/app 写入，也没有 fallback 到 `linear` CLI。
  - `.worktrees/ZEE-94` 保持 clean，`SMOKE.md` 未被修改，PR / `AI Review` / `Merging` 未开始；监督会话已把 blocker 写入 ZEE-94 Workpad，并将 issue 移到 `Human Review`。
  - 代码/文档契约存在冲突：`SPEC.md` 把 `linear_graphql` 定义为可选 client-side tool extension，`docs/plan/go-symphony-v3-core.md` 明确 v3 不包含该扩展，而当前 `WORKFLOW.md` / `.codex/skills/linear/SKILL.md` 要求 child 使用 `linear_graphql`。
- Optimization:
  - 本轮不在 smoke 中临时改 runner。根因不是 Linear tracker auth，也不是 child 违规 fallback，而是 workflow/skill 依赖的 `linear_graphql` 工具没有被 app-server session 注入。
  - 后续需要二选一：实现并广告受限的 `linear_graphql` client-side tool，或把 Workpad / issue state 写入下沉为 orchestrator-owned GraphQL 动作，并同步收紧 workflow/skills。
  - 已创建 follow-up issue `ZEE-95` 跟踪该框架缺口，避免后续 smoke 再次卡在同一位置。
- Files:
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
- Follow-up: 先解决 child Linear 写入所有权，再重跑 ZEE-94 同类 Todo issue，验收标准是无人值守完成 Workpad 更新、PR handoff、`AI Review` 和 `Merging` land，且不触发 MCP 审批、不使用 Linear CLI。

## 2026-05-06 15:19 +08 - repo-only

- Trigger: 用户要求参考 `ref/elixir-symphony-example/elixir/WORKFLOW.md` 和示例 `.codex/skills`，把根目录 workflow 按示例规范调整；随后明确示例里的 `Human Review` 位置改成 `AI Review` 即可。
- Evidence:
  - 示例 workflow 的核心路径是 PR 先创建/更新并完成 feedback sweep，review 后进入 `Merging`，`Merging` 打开 `.codex/skills/land/SKILL.md` 并循环 land，而不是直接跑本仓旧的 PR script。
  - Go orchestrator 对 `AI Review` 有一等路由：`internal/service/orchestrator/phases.go` 会在 `AI Review` 和 `Merging` 状态运行 reviewer phase；`eligibility.go` 需要 `agent.review_policy` 允许 AI Review。
- Optimization:
  - Workflow 层：移除 active states 里的 `Human Review`，保留 `AI Review`，并把示例的 PR handoff/review/land 规范迁入根目录 `WORKFLOW.md`。
  - Skill 层：同步 `linear`、`land`、`symphony-issue-run`、`tdd-acceptance-pr` 和 `prd-issue-run`，让 PR 创建发生在 AI Review 前，Merging 只执行 `land`。
  - 测试层：更新 repo workflow contract，锁住 Elixir-style AI Review + land flow，并禁止旧 `.codex/skills/pr/scripts/pr_merge_flow.sh` 文案回归。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/linear/SKILL.md`
  - `.codex/skills/land/SKILL.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `.codex/skills/tdd-acceptance-pr/SKILL.md`
  - `.codex/skills/prd-issue-run/SKILL.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：`./test.sh ./internal/service/workflow`
  - 通过：`make build`
- Follow-up: 下一次真实 issue-run 重点验证 PR 是否在 `AI Review` 前创建并链接，`Merging` 是否打开 `land` skill 而不是旧 PR script。

## 2026-05-06 15:09 +08 - repo-only

- Trigger: 用户根据 ZEE-93 MCP smoke 结果纠正方向：MCP 写入会触发审批，不能作为全自动 child workflow；应参考 `ref/elixir-symphony-example/.codex/skills/linear/SKILL.md`，回到本仓 listener 使用的 Linear GraphQL 路径。
- Evidence:
  - ZEE-93 retry 证明 listener 使用有效 `LINEAR_API_KEY` 后能完成 `Todo -> In Progress`、创建 `.worktrees/ZEE-93` 并启动 child Codex。
  - 同一轮 child 使用 Linear MCP/app 读取可行，但 `save_comment` 写入触发 `mcpServer/elicitation/request`，runner 报错 `codex requested interactive MCP approval; unattended runs must not use MCP write tools`。
  - 参考 skill `ref/elixir-symphony-example/.codex/skills/linear/SKILL.md` 的主路径是 `linear_graphql`，包括 issue 查询、team states、`issueUpdate`、`commentCreate` 和 `commentUpdate`。
  - 代码层 `internal/integration/linear/client.go` 的 listener/tracker 也是 Linear GraphQL HTTP client，而不是 MCP 写入。
- Optimization:
  - Workflow 层：把 Linear 前置条件改回 `linear_graphql`，明确派生会话不要使用 Linear MCP/app issue/comment 写入。
  - Skill/文档层：同步 `linear`、`linear-cli`、`symphony-issue-run`、`tdd-acceptance-pr`、`prd-issue-run` 和 `docs/agents/issue-tracker.md`，让自动化路径统一指向 GraphQL。
  - 测试层：更新 repo workflow contract，锁住 `linear_graphql`、`issueUpdate`、`commentCreate` / `commentUpdate` 约束，并防止旧 MCP smoke 文案回归。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/linear/SKILL.md`
  - `.codex/skills/linear-cli/SKILL.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `.codex/skills/tdd-acceptance-pr/SKILL.md`
  - `.codex/skills/prd-issue-run/SKILL.md`
  - `docs/agents/issue-tracker.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：`./test.sh ./internal/service/workflow`
  - 通过：`make build`
- Follow-up: 如果后续真实 child session 没有注入 `linear_graphql`，应把 Linear workpad/status 写入下沉到 orchestrator-owned GraphQL client，而不是回退 MCP 写入。

## 2026-05-06 14:55 +08 - ZEE-93

- Trigger: 启动 MCP smoke issue-scoped listener，验证服务能否监听 Linear `Todo` 工单，并观察派生 Codex 会话是否使用 Linear MCP/app 工具。
- Evidence:
  - Linear issue: `ZEE-93`，创建于 `Todo`，随后因 blocker 由监督会话移动到 `Human Review`。
  - daemon log: `.symphony/logs/ZEE-93-20260506-145248.out`。
  - listener 启动命令：`./bin/symphony-go run --workflow ./WORKFLOW.md --no-tui --issue ZEE-93 --merge-target main`。
  - daemon log 连续记录 `Linear GraphQL status 401: Authentication required, not authenticated`，发生在 `startup_cleanup_fetch_failed` 和 `poll_error`，因此没有创建 `.worktrees/ZEE-93`，也没有启动 child Codex session。
  - 当前 shell 的 `LINEAR_API_KEY` 存在但直接 GraphQL `viewer` 查询返回 `HTTP Error 401: Unauthorized`；`linear auth whoami` 同样返回 `You need to authenticate to access this operation.`。
  - 监督会话已通过 Linear MCP 写入 ZEE-93 的 `## Codex Workpad` comment，说明 MCP connector 本身可写，但 Go listener 的 tracker auth 仍依赖 `WORKFLOW.md` 中的 `tracker.api_key: $LINEAR_API_KEY`。
- Optimization:
  - 本轮不改代码；先把结论记录为环境/auth blocker。派生会话是否能使用 Linear MCP/app 工具尚未被验证，因为 listener 在 poll 阶段已失败。
  - 后续要么注入有效 `LINEAR_API_KEY` 后重跑 ZEE-93，要么把 Go listener 的 Linear tracker auth 设计为可复用 MCP/app 授权，但后者是代码级新需求，不能用 workflow 文案伪装完成。
- Files:
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：`./test.sh ./internal/service/workflow`
  - 通过：`make build`
- Follow-up: 补有效 Linear API key 后，把 ZEE-93 从 `Human Review` 移回 `Todo` 或新建一张 Todo issue，再启动 listener 观察 child session tool calls。

## 2026-05-06 15:02 +08 - ZEE-93 retry with valid key

- Trigger: 用户提供新的 Linear API key，要求继续验证同一条 MCP smoke。
- Evidence:
  - 新 key 通过 GraphQL `viewer` 查询，返回当前用户 `zee fan`。
  - ZEE-93 从 `Human Review` 移回 `Todo` 后，listener 使用同一个临时环境 key 启动：`.symphony/logs/ZEE-93-20260506-145834.out`。
  - human log `.symphony/logs/run-20260506-145834.human.log` 记录 `Todo -> In Progress`、`.worktrees/ZEE-93` 创建、child Codex session 启动。
  - 第一轮 child 因读取 memory 输出过长触发 `bufio.Scanner: token too long`，orchestrator 随后自动启动第二轮 child。
  - 第二轮 child 未退回 `linear` CLI 或 `linear_graphql`，并通过 Linear MCP/app 完成 `get_issue`、`list_comments`、team states 读取；JSONL 里对应工具事件是 `mcpToolCall`。
  - child 尝试用 Linear MCP `save_comment` 更新 Workpad 时触发 `mcpServer/elicitation/request`，runner 报错 `codex requested interactive MCP approval; unattended runs must not use MCP write tools`。
- Optimization:
  - 本轮结论是框架差距而不是 auth 问题：Go listener tracker auth 可通过有效 API key 解决；child session 的 Linear MCP 读可用；Linear MCP 写在无人值守 run 中会触发交互审批并失败。
  - 当前暂不改代码。后续若要让 MCP 写成为正常路径，需要在 Codex app-server / runner 层提供明确的 MCP approval policy 或 orchestrator-owned Linear 写入，不应只靠 workflow 文案要求 child 写 MCP。
- Files:
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：ZEE-93 listener retry 到 child MCP read；blocker 证据写入 ZEE-93 Workpad。
- Follow-up: 设计一条最小代码改动：要么支持 unattended MCP write auto-approval allowlist，要么恢复 child read-only MCP、由 orchestrator 负责 Linear writes。

## 2026-05-06 14:51 +08 - repo-only

- Trigger: 用户希望把 Linear 读写从 CLI/`linear_graphql` 切到 MCP，启动服务后通过真实 Linear issue smoke 验证派生 Codex 会话是否会使用 Linear MCP/app 工具。
- Evidence:
  - `WORKFLOW.md` 原文要求优先 `linear_graphql`，再 fallback 到 `.codex/skills/linear-cli/SKILL.md`，并明确不调用 Linear MCP/app 工具。
  - `.codex/skills/symphony-issue-run/SKILL.md` 和相关 Linear skill 仍把 child agent 约束到 CLI/GraphQL 路径，和本轮 MCP smoke 目标冲突。
- Optimization:
  - Workflow 层：把 Linear 前置条件改成“使用 Linear MCP/app 工具，不要使用 Linear CLI”，并禁止 child agent fallback 到 `linear` CLI 或 `linear_graphql`。
  - Skill/文档层：同步 `symphony-issue-run`、`linear-cli`、`linear`、`tdd-acceptance-pr`、`prd-issue-run` 和 `docs/agents/issue-tracker.md`，避免 agent 读到旧规则后绕回 CLI/GraphQL。
  - 测试层：扩展 repo workflow contract，锁住 MCP smoke 约束并防止旧的“不要使用 Linear MCP/app 工具”文案回归。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `.codex/skills/linear-cli/SKILL.md`
  - `.codex/skills/linear/SKILL.md`
  - `.codex/skills/tdd-acceptance-pr/SKILL.md`
  - `.codex/skills/prd-issue-run/SKILL.md`
  - `docs/agents/issue-tracker.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：`./test.sh ./internal/service/workflow`
  - 通过：`make build`
- Follow-up: 创建一个 Todo smoke issue，启动 issue-scoped listener，检查 child session 日志中是否出现 Linear MCP/app 调用、MCP approval blocker，或错误 fallback 到 CLI/GraphQL。

## 2026-05-04 20:33 +08 - ZEE-75

- Trigger: 跑优化后的 todo 冒烟时，用户希望确认下次执行是否还有卡点。
- Evidence:
  - `.symphony/logs/run-20260504-201403.human.log` 显示 `Merging` session 在 `20:23:56` 启动，但直到 `20:27:33` 才开始执行 `.codex/skills/pr/scripts/pr_merge_flow.sh`，脚本前约 3 分半仍花在技能读取、Linear CLI auth/status/comment 查询和模型计划上。
  - 同一日志显示 PR script 从 `20:27:33` 到 `20:28:23` 左右完成 PR 创建、merge 和 worktree cleanup，脚本本体耗时约 50 秒。
  - `linear issue view ZEE-75 --json` 显示 issue 已 `Done` 且 PR #33 merged，但 root checkout 仍 `behind 1`；监督会话随后执行 `git pull --ff-only origin main` 才从 `2f5c6a5` fast-forward 到 `a18f37e`。
  - Workpad 缺少 Merging 最终证据；监督会话已补写同一个 comment `56849a19-e7ea-4c4a-8779-876113b76eaf`。
- Optimization:
  - Workflow 层：收紧 `Merging 快路径`，明确 listener 已经按状态路由，脚本前不要再执行 `linear auth whoami`、不要读取 `.codex/skills/linear*.md`、不要读取完整历史 workpad。
  - Workflow 层：要求 Linear comment / state 更新集中放在 PR script 成功或失败之后。
  - Workflow 层：如果 PR script 成功但 root `main` 未同步到 `origin/main`，立即执行 `git pull --ff-only origin main` 作为 Merging 收尾，并写入 workpad。
  - 测试层：扩展 repo workflow contract，防止后续删除这些 Merging 快路径约束。
- Files:
  - `WORKFLOW.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/service/workflow`
  - 通过：`GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go make build`
- Follow-up: 下一轮真实 issue-run 应重点看 `Merging` session start 到 PR script start 是否降到 90 秒内，以及 root main 是否自动同步。

## 2026-05-04 20:12 +08 - ZEE-74 follow-up

- Trigger: 用户反馈 PR flow 冒烟整体偏慢，希望优化流程后再跑一个 todo 冒烟确认下次是否还有卡点。
- Evidence:
  - `.symphony/logs/run-20260504-194540.jsonl` 显示 `Merging` 阶段真正的 PR script 在 `19:48:24.877` 左右启动，并在约 25 秒内输出 PR URL；主要耗时发生在脚本前的上下文读取、issue/comment 检查和 workpad 更新。
  - `.symphony/logs/ZEE-74-20260504-194540.out` / human log 显示 PR flow 本身可完成，但 `Merging` 语义仍允许 agent 重新展开较多 workflow 和历史 workpad 内容。
- Optimization:
  - Workflow 层：新增 `Merging 快路径`，明确 `Merging` 已经过 AI Review，不重新执行实现或审查流程；只读取当前 issue、唯一 workpad、git status/HEAD 和 PR skill 必需部分。
  - Workflow 层：要求先运行 `.codex/skills/pr/scripts/pr_merge_flow.sh`，再集中更新一次 workpad，减少脚本前多轮外部写入和长上下文消耗。
  - 测试层：扩展 repo workflow contract，锁住快路径、跳过重审、先跑 PR script 和以 PR script/checks 为质量门槛的文案。
- Files:
  - `WORKFLOW.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/service/workflow`
  - 通过：`GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go make build`
- Follow-up: 提交并推送后创建新的 Todo 冒烟 issue，观察 `Merging` 到 PR script 启动之间是否仍有明显等待。

## 2026-05-04 19:45 +08 - ZEE-74

- Trigger: 用户明确不希望通过给 reviewer 额外 repo root 写权限来解决 merge 卡点，希望把 workflow 改为 PR merge flow。
- Evidence:
  - `.symphony/logs/ZEE-74-20260504-192614.out` 记录 reviewer 成功执行 `linear issue update ZEE-74 --state Merging`，但随后在同一 reviewer turn 里尝试 root main merge，最终把 issue 退到 `Human Review`。
  - 直接扩大 `AI Review` turn writable roots 会把 workflow 语义泄漏成 hardcoded sandbox 权限，不符合用户预期。
- Optimization:
  - Workflow 层：`Merging` 阶段改为使用 `.codex/skills/pr/SKILL.md` 的 PR merge flow；从 issue worktree push branch、创建/更新 PR、等待 checks、squash merge，并由脚本同步 root `main`。
  - Skill 层：同步更新 `symphony-issue-run` 监控口径，禁止正常路径 fallback 到 root local merge。
  - 测试层：新增 repo workflow contract，确保 `WORKFLOW.md` 指向 PR skill/script，且不再包含直接 local merge 的关键命令和禁 PR 文案。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `internal/service/workflow/workflow_test.go`
  - `docs/architecture/symphony-go-architecture.md`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - `GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/service/workflow`
- Follow-up: 重建 `bin/symphony-go` 后继续跑 `ZEE-74`，验证 `Merging` 是否走 PR script 而不是 root local merge。

## 2026-05-04 19:14 +08 - ZEE-74

- Trigger: 继续 `ZEE-74` 冒烟时，用户要求验证全自动链路是否能从 `AI Review` 往后推进。
- Evidence:
  - `.symphony/logs/ZEE-74-20260504-190717.out` 记录 reviewer 子进程已能调用 `linear 2.0.0`、`linear auth whoami` 和 `linear issue view ZEE-74 --json`，说明 Linear CLI 工具链已通。
  - 同一日志 `19:12:46` 记录 `codex_final` 输出 `Review: PASS`，但 issue 仍保持 `AI Review`，随后 listener 又启动新的 reviewer session。
  - 修复后重跑 `.symphony/logs/ZEE-74-20260504-191829.out`，`19:24:02` reviewer 输出中文格式 `结论: PASS`，再次证明兜底判断必须兼容中英文结构化 PASS。
  - 问题不再是 Linear 工具不可用，而是 reviewer 已给出通过结论时，agent 未可靠执行状态推进，orchestrator 也没有兜底。
- Optimization:
  - 代码层：orchestrator 在 reviewer phase 捕获最终 `agentMessage`；当最终消息以 `Review: PASS`、`Conclusion: PASS` 或 `结论: PASS` 开头且 Linear 状态仍为 `AI Review` 时，自动执行 `AI Review -> Merging` 并在同一 session 追加 merge continuation prompt。
  - 测试层：新增回归用例覆盖 reviewer 只输出 `Review: PASS`、没有自行移动状态时，framework 仍继续进入 `Merging` 并执行后续 turn。
- Files:
  - `internal/service/orchestrator/agent_session.go`
  - `internal/service/orchestrator/orchestrator_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - `GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/service/orchestrator`
- Follow-up: 重建 `bin/symphony-go` 后继续跑 `ZEE-74`，验证 `AI Review -> Merging -> Done` 是否真的走通。

## 2026-05-02 16:36 +08 - ZEE-41

- Trigger: 用户指出 `ZEE-41` 耗时过长，状态流经过 `Human Review`，且最终由父会话手工接管 `Merging`，不符合 `symphony-go` 全自动处理目标。
- Evidence:
  - `.symphony/logs/run-20260502-155913.human.log` 多次记录 `response timeout waiting for id=2`，导致第一轮 listener 在 Codex thread handshake 前反复失败。
  - `.symphony/logs/run-20260502-161318.human.log` 记录 `16:23:43` child 在 `Merging` 执行 `git merge --no-ff` 时失败，错误为 `fatal: cannot create directory at 'docs/architecture': Operation not permitted`。
  - 同一日志 `16:24:41` 记录 child 按 blocker 规则把 issue 移到 `Human Review`；`16:26:04` 后 orchestrator 又按 commit handoff 记录 `In Progress -> AI Review -> Merging`，造成 Linear 面板看起来像多段重复流转。
  - 当前 `WORKFLOW.md` 同时要求 child 在实现完成后移动到 `AI Review`，而 `internal/service/orchestrator/orchestrator.go` 也会在 turn 结束后基于 HEAD 变化执行同一 handoff，存在状态 owner 重叠。
  - `internal/service/codex/runner.go` 只把 issue worktree 和 git metadata 加进 `workspaceWrite.writableRoots`；`Merging` prompt 要写 repo root 的 `main` checkout，权限边界不匹配。
- Optimization:
  - Workflow 层：明确 `In Progress` / `Rework` agent turn 只提交、验证和写 workpad handoff，不自行切 `AI Review` / `Merging`；状态推进由 orchestrator 统一负责。
  - 代码层：仅当 issue state 为 `Merging` 时，把 git common-dir 对应的主 checkout root 加入 Codex turn writable roots，让 child 可以自己完成 local main merge、验证和 push。
  - 流程层：把这次用户纠正写入 `lesson.md`，后续 `symphony-issue-run` 不能把父会话手工 merge 当成正常成功路径；若需要人接管，必须先分类为 framework gap 并优化。
- Files:
  - `WORKFLOW.md`
  - `internal/service/codex/runner.go`
  - `internal/service/codex/runner_test.go`
  - `docs/optimization/symphony-issue-run.md`
  - `lesson.md`
- Validation:
  - `git diff --check`
  - `./test.sh ./internal/service/codex ./internal/service/orchestrator`
  - `make build`
- Follow-up: 下次用真实 issue-run 验证 `Merging` 不再进入 `Human Review`；如果仍出现 root merge 卡点，应把 local merge 下沉为 orchestrator first-class action，而不是继续依赖 prompt 执行。

## 2026-05-02 16:05 +08 - ZEE-41

- Trigger: `ZEE-41` issue-scoped listener created the worktree but repeatedly failed before starting the Codex turn.
- Evidence: `.symphony/logs/run-20260502-155913.human.log` recorded `response timeout waiting for id=2`; `internal/service/codex/runner.go:279` sends `thread/start` with id 2; `internal/runtime/config/config.go:94` defaulted `codex.read_timeout_ms` to 5000; current `WORKFLOW.md` did not override it.
- Optimization: Set `codex.read_timeout_ms: 60000` in `WORKFLOW.md` so the app-server startup/thread handshake has enough room in real unattended runs.
- Files: `WORKFLOW.md`, `docs/optimization/symphony-issue-run.md`.
- Validation: `git diff --check`; `./test.sh ./internal/runtime/config ./internal/service/workflow`; `make build`.
- Follow-up: none unless the retry still stalls after the wider handshake timeout.

## 2026-05-02 11:29 +08 - repo-only

- Trigger: 用户希望 `symphony-issue-run` 不只是创建 issue 和启动 listener，而是让框架全自动跑到终态，并在每轮结束后复盘卡点、优化 Skill / Workflow / 代码、记录文档、再 commit 和 push。
- Evidence:
  - 当前 `WORKFLOW.md` 仍使用 `agent.review_policy.mode: human`，默认会停到 `Human Review`。
  - 当前 `.codex/skills/symphony-issue-run/SKILL.md` 仍写着不要在 `Human Review` 停止、等待用户把 issue 移到 `Merging`，不符合全自动 AI 控制目标。
  - 既有中文 smoke 记录已经证明 `Todo -> In Progress -> AI Review -> Merging -> Done` 可以作为自动闭环基线，见 `.codex/skills/zh-smoke-harness/experiments/rounds.md` 的 ZEE-17 记录。
- Optimization:
  - 将 `WORKFLOW.md` 默认 review policy 改成 `mode: auto`，关闭手动 AI review gate，并把默认路径写成 `In Progress -> AI Review -> Merging -> Done`。
  - 重写 `symphony-issue-run` Skill：默认创建 `Todo` issue、启动 issue-scoped listener、监控到 terminal、复盘 Skill / Workflow / code / environment 卡点、记录本文件、验证后 commit 并 push。
  - 明确模型只通过 `codex.command` 切换，不把具体 model 写成流程语义。
- Files:
  - `WORKFLOW.md`
  - `.codex/skills/symphony-issue-run/SKILL.md`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`git diff --check`
  - 通过：`go test ./internal/runtime/config ./internal/service/orchestrator ./internal/service/workflow ./internal/service/issue`
  - 通过：`CGO_ENABLED=0 go test ./...`
- Follow-up:
  - 下一次真实 issue-run 后，用该 issue 的 human log、JSONL log、Linear workpad 和 git evidence 追加一条运行级复盘记录。
