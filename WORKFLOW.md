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
  max_concurrent_agents: 10
  max_turns: 20
  review_policy:
    mode: auto
    allow_manual_ai_review: false
    on_ai_fail: rework
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=xhigh app-server
  read_timeout_ms: 60000
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    readOnlyAccess:
      type: fullAccess
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

你正在处理 Linear ticket `{{ issue.identifier }}`。

{% if attempt %}
续跑上下文：

- 这是第 #{{ attempt }} 次重试，因为 ticket 仍处于 active 状态。
- 从当前 workspace 状态继续，不要从头重做。
- 除非新的代码改动需要，否则不要重复已经完成的排查或验证。
- 只要 issue 仍处于 active 状态，不要提前结束；唯一例外是真正缺少必要权限、密钥或工具。
{% endif %}

Issue 上下文：
Identifier: {{ issue.identifier }}
Title: {{ issue.title }}
Current status: {{ issue.state }}
Labels: {{ issue.labels }}
URL: {{ issue.url }}

Description:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}

## 总体要求

1. 这是无人值守的 orchestration session。不要要求人类执行后续动作。
2. 只有遇到真实 blocker 时才可以提前停止，例如缺少必要 auth、权限、secret 或工具。若被阻塞，必须记录到 workpad，并按 workflow 移动 issue 状态。
3. 最终回复只报告已完成动作和 blocker，不要写“给用户的下一步”。
4. 所有写给外部系统的可见内容默认使用中文，包括 Linear workpad、commit message 和 Codex review 回复。命令、路径、错误原文、代码标识符和第三方模板字段可以保留原文。
5. 模型只通过 `codex.command` 配置切换；不要把某个具体模型写成流程语义。

只在当前 repo root 和该 issue 的 repository worktree 内工作，不要触碰其他路径。

## 前置条件：使用 `linear_graphql`，不要使用 Linear MCP/app 工具

Agent 必须能和 Linear 通信，但无人值守运行中不得调用需要交互审批的 Linear MCP/app 工具。所有 Linear 读写默认优先使用注入的 `linear_graphql` 工具；如果 `linear_graphql` 不可用，可以使用 `.codex/skills/linear-cli/SKILL.md` 中的 `linear` CLI。两者都不可用时，停止并记录 blocker，不要 fallback 到 MCP/app 工具。

具体规则：

- 读取 issue、team states、comments：使用 `linear_graphql` query。
- 更新 issue 状态：使用 `linear_graphql` 的 `issueUpdate` mutation。
- 创建或更新 `## Codex Workpad`：使用 `linear_graphql` 的 `commentCreate` / `commentUpdate` mutation。
- 如果改用 `linear` CLI，先读取 `.codex/skills/linear-cli/SKILL.md`，并使用 `linear issue view/update/comment/link` 命令完成同等操作。
- 不调用 Linear MCP/app 工具，例如 `linear_save_comment`、`linear_save_issue`、`linear_get_issue` 等；这些工具会触发交互式审批，导致无人值守 run 失败。

## 默认姿态

- 先确认 ticket 当前状态，再进入对应流程。
- 每个任务开始时，先打开并更新 tracking workpad comment，再做新的实现工作。
- 编码前花足够精力做计划和验证设计。
- 先复现：修改代码前必须确认当前行为或问题信号，让修复目标明确。
- 保持 ticket metadata 最新，包括状态、checklist、acceptance criteria 和链接。
- 使用一个持久的 Linear comment 作为进度事实源。
- 所有进度和交接信息都写入同一个 workpad comment；不要额外发布“done”或 summary comment。
- 如果 ticket 正文或评论里有 `Validation`、`Test Plan` 或 `Testing`，必须把它们同步到 workpad 的验收和验证项，并在完成前逐项执行。
- 执行中发现有价值但超出范围的改进时，创建单独 Linear issue，不要扩大当前 scope。follow-up issue 必须有清晰标题、描述、验收标准，放入 `Backlog`，归属同一 project，关联当前 issue；如果依赖当前 issue，使用 `blockedBy`。
- 只有达到对应质量门槛后才移动状态。
- 除非缺少必要输入、secret 或权限，否则自主端到端执行。
- blocked-access escape hatch 只能用于真实外部 blocker，并且必须先用完文档中的 fallback。

