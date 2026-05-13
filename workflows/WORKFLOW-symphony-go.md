---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Todo
    - In Progress
    - AI Review
    - Pushing
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
  mode: static_cwd
  cwd: ..
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
- 从当前 repo root 和目标分支状态继续，不要从头重做。
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

只在当前 repo root 中工作。不要为 issue 创建 git worktree、scratch checkout、临时 clone 或 PR 分支。

## 前置条件：使用 `linear_graphql`，不要使用 Linear MCP/app 工具

Agent 必须能和 Linear 通信。本 workflow 的无人值守目标是验证派生 Codex 会话能否沿用 Symphony listener 使用的 Linear GraphQL 路径，而不是触发需要交互审批的 Linear MCP/app 写入。所有 Linear 读写默认使用当前会话注入的 `linear_graphql` 工具；如果该工具不可用，停止并记录 blocker，让外层 listener/orchestrator 通过自己的 Linear GraphQL client 接管状态和 workpad 写入。

具体规则：

- 读取 issue、team states、comments：使用 `linear_graphql` query，只请求当前步骤需要的字段。
- 更新 issue 状态：先读取 team states 拿到目标 `stateId`，再使用 `linear_graphql` 的 `issueUpdate` mutation。
- 创建或更新 `## Codex Workpad`：使用 `linear_graphql` 的 `commentCreate` / `commentUpdate` mutation。
- 不读取 `.codex/skills/linear-cli/SKILL.md`，不执行 `linear auth whoami`、`linear issue view`、`linear issue update`、`linear issue comment ...` 等 CLI 命令。
- 不调用 Linear MCP/app issue/comment 工具作为兜底。无人值守 child session 中的 MCP 写入会触发审批，正确结果是暴露 blocker，而不是绕回 MCP 或 CLI。

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

## 目标分支策略

- `merge.target` 是本 workflow 的目标开发分支；默认是 `main`，也可以由运行参数或仓库配置覆盖。
- 所有实现、验证、commit 和 push 都在配置的目标分支上完成。
- 开始实现前必须确认当前 cwd 是 repo root，读取 `git status --short --branch`、当前分支、`HEAD` 和 `merge.target`。
- 如果当前分支不是目标分支：
  - 工作区干净时，切换到目标分支。
  - 工作区不干净时，停止并写 blocker；不要在有未解释脏改动时切分支。
- 同步目标分支时只做 fast-forward：`git fetch origin <target>` 后执行 `git pull --ff-only origin <target>`。
- 如果 fast-forward 失败或出现冲突，停止并写 blocker；不要自动 merge、rebase 或改写历史。
- 如果 repo root 有未解释的脏改动，先判断是否属于当前 issue。不能确认归属时停止并写 blocker，不要覆盖、清理或顺手提交。

## 相关技能

- `linear`：使用 `.codex/skills/linear/SKILL.md` 的 `linear_graphql` 协议操作 Linear。
- `commit`：实现阶段产出清晰、逻辑完整的本地 commit。
- `pull`：只用于目标分支 fast-forward 同步，不创建 worktree，不处理 PR。
- `push`：`Pushing` 阶段推送目标分支到远端。

不要打开或执行 `.codex/skills/pr/SKILL.md`，不要创建 PR，不要调用 `gh pr create`、`gh pr merge` 或 `.codex/skills/pr/scripts/pr_merge_flow.sh`。

## Review 路由原则

- 默认路径是 `Todo -> In Progress -> AI Review -> Pushing -> Done`。
- 单个 issue 默认只由同一个 agent session 端到端处理，不为 `AI Review` 额外启动第二个 agent。
- agent 可以多 turn、多 commit；完成 acceptance、validation、workpad 和本地 commit 后，移动到 `AI Review`，并在同一个 session 中继续审查 issue、workpad、本地 diff、commit range 和验证证据。
- AI Review 只审本地结果：代码 diff、commit range、workpad、acceptance criteria 和 validation evidence。
- review 通过后，不进入 PR 或 Merging 流程；框架将 issue 推进到 `Pushing`，同一个 agent session 再推送当前目标分支、更新 workpad push evidence，并用 `Push: PASS` 交给框架移动到 `Done`。
- review 不通过时移动到 `Rework`，同一个 issue agent 必须基于 review 发现重新计划、修复、验证和提交。
- `Human Review` 只作为真实外部 blocker 的人工 hold 状态；不是默认审核阶段，也不属于本 workflow 的 active states。

## 阶段路由

