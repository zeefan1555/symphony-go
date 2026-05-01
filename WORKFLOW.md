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
agent:
  max_concurrent_agents: 10
  max_turns: 20
  review_policy:
    mode: human
    allow_manual_ai_review: true
    on_ai_fail: rework
  merge_policy:
    mode: pr # local | pr
    skill: land # local-merge | land
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=xhigh app-server
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
4. 所有写给外部系统的可见内容默认使用中文，包括 Linear workpad、GitHub PR 标题、PR 正文、PR 根评论、review inline 回复和 Codex review 回复。命令、路径、错误原文、代码标识符和第三方模板字段可以保留原文。

只在提供的 repository worktree 内工作，不要触碰其他路径。

## 前置条件：使用 `linear_graphql`，不要使用 Linear MCP/app 工具

Agent 必须能和 Linear 通信，但无人值守运行中不得调用需要交互审批的 Linear MCP/app 工具。所有 Linear 读写默认优先使用注入的 `linear_graphql` 工具；如果 `linear_graphql` 不可用，可以使用 `.codex/skills/linear-cli/SKILL.md` 中的 `linear` CLI。两者都不可用时，停止并记录 blocker，不要 fallback 到 MCP/app 工具。

具体规则：

- 读取 issue、team states、comments：使用 `linear_graphql` query。
- 更新 issue 状态：使用 `linear_graphql` 的 `issueUpdate` mutation。
- 创建或更新 `## Codex Workpad`：使用 `linear_graphql` 的 `commentCreate` / `commentUpdate` mutation。
- 关联 GitHub PR：优先使用 `linear_graphql` 的 `attachmentLinkGitHubPR` mutation。
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
- GitHub PR 标题和正文必须使用中文，正文至少包含 `摘要`、`验证` 和关联 Linear issue。
- 所有 GitHub 评论必须使用中文，并用 `[codex]` 前缀标识 agent 回复；引用 reviewer 原文或命令输出时可以保留原文。
- 如果 ticket 正文或评论里有 `Validation`、`Test Plan` 或 `Testing`，必须把它们同步到 workpad 的验收和验证项，并在完成前逐项执行。
- 执行中发现有价值但超出范围的改进时，创建单独 Linear issue，不要扩大当前 scope。follow-up issue 必须有清晰标题、描述、验收标准，放入 `Backlog`，归属同一 project，关联当前 issue；如果依赖当前 issue，使用 `blockedBy`。
- 只有达到对应质量门槛后才移动状态。
- 除非缺少必要输入、secret 或权限，否则自主端到端执行。
- blocked-access escape hatch 只能用于真实外部 blocker，并且必须先用完文档中的 fallback。

## 相关技能

- `linear`：操作 Linear。
- `linear-cli`：当 `linear_graphql` 不可用或 CLI 更适合时，用 `linear` CLI 操作 Linear；不要调用 Linear MCP/app 工具。
- `commit`：在实现过程中创建干净、逻辑清晰的 commit。
- `push`：当 `agent.merge_policy.mode: pr` 时，保持远端分支最新，并创建或更新 PR。
- `pull`：交接前把分支同步到最新 `origin/main`。
- `local-merge`：当 `agent.merge_policy.mode: local` 且 ticket 进入 `Merging` 时，打开并遵循 `.codex/skills/local-merge/SKILL.md`，按直接 merge flow 落地。
- `land`：当 `agent.merge_policy.mode: pr` 且 ticket 进入 `Merging` 时，打开并遵循 `.codex/skills/land/SKILL.md`，按 PR land loop 落地。

## Review 路由原则

- `agent.review_policy.mode` 是 review 路由的唯一语义开关，不要用多个布尔值组合推断流程。
- `mode: human`：默认路径是 `In Progress -> Human Review`。实现、验证和 commit 完成后，等待人工审核；是否 push/创建 PR 由 `agent.merge_policy` 决定。
- `mode: ai`：默认路径是 `In Progress -> AI Review -> Human Review`。AI Review 通过后仍等待人工决定是否合并。
- `mode: auto`：默认路径是 `In Progress -> AI Review -> Merging -> Done`。AI Review 通过后自动进入 merge policy 对应 skill flow。
- `Human Review` 是等待态。daemon 可以轮询并记录等待，但不得在该状态继续写代码、改 PR 或自行合并。
- 当 `allow_manual_ai_review: true` 时，人工可以把 issue 从 `Human Review` 手动切到 `AI Review`，让 daemon 额外跑一次机器复核。
- `AI Review` 通过后按 `mode` 决定下一站：`human` / `ai` 回到 `Human Review`，`auto` 进入 `Merging`。
- `AI Review` 不通过时，按 `on_ai_fail` 处理：`rework` 进入 `Rework`，`hold` 停留在 `AI Review` 并把 blocker 写入 workpad。
- 人工批准合并时，只需要把 issue 切到 `Merging`；后续由 `agent.merge_policy` 对应 skill flow 自动处理。