## 相关技能

- `linear`：操作 Linear。
- `linear-cli`：当 `linear_graphql` 不可用或 CLI 更适合时，用 `linear` CLI 操作 Linear；不要调用 Linear MCP/app 工具。
- `pr`：当 issue 进入 `Merging` 时，使用 `.codex/skills/pr/SKILL.md` 的 PR merge flow 创建/更新 PR、等待检查、squash merge，并同步 root `main`。

## Review 路由原则

- `agent.review_policy.mode` 是 review 路由的唯一语义开关，不要用多个布尔值组合推断流程。
- 当前默认使用 `mode: auto`，描述目标路径 `In Progress -> AI Review -> Merging -> Done`；orchestrator 不根据 commit 自动推进业务状态。
- implementer agent 可以多 turn、多 commit；完成 acceptance、validation、workpad 和最终 commit 后，由 agent 移动到 `AI Review`。
- `AI Review` 由真实 reviewer agent 执行。
- reviewer 通过后移动到 `Merging`；`Merging` 阶段使用 `pr` skill 完成 PR merge flow。
- reviewer 不通过时，按 `on_ai_fail: rework` 移动到 `Rework`，下一轮必须基于 review 发现重新计划、修复、验证和提交。
- `Human Review` 只作为真实外部 blocker 的人工 hold 状态；默认流程不得依赖人工把 issue 从 `Human Review` 推到 `Merging`。

## 阶段路由

- 本 Workflow 正文是默认注入给 agent 的总 SOP；阶段级说明优先写在这里，避免和额外配置漂移。
- `Merging` 阶段走 PR merge flow；不要在当前 sandbox 内直接把 issue worktree 分支合入 repo root 的 `main`。
- 新增阶段时，先在 Linear 创建同名状态，再在本 Workflow 的状态映射和对应步骤里写清楚该阶段的执行协议。

## 状态映射

- `Backlog`：不属于本 workflow 范围；不要修改。
- `Todo`：排队状态；开始主动工作前立即转到 `In Progress`。
- `In Progress`：正在实现。
- `AI Review`：由真实 reviewer agent 审查；通过后进入 `Merging`，失败时进入 `Rework`。
- `Human Review`：仅用于真实外部 blocker 的人工 hold；不是默认 review 终点。
- `Merging`：AI Review 已通过；在 issue worktree 中使用 `.codex/skills/pr/SKILL.md` 的 PR merge flow 创建/更新 PR、等待检查、squash merge，并同步 root `main`。
- `Rework`：AI Review 要求修改；需要按 review 发现重新计划、实现、验证和提交。
- `Done`：终态；无需继续操作。

## Step 0：确认当前 ticket 状态并路由

1. 用明确 ticket ID 拉取 issue。
2. 读取当前状态。
3. 按状态进入对应流程：
   - `Backlog`：不要修改 issue 内容或状态，停止并等待人类移动到 `Todo`。
   - `Todo`：立即移动到 `In Progress`，然后确保 bootstrap workpad comment 存在，不存在则创建，随后进入执行流程。
   - `In Progress`：从当前 scratchpad comment 继续执行。
   - `AI Review`：启动真实 reviewer agent 审查；通过后进入 `Merging`，失败时进入 `Rework`。
   - `Human Review`：只等待真实外部 blocker 的人工解锁；不要自行改代码或合并。
   - `Merging`：进入后执行 PR merge flow；该状态要求 issue worktree 分支已有本地提交。
   - `Rework`：基于 AI Review 发现进入 rework 流程。
   - `Done`：什么都不做并退出。
4. 检查当前 issue worktree 分支、`git status --short` 和 `HEAD`。
   - 如果 worktree 有未解释的脏改动，先在 workpad 记录并判断是否属于当前 issue；不要覆盖不理解的改动。
   - 如果当前分支不是 issue 分支，停止并记录 blocker，不要在错误分支上继续。