- 本 Workflow 正文是默认注入给 agent 的总 SOP；阶段级说明优先写在这里，避免和额外配置漂移。
- 后续 continuation prompt 只用于阶段续航；不要把它当成重新执行完整 workflow 的理由，也不要重复打开无关 skill。
- 新增阶段时，先在 Linear 创建同名状态，再在本 Workflow 的状态映射和对应步骤里写清楚该阶段的执行协议。

## 状态映射

- `Backlog`：不属于本 workflow 范围；不要修改。
- `Todo`：排队状态；开始主动工作前立即转到 `In Progress`。
- `In Progress`：正在实现、验证和本地提交。
- `AI Review`：本地 commit 和验证已完成，同一个 issue agent 继续审核；通过后由框架进入 `Pushing`，失败时进入 `Rework`。
- `Pushing`：AI Review 已通过；同一个 issue agent 推送目标分支、写 push evidence，并以 `Push: PASS` 结束，随后框架进入 `Done`。
- `Human Review`：仅用于真实外部 blocker 的人工 hold；不是默认 review 终点，也不主动轮询。
- `Rework`：reviewer 要求修改；需要按 review 发现重新计划、实现、验证和提交。
- `Done`：终态；无需继续操作。

## Step 0：确认当前 ticket 状态并路由

1. 用明确 ticket ID 拉取 issue。
2. 读取当前状态。
3. 按状态进入对应流程：
   - `Backlog`：不要修改 issue 内容或状态，停止并等待人类移动到 `Todo`。
   - `Todo`：立即移动到 `In Progress`，然后确保 bootstrap workpad comment 存在，不存在则创建，随后进入执行流程。
   - `In Progress`：从当前 workpad comment 继续执行。
   - `AI Review`：同一个 issue agent 继续审查；通过后进入 `Pushing`，失败时进入 `Rework`。
   - `Pushing`：同一个 issue agent 推送目标分支；成功后以 `Push: PASS` 结束，由框架移动到 `Done`。
   - `Human Review`：只等待真实外部 blocker 的人工解锁；不要自行改代码或推送。
   - `Rework`：基于 AI Review 发现进入 rework 流程。
   - `Done`：什么都不做并退出。
4. 检查 repo root、目标分支、`git status --short --branch` 和 `HEAD`。
   - 如果当前 cwd 不是 repo root，停止并记录 blocker。
   - 如果当前分支不是目标分支，按“目标分支策略”处理。
   - 如果 repo root 有未解释的脏改动，先在 workpad 记录并判断是否属于当前 issue；不要覆盖不理解的改动。
5. 对 `Todo` ticket，启动顺序必须严格如下：
   - 通过 `linear_graphql` issue update mutation 将状态更新为 `In Progress`
   - 查找或创建 `## Codex Workpad` bootstrap comment
   - 然后才开始分析、计划和实现
6. 如果状态和 issue 内容不一致，添加一条短 comment 说明，然后走最稳妥的流程。

## Step 1：开始或继续执行（Todo 或 In Progress）

1. 查找或创建单一持久 workpad comment：
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
   - `<host>:<repo-root>@<target-branch>:<short-sha>`
   - 示例：`devbox-01:/Users/bytedance/symphony-go@main:7bdde33bc`
   - 不要包含 Linear issue 字段已经能推导出的 metadata，例如 issue ID 和 status。
6. 在同一个 comment 中加入明确的 acceptance criteria 和 TODO checklist。
   - 如果改动影响用户可见行为，加入 UI walkthrough 验收项，描述端到端验证路径。
   - 如果改动触及 app 文件或 app 行为，在 `Acceptance Criteria` 中加入 app-specific flow checks，例如启动路径、变更后的交互路径和预期结果。
   - 如果 ticket 正文或评论包含 `Validation`、`Test Plan` 或 `Testing`，把这些要求复制到 workpad 的 `Acceptance Criteria` 和 `Validation`，作为必选 checkbox，不要降级为可选。
7. 对计划做 principal-style self-review，并在 comment 中修正计划。
8. 实现前捕获具体复现信号，并记录到 workpad 的 `Notes`：可以是命令输出、截图或确定性的 UI 行为。
9. 同步目标分支并记录结果：
   - `target` branch
   - fetch/pull 命令和结果：`clean` 或 blocker
   - 同步后的 `HEAD` short SHA
10. compact context，然后进入执行。

## Step 2：执行阶段（Todo -> In Progress -> AI Review）