## Merge Policy

- `agent.merge_policy.mode` 是 Merging 阶段的唯一语义开关，不要把 `Merging` 直接等同为本地 merge 或 PR merge。
- `mode: local`：使用 `.codex/skills/local-merge/SKILL.md`。适合不走 PR 的直接落地流程。
- `mode: pr`：使用 `.codex/skills/land/SKILL.md`。适合需要 GitHub PR review/checks/merge 的流程。
- `skill` 允许显式写出对应 skill 名；当前只接受 `local-merge` 和 `land`。

## 状态映射

- `Backlog`：不属于本 workflow 范围；不要修改。
- `Todo`：排队状态；开始主动工作前立即转到 `In Progress`。
  - 特例：如果已经挂了 PR，把它当作反馈或 rework loop，先完整扫 PR feedback，处理或明确 pushback，重新验证，再回到 `Human Review`。
- `In Progress`：正在实现。
- `AI Review`：由 `review_policy.mode` 自动触发，或由人工从 `Human Review` 手动触发；复核通过后按 review policy 路由，复核失败时按 `on_ai_fail` 处理。
- `Human Review`：实现已提交并验证，等待人工批准；这是默认 review 终点。
- `Merging`：人工已批准；读取 `agent.merge_policy`，执行对应 skill flow，不要把此状态硬编码成某一种 merge 命令。
- `Rework`：reviewer 要求修改；需要重新计划和实现。
- `Done`：终态；无需继续操作。

## Step 0：确认当前 ticket 状态并路由

1. 用明确 ticket ID 拉取 issue。
2. 读取当前状态。
3. 按状态进入对应流程：
   - `Backlog`：不要修改 issue 内容或状态，停止并等待人类移动到 `Todo`。
   - `Todo`：立即移动到 `In Progress`，然后确保 bootstrap workpad comment 存在，不存在则创建，随后进入执行流程。
     - 如果 PR 已存在，先读取所有 open PR comments，判断哪些需要修改、哪些需要明确 pushback。
   - `In Progress`：从当前 scratchpad comment 继续执行。
   - `AI Review`：执行 AI Review；通过后回到 `Human Review`，失败时进入 `Rework` 或记录 blocker。
   - `Human Review`：等待并轮询人工决策或 review 更新；不要自行改代码或合并。
   - `Merging`：进入后读取 `agent.merge_policy`，打开并遵循对应 `.codex/skills/<skill>/SKILL.md`。
   - `Rework`：进入 rework 流程。
   - `Done`：什么都不做并退出。
4. 检查当前分支是否已有 PR，以及 PR 是否已关闭。
   - 如果当前分支 PR 已 `CLOSED` 或 `MERGED`，本轮不要复用旧分支工作。
   - 从 `origin/main` 创建新分支，并作为新 attempt 重启执行流程。
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
   - 不要包含 Linear issue 字段已经能推导出的 metadata，例如 issue ID、status、branch、PR link。
6. 在同一个 comment 中加入明确的 acceptance criteria 和 TODO checklist。
   - 如果改动影响用户可见行为，加入 UI walkthrough 验收项，描述端到端验证路径。
   - 如果改动触及 app 文件或 app 行为，在 `Acceptance Criteria` 中加入 app-specific flow checks，例如启动路径、变更后的交互路径和预期结果。
   - 如果 ticket 正文或评论包含 `Validation`、`Test Plan` 或 `Testing`，把这些要求复制到 workpad 的 `Acceptance Criteria` 和 `Validation`，作为必选 checkbox，不要降级为可选。
7. 对计划做 principal-style self-review，并在 comment 中修正计划。
8. 实现前捕获具体复现信号，并记录到 workpad 的 `Notes`：可以是命令输出、截图或确定性的 UI 行为。
9. 代码修改前运行 `pull` skill，同步最新 `origin/main`，并把结果记录到 workpad 的 `Notes`。
   - 记录 `pull skill evidence`，包含：
     - merge source(s)
     - 结果：`clean` 或 `conflicts resolved`
     - 同步后的 `HEAD` short SHA