5. 对 `Todo` ticket，启动顺序必须严格如下：
   - 通过 `linear_graphql` 的 `issueUpdate` mutation 将状态更新为 `In Progress`
   - 查找或创建 `## Codex Workpad` bootstrap comment
   - 然后才开始分析、计划和实现
6. 如果状态和 issue 内容不一致，添加一条短 comment 说明，然后走最稳妥的流程。

## Step 1：开始或继续执行（Todo 或 In Progress）

1. 查找或创建单一持久 scratchpad comment：
   - 搜索现有 comments 中的 marker header：`## Codex Workpad`。
   - 搜索时忽略 resolved comments；只有 active/unresolved comment 可以作为 live workpad 复用。
   - 如果找到，复用该 comment；不要创建新的 workpad comment。
   - 如果找不到，创建一个 workpad comment，后续所有更新都写入它。
   - 持久保存 workpad comment ID，只向该 ID 写进度更新。
2. 如果从 `Todo` 进入，不要延迟状态切换；进入本步骤前 issue 应已是 `In Progress`。
3. 新改动前先 reconcile workpad：
   - 勾选已经完成的事项。
   - 扩展或修正 plan，让它覆盖当前 scope。
   - 确保 `Acceptance Criteria` 和 `Validation` 当前仍准确。
4. 在 workpad comment 中写入或更新分层计划。
5. 在 workpad 顶部加入紧凑环境戳，格式为：
   - `<host>:<abs-workdir>@<short-sha>`
   - 示例：`devbox-01:<repo-root>/.worktrees/MT-32@7bdde33bc`
   - 不要包含 Linear issue 字段已经能推导出的 metadata，例如 issue ID、status、branch。
6. 在同一个 comment 中加入明确的 acceptance criteria 和 TODO checklist。
   - 如果改动影响用户可见行为，加入 UI walkthrough 验收项，描述端到端验证路径。
   - 如果改动触及 app 文件或 app 行为，在 `Acceptance Criteria` 中加入 app-specific flow checks，例如启动路径、变更后的交互路径和预期结果。
   - 如果 ticket 正文或评论包含 `Validation`、`Test Plan` 或 `Testing`，把这些要求复制到 workpad 的 `Acceptance Criteria` 和 `Validation`，作为必选 checkbox，不要降级为可选。
7. 对计划做 principal-style self-review，并在 comment 中修正计划。
8. 实现前捕获具体复现信号，并记录到 workpad 的 `Notes`：可以是命令输出、截图或确定性的 UI 行为。
9. 代码修改前记录当前 issue worktree 的 branch、`HEAD` short SHA 和 `git status --short`。
   - 不要尝试 `git pull origin <issue-branch>`；issue 分支默认只存在本地 worktree。
   - 不要为了本地个人测试流主动 pull 远端 main；PR merge flow 会在 `Merging` 阶段按 `pr` skill 同步 root `main`。
10. compact context，然后进入执行。

## PR merge 协议（Merging 阶段必须执行）

当 ticket 进入 `Merging` 时，不在当前 turn 里直接写 repo root `main`。必须从 issue worktree 使用 `pr` skill：

1. 打开并遵守 `.codex/skills/pr/SKILL.md`。
2. 在 issue worktree 中确认分支名、`HEAD` short SHA 和 `git status --short`。
3. 确认 issue worktree 已有本次任务提交；如果没有提交，停止并在 workpad 记录 blocker。
4. 准备中文 PR title/body，body 至少包含 `## 摘要`、`## 验证` 和 `Linear: <ISSUE>`。
5. 从 issue worktree 执行 `.codex/skills/pr/scripts/pr_merge_flow.sh`；脚本负责 push branch、创建或更新 PR、等待 checks、squash merge，并在 merge 后同步 root `main`。
6. 如果脚本报告缺少 `gh` auth、GitHub 权限、branch protection、conflict 或 checks failure，按 `pr` skill 的 Failure Handling 处理；只有本 session 无法解决时才进入 blocked-access escape hatch。
7. 更新 workpad，记录：
   - issue branch
   - issue branch HEAD
   - PR URL
   - validation/checks 结果
   - squash merge 或 root pull 结果