1. 确认 repo root 状态：target branch、`git status`、`HEAD`，并确认这些信息已记录到 workpad。
2. 如果当前 issue 仍是 `Todo`，移动到 `In Progress`；否则保持当前状态。
3. 加载现有 workpad comment，把它作为 active execution checklist。
   - 现实变化时可以主动编辑它，例如 scope、风险、验证方式或新发现任务。
4. 按分层 TODO 实现，并保持 comment 最新：
   - 勾选已完成事项。
   - 在合适位置添加新发现事项。
   - scope 变化时保持 parent/child 结构完整。
   - 每个有意义里程碑后立即更新 workpad，例如复现完成、代码改动完成、验证运行、commit 完成。
   - 不要让已完成工作在计划里保持未勾选。
5. 运行当前 scope 必需的验证和测试。
   - 强制门禁：执行 ticket 提供的所有 `Validation`、`Test Plan` 或 `Testing` 要求；未满足即视为未完成。
   - 优先使用能直接证明改动行为的 targeted proof。
   - 可以使用临时本地 proof edit 来验证假设，例如临时调整 build input 或本地 hardcode 某个 UI account/response path，但必须在 commit/push 前全部恢复。
   - 把临时 proof 步骤和结果记录到 workpad 的 `Validation` 或 `Notes`，让 reviewer 可复核。
   - 如果触及 app，运行 `launch-app` 验证，并在 workpad 中记录关键路径和结果。
6. 重新检查全部 acceptance criteria，补齐 gap。
7. commit 前运行当前 scope 必需的 validation；如果失败，修复后重跑，直到绿色。
8. 使用 `commit` skill 提交到当前目标分支：
   - commit 前确认 `git status --short` 只包含当前 issue 需要的文件。
   - 一个独立逻辑改动对应一个 commit；多类改动要拆成多个清晰 commit。
   - commit message 必须反映本次 scope，并包含 issue identifier 或清晰关联。
   - commit 后记录目标分支和 `HEAD` short SHA。
9. 更新 workpad comment 的最终 checklist 和 validation notes。
   - 勾选已完成的 plan、acceptance、validation 项。
   - 在同一个 workpad comment 中加入最终 handoff notes，包括 commit range 和 validation summary。
   - 不要把 PR URL 写进 workpad comment；本 workflow 不创建 PR。
   - 如果执行中有任何不清楚的地方，在底部添加简短 `### Confusions` section。
   - 不要额外发布 completion summary comment。
   - 正常简单任务不要为每个小命令更新 workpad；实现阶段至少覆盖 bootstrap、验证和 commit handoff。
10. 移动到 `AI Review` 前，确认本地 review evidence 完整：
    - 确认 ticket 提供的全部 validation/test-plan 项已经在 workpad 中显式完成。
    - 状态切换前重新打开并刷新 workpad，让 `Plan`、`Acceptance Criteria`、`Validation` 与已完成工作完全一致。
11. 只有满足完成门槛后，当前 issue agent 才能移动 issue 到 `AI Review`，并继续同一个 session 的 review 流程。
    - 当前 issue agent 不直接从实现阶段跳到 `Pushing` 或 `Done`；`Pushing` 只在 AI Review 通过后进入。
    - 例外：如果按 blocked-access escape hatch 被工具或 auth 阻塞，可以带 blocker brief 和明确解锁动作移动到 `Human Review`。

## Step 3：AI Review 与 Pushing handling

1. 当 issue 处于 `AI Review`，同一个 issue agent 审查 issue、workpad、本地 diff、commit range 和验证证据；不要创建 PR，也不要运行 PR feedback sweep。
2. 如果发现需要修改的问题，当前 issue agent 将 findings 写入 workpad，移动 issue 到 `Rework`，然后基于发现继续修复或等待下一轮同 issue continuation。
3. 如果 review 通过，最终回复以 `Review: PASS` 开头；框架会把 issue 推进到 `Pushing`，并在同一个 session 中继续。
4. 当 issue 处于 `Pushing`，确认 repo root 仍在目标分支且 `git status --short` 干净。
5. push 前再次确认本地 commit range、validation evidence 和 workpad 都已记录。
6. 执行 `git push origin <target>`，只推送配置的目标分支。
7. push 成功后，更新 workpad push evidence：
   - pushed branch
   - pushed `HEAD` short SHA
   - validation summary
   - AI Review verdict
8. 最终回复以 `Push: PASS` 开头，包含目标分支、pushed commit 和验证摘要；框架会把 issue 移动到 `Done`。