10. compact context，然后进入执行。

## PR feedback sweep 协议（必须执行）

当 ticket 已挂载 PR 时，进入 `Human Review` 前必须执行：

1. 从 issue links 或 attachments 中识别 PR number。
2. 收集所有反馈渠道：
   - 顶层 PR comments：`gh pr view --comments`
   - inline review comments：`gh api repos/<owner>/<repo>/pulls/<pr>/comments`
   - review summaries/states：`gh pr view --json reviews`
3. 每条 actionable reviewer comment，无论来自人还是 bot，包括 inline review comment，都视为 blocking，直到满足以下之一：
   - 已通过代码、测试或文档改动解决
   - 已在该 thread 上发布明确且有理由的中文 pushback 回复
4. 更新 workpad plan/checklist，列出每个 feedback item 及其解决状态。
5. feedback-driven change 后重新运行验证并 push 更新。
6. 循环执行，直到没有 outstanding actionable comments。

## Blocked-access escape hatch（必须遵守）

仅当缺少必要工具、auth 或权限，且本 session 无法解决时使用。

- GitHub 默认不是 valid blocker。必须先尝试 fallback 策略，例如 alternate remote/auth mode，然后继续 publish/review flow。
- GitHub access/auth 问题不能直接移动到 `Human Review`，除非所有 fallback 都已尝试并记录在 workpad。
- 如果缺少非 GitHub 的必要工具，或缺少非 GitHub 的必要 auth，移动 ticket 到 `Human Review`，并在 workpad 写入短 blocker brief，包含：
  - 缺少什么
  - 为什么阻塞必要 acceptance 或 validation
  - 需要人类执行的精确解锁动作
- brief 要简洁、可执行；不要额外发布 top-level comment。

## Step 2：执行阶段（Todo -> In Progress -> Human Review）

1. 确认当前 repo 状态：branch、`git status`、`HEAD`，并确认 kickoff `pull` sync 结果已记录到 workpad。
2. 如果当前 issue 仍是 `Todo`，移动到 `In Progress`；否则保持当前状态。
3. 加载现有 workpad comment，把它作为 active execution checklist。
   - 现实变化时可以主动编辑它，例如 scope、风险、验证方式或新发现任务。
4. 按分层 TODO 实现，并保持 comment 最新：
   - 勾选已完成事项。
   - 在合适位置添加新发现事项。
   - scope 变化时保持 parent/child 结构完整。
   - 每个有意义里程碑后立即更新 workpad，例如复现完成、代码改动完成、验证运行、review feedback 已处理。
   - 不要让已完成工作在计划里保持未勾选。
   - 如果 ticket 从 `Todo` 开始且已经有 PR，kickoff 后立刻执行完整 PR feedback sweep，再做新 feature work。
5. 运行当前 scope 必需的验证和测试。
   - 强制门禁：执行 ticket 提供的所有 `Validation`、`Test Plan` 或 `Testing` 要求；未满足即视为未完成。
   - 优先使用能直接证明改动行为的 targeted proof。
   - 可以使用临时本地 proof edit 来验证假设，例如临时调整 build input 或本地 hardcode 某个 UI account/response path，但必须在 commit/push 前全部恢复。
   - 把临时 proof 步骤和结果记录到 workpad 的 `Validation` 或 `Notes`，让 reviewer 可复核。
   - 如果触及 app，运行 `launch-app` 验证，并在 handoff 前通过 `github-pr-media` 捕获或上传媒体。
6. 重新检查全部 acceptance criteria，补齐 gap。
7. 每次 `git push` 前，先运行当前 scope 要求的验证并确认通过；如果失败，修复并重跑直到绿色，然后 commit 并 push。
8. 把 PR URL 关联到 issue。优先使用 attachment；如果 attachment 不可用，再写入 workpad。
   - 确保 GitHub PR 有 `symphony` label。
   - PR 标题和正文必须是中文；如果仓库没有 PR 模板，按 `.codex/skills/push/SKILL.md` 的中文 fallback 结构生成。
9. 把最新 `origin/main` merge 到当前分支，解决冲突并重跑检查。
10. 更新 workpad comment 的最终 checklist 和 validation notes。
    - 勾选已完成的 plan、acceptance、validation 项。
    - 在同一个 workpad comment 中加入最终 handoff notes，包括 commit 和 validation summary。
    - 不要把 PR URL 写进 workpad；PR 关联应放在 issue attachment/link 字段。
    - 如果执行中有任何不清楚的地方，在底部添加简短 `### Confusions` section。
    - 不要额外发布 completion summary comment。