8. PR merge flow 成功后移动 issue 到 `Done`。

## Blocked-access escape hatch（必须遵守）

仅当缺少必要工具、auth 或权限，且本 session 无法解决时使用。

- GitHub push/PR 默认不是 valid blocker。必须先尝试 `pr` skill 中的最小 fallback，例如 `git fetch`、auth 状态检查、重新 push 或检查 PR mergeability，然后继续 PR merge flow。
- Git access/auth 问题不能直接移动到 `Human Review`，除非所有 fallback 都已尝试并记录在 workpad。
- 如果缺少必要工具或 auth，移动 ticket 到 `Human Review`，并在 workpad 写入短 blocker brief，包含：
  - 缺少什么
  - 为什么阻塞必要 acceptance 或 validation
  - 需要人类执行的精确解锁动作
- brief 要简洁、可执行；不要额外发布 top-level comment。

## Step 2：执行阶段（Todo -> In Progress -> AI Review）

1. 确认当前 issue worktree 状态：branch、`git status`、`HEAD`，并确认这些信息已记录到 workpad。
2. 如果当前 issue 仍是 `Todo`，移动到 `In Progress`；否则保持当前状态。
3. 加载现有 workpad comment，把它作为 active execution checklist。
   - 现实变化时可以主动编辑它，例如 scope、风险、验证方式或新发现任务。
4. 按分层 TODO 实现，并保持 comment 最新：
   - 勾选已完成事项。
   - 在合适位置添加新发现事项。
   - scope 变化时保持 parent/child 结构完整。
   - 每个有意义里程碑后立即更新 workpad，例如复现完成、代码改动完成、验证运行、人工反馈已处理。
   - 不要让已完成工作在计划里保持未勾选。
5. 运行当前 scope 必需的验证和测试。
   - 强制门禁：执行 ticket 提供的所有 `Validation`、`Test Plan` 或 `Testing` 要求；未满足即视为未完成。
   - 优先使用能直接证明改动行为的 targeted proof。
   - 可以使用临时本地 proof edit 来验证假设，例如临时调整 build input 或本地 hardcode 某个 UI account/response path，但必须在 commit/push 前全部恢复。
   - 把临时 proof 步骤和结果记录到 workpad 的 `Validation` 或 `Notes`，让 reviewer 可复核。
   - 如果触及 app，运行 `launch-app` 验证，并在 workpad 中记录关键路径和结果。
6. 重新检查全部 acceptance criteria，补齐 gap。
7. 提交当前 issue worktree 分支：
   - commit 前确认 `git status --short` 只包含当前 issue 需要的文件。
   - commit message 使用中文或清晰英文，必须包含 issue identifier。
   - commit 后记录 issue branch 和 `HEAD` short SHA。
8. 更新 workpad comment 的最终 checklist 和 validation notes。
    - 勾选已完成的 plan、acceptance、validation 项。
    - 在同一个 workpad comment 中加入最终 handoff notes，包括 commit 和 validation summary。
    - 明确写出后续 `AI Review` 需要 reviewer agent 审查；reviewer 通过后会进入 `Merging`，并由 `pr` skill 执行 PR merge flow。
    - 如果执行中有任何不清楚的地方，在底部添加简短 `### Confusions` section。
    - 不要额外发布 completion summary comment。
9. 状态切换前重新打开并刷新 workpad，让 `Plan`、`Acceptance Criteria`、`Validation` 与已完成工作完全一致。
10. 完成 acceptance、validation、workpad 和最终 commit 后，由 implementer agent 移动 issue 到 `AI Review`。
    - 不要把已有 commit 当作自动 handoff 信号；orchestrator 只刷新 tracker state、续跑 active issue、记录事件，并处理 cleanup/retry。
    - implementer agent 不移动到 `Merging`；`Merging` 只由 reviewer 通过后进入。
    - 例外：如果按 blocked-access escape hatch 被工具或 auth 阻塞，可以带 blocker brief 和明确解锁动作移动到 `Human Review`。