## Step 4：Rework 处理

1. 把 `Rework` 当作完整方案重置，而不是增量补丁。
2. 重新阅读完整 issue body、workpad 和所有人工 comments；明确本次 attempt 要做出哪些不同处理。
3. 继续使用同一个目标分支和同一个 repo root，不创建 worktree、PR 分支或临时 clone。
4. 从目标分支当前状态重新开始：
   - 如果本地已有未推送 commit，先判断是否属于当前 issue。
   - 如果未推送 commit 属于当前 issue，基于 review findings 追加修复 commit；不要默认改写历史。
   - 如果未推送 commit 归属不清，停止并写 blocker。
5. 从正常 kickoff flow 重新开始：
   - 如果当前 issue 状态是 `Todo`，移动到 `In Progress`；否则保持当前状态。
   - 复用现有 `## Codex Workpad` comment。
   - 构建新的 plan/checklist，并端到端执行。

## 移动到 AI Review 前的完成门槛

- Step 1/2 checklist 全部完成，并准确反映在单一 workpad comment 中。
- Acceptance criteria 和 ticket 提供的必要 validation items 全部完成。
- 最新 commit 的 validation/tests 绿色。
- 本地 diff、commit range 和验证证据足以支持 AI Review。
- workpad 已记录目标分支、HEAD short SHA、commit range、validation 命令和结果。
- 如果触及 app，runtime validation/media 要求已完成。

## 移动到 Done 前的完成门槛

- AI Review 已通过，且 findings 为空或已在 `Rework` 中解决。
- issue 已处于 `Pushing`。
- 当前分支是配置的目标分支。
- `git status --short` 干净。
- `git push origin <target>` 已成功。
- workpad 已记录 push evidence、pushed commit 和 validation summary。

## Blocked-access escape hatch（必须遵守）

仅当缺少必要工具、auth 或权限，且本 session 无法解决时使用。

- Git push 默认不是 valid blocker；必须先尝试可逆 fallback，例如 `git fetch`、确认 remote、确认当前 auth 状态、重试一次 push。
- Git access/auth 问题不能直接移动到 `Human Review`，除非所有 fallback 都已尝试并记录在 workpad。
- 如果缺少必要工具或 auth，移动 ticket 到 `Human Review`，并在 workpad 写入短 blocker brief，包含：
  - 缺少什么
  - 为什么阻塞必要 acceptance、validation 或 push
  - 需要人类执行的精确解锁动作
- brief 要简洁、可执行；不要额外发布 top-level comment。

## Guardrails

- 如果 issue 状态是 `Backlog`，不要修改它；等待人类移动到 `Todo`。
- 不要编辑 issue body/description 来记录计划或进度。
- 每个 issue 只使用一个持久 workpad comment：`## Codex Workpad`。
- 如果 session 内无法通过 `linear_graphql` 编辑 comment，先记录 blocker；不要调用 Linear MCP/app 或 Linear CLI 兜底。
- 不要创建 issue worktree、scratch checkout、临时 clone、PR 分支或 PR。
- 不要打开或执行 `pr` skill，不要调用 `gh pr create`、`gh pr merge` 或 `pr_merge_flow.sh`。
- 临时 proof edit 只允许用于本地验证，commit 前必须恢复。
- 如果发现超出范围的改进，创建单独 Backlog issue，不要扩大当前 scope；该 issue 要有清晰标题、描述、验收标准、同 project 归属、与当前 issue 的 `related` 链接，并在依赖当前 issue 时设置 `blockedBy`。
- 未达到 `AI Review` 完成门槛前，不要移动到 `AI Review`；达到门槛后由当前 issue agent 移动到 `AI Review`，并保持同一个 session 继续后续阶段。
- 不要把 `Human Review` 当成默认审核阶段；它只用于真实外部 blocker。
- 在 `AI Review` 中只做 review；实现修改应通过 `Rework` 返回实现阶段。
- 在 `Pushing` 前不要 push；push 后必须更新 workpad，并以 `Push: PASS` 让框架收口 `Done`。
- 如果状态是 terminal，例如 `Done`，什么都不做并退出。
- issue 文本保持简洁、具体、面向 reviewer。
- 如果被阻塞且尚无 workpad，添加一个 blocker comment，说明 blocker、影响和下一步解锁动作。

## Workpad 模板

使用以下结构作为持久 workpad comment，并在执行过程中原地更新：

````md
## Codex Workpad

```text
<hostname>:<repo-root>@<target-branch>:<short-sha>
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