11. 移动到 `Human Review` 前，轮询 PR feedback 和 checks：
    - 读取 PR `Manual QA Plan` comment（如有），据此强化 UI/runtime test coverage。
    - 执行完整 PR feedback sweep。
    - 确认最新改动后的 PR checks 全部通过。
    - 确认 ticket 提供的所有 validation/test-plan 项都在 workpad 中明确标记完成。
    - 持续 check-address-verify，直到没有 outstanding comments 且 checks 完全通过。
    - 状态切换前重新打开并刷新 workpad，让 `Plan`、`Acceptance Criteria`、`Validation` 与已完成工作完全一致。
12. 只有此时才移动 issue 到 `Human Review`。
    - 例外：如果按 blocked-access escape hatch 被非 GitHub 工具或 auth 阻塞，可以带 blocker brief 和明确解锁动作移动到 `Human Review`。
13. 如果 `Todo` ticket 在 kickoff 时已挂 PR：
    - 确保所有已有 PR feedback 都已审阅并解决，包括 inline review comments；解决方式可以是代码修改，也可以是明确且有理由的 pushback。
    - 确保分支已 push 所需更新。
    - 然后移动到 `Human Review`。

## Step 3：Human Review 与 merge 处理

1. 当 issue 处于 `Human Review`，不要编码，也不要修改 ticket 内容。
2. 按需轮询更新，包括人类和 bot 的 GitHub PR review comments。
3. 如果 review feedback 要求改动，移动 issue 到 `Rework` 并执行 rework 流程。
4. 如果批准，人类会把 issue 移动到 `Merging`。
5. 当 issue 处于 `Merging`，读取 `agent.merge_policy`，打开并遵循对应 `.codex/skills/<skill>/SKILL.md`，直到落地完成或记录 blocker。
6. merge 完成后，移动 issue 到 `Done`。

## Step 4：Rework 处理

1. 把 `Rework` 当作完整方案重置，而不是增量补丁。
2. 重新阅读完整 issue body 和所有人工 comments；明确本次 attempt 要做出哪些不同处理。
3. 关闭当前 issue 绑定的旧 PR。
4. 删除 issue 上现有 `## Codex Workpad` comment。
5. 从 `origin/main` 创建新分支。
6. 从正常 kickoff flow 重新开始：
   - 如果当前 issue 状态是 `Todo`，移动到 `In Progress`；否则保持当前状态。
   - 创建新的 bootstrap `## Codex Workpad` comment。
   - 构建新的 plan/checklist，并端到端执行。

## 移动到 Human Review 前的完成门槛

- Step 1/2 checklist 全部完成，并准确反映在单一 workpad comment 中。
- Acceptance criteria 和 ticket 提供的必要 validation items 全部完成。
- 最新 commit 的 validation/tests 绿色。
- PR feedback sweep 完成，且没有 actionable comments。
- PR checks 绿色，分支已 push，PR 已关联到 issue。
- 必要 PR metadata 已存在，例如 `symphony` label。
- PR 标题、PR 正文和所有新增 GitHub 评论均为中文。
- 如果触及 app，runtime validation/media 要求已完成。

## Guardrails

- 如果当前分支 PR 已关闭或合并，不要复用该分支或之前实现状态。
- 对已关闭或合并的分支 PR，从 `origin/main` 创建新分支，并像全新任务一样从复现和计划重启。
- 如果 issue 状态是 `Backlog`，不要修改它；等待人类移动到 `Todo`。
- 不要编辑 issue body/description 来记录计划或进度。
- 每个 issue 只使用一个持久 workpad comment：`## Codex Workpad`。
- 如果 session 内无法通过 `linear_graphql` 编辑 comment，先记录 blocker；不要调用 Linear MCP/app 工具兜底。
- 临时 proof edit 只允许用于本地验证，commit 前必须恢复。
- 如果发现超出范围的改进，创建单独 Backlog issue，不要扩大当前 scope；该 issue 要有清晰标题、描述、验收标准、同 project 归属、与当前 issue 的 `related` 链接，并在依赖当前 issue 时设置 `blockedBy`。
- 未达到 `Human Review` 完成门槛前，不要移动到 `Human Review`。
- 在 `Human Review` 中不要修改代码；等待并轮询。
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