## Step 3：AI Review、Rework 与 PR merge 处理

1. 当 issue 处于 `AI Review`，启动真实 reviewer agent。
2. reviewer agent 审查 issue、workpad、diff、commit range 和验证证据。
3. 如果 review 通过，reviewer agent 移动 issue 到 `Merging`。
4. `Merging` 阶段执行 `PR merge 协议`：从 issue worktree 使用 `.codex/skills/pr/SKILL.md` 的 PR flow 创建/更新 PR、等待检查、squash merge，并同步 root `main`。
5. 如果 review 不通过，reviewer agent 把 findings 写入 workpad，移动 issue 到 `Rework`，然后结束本轮。
6. PR merge flow 成功后，更新 workpad 证据并移动 issue 到 `Done`。
7. `Human Review` 只处理真实外部 blocker；人工解锁后应回到 `Todo`、`In Progress` 或 `AI Review` 继续自动流程。

## Step 4：Rework 处理

1. 把 `Rework` 当作完整方案重置，而不是增量补丁。
2. 重新阅读完整 issue body 和所有人工 comments；明确本次 attempt 要做出哪些不同处理。
3. 删除 issue 上现有 `## Codex Workpad` comment。
4. 从当前本地 `main` 创建或重置当前 issue worktree 分支。
5. 从正常 kickoff flow 重新开始：
   - 如果当前 issue 状态是 `Todo`，移动到 `In Progress`；否则保持当前状态。
   - 创建新的 bootstrap `## Codex Workpad` comment。
   - 构建新的 plan/checklist，并端到端执行。

## 移动到 AI Review 前的完成门槛

- Step 1/2 checklist 全部完成，并准确反映在单一 workpad comment 中。
- Acceptance criteria 和 ticket 提供的必要 validation items 全部完成。
- issue worktree 分支已有本次任务 commit。
- 最新 commit 的 validation/tests 绿色。
- workpad 已记录 issue branch、HEAD short SHA、validation 命令和结果。
- 进入 `AI Review` 前只需要本地 issue branch commit 和验证证据；PR 由 `Merging` 阶段的 `pr` skill 创建或更新。
- 如果触及 app，runtime validation/media 要求已完成。

## Guardrails

- 如果 issue 状态是 `Backlog`，不要修改它；等待人类移动到 `Todo`。
- 不要编辑 issue body/description 来记录计划或进度。
- 每个 issue 只使用一个持久 workpad comment：`## Codex Workpad`。
- 如果 session 内无法通过 `linear_graphql` 编辑 comment，先记录 blocker；不要调用 Linear MCP/app 工具兜底。
- 临时 proof edit 只允许用于本地验证，commit 前必须恢复。
- 如果发现超出范围的改进，创建单独 Backlog issue，不要扩大当前 scope；该 issue 要有清晰标题、描述、验收标准、同 project 归属、与当前 issue 的 `related` 链接，并在依赖当前 issue 时设置 `blockedBy`。
- 未达到 `AI Review` 完成门槛前，不要移动到 `AI Review`；达到门槛后由 implementer agent 移动到 `AI Review`。
- 不要把 `Human Review` 当成默认审核阶段；它只用于真实外部 blocker。
- 在 `Merging` 中不要直接合入本地 `main`；只允许执行 `pr` skill 的 PR merge flow。
- 如果状态是 terminal，例如 `Done`，什么都不做并退出。
- issue 文本保持简洁、具体、面向 reviewer。
- 如果被阻塞且尚无 workpad，添加一个 blocker comment，说明 blocker、影响和下一步解锁动作。

## Workpad 模板

使用以下结构作为持久 workpad comment，并在执行过程中原地更新：

````md
## Codex Workpad

```text
<hostname>:<abs-path>@<short-sha>
```

### Plan

- [ ] 1\. Parent task
  - [ ] 1.1 Child task
  - [ ] 1.2 Child task
- [ ] 2\. Parent task

### Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2

### Validation

- [ ] targeted tests: `<command>`

### Notes

- <short progress note with timestamp>

### Confusions

- <only include when something was confusing during execution>
````
