# Symphony Issue Run Optimization Log

本文件记录 `symphony-issue-run` 流程每次保留下来的优化点。每条记录必须能回答：
这次卡在哪里、证据是什么、改了 Skill / Workflow / 代码的哪一层、以及怎么验证。

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
  - `internal/workflow/workflow_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - 通过：`GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/workflow`
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
  - `internal/workflow/workflow_test.go`
  - `docs/architecture/symphony-go-architecture.md`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - `GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/workflow`
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
  - `internal/orchestrator/agent_session.go`
  - `internal/orchestrator/orchestrator_test.go`
  - `docs/optimization/symphony-issue-run.md`
- Validation:
  - `GOROOT=/Users/yibeikongqiu/sdk/go1.22.12 GOCACHE=/private/tmp/symphony-go-gocache GO=/Users/yibeikongqiu/sdk/go1.22.12/bin/go ./test.sh ./internal/orchestrator`
- Follow-up: 重建 `bin/symphony-go` 后继续跑 `ZEE-74`，验证 `AI Review -> Merging -> Done` 是否真的走通。

## 2026-05-02 16:36 +08 - ZEE-41

- Trigger: 用户指出 `ZEE-41` 耗时过长，状态流经过 `Human Review`，且最终由父会话手工接管 `Merging`，不符合 `symphony-go` 全自动处理目标。
- Evidence:
  - `.symphony/logs/run-20260502-155913.human.log` 多次记录 `response timeout waiting for id=2`，导致第一轮 listener 在 Codex thread handshake 前反复失败。
  - `.symphony/logs/run-20260502-161318.human.log` 记录 `16:23:43` child 在 `Merging` 执行 `git merge --no-ff` 时失败，错误为 `fatal: cannot create directory at 'docs/architecture': Operation not permitted`。
  - 同一日志 `16:24:41` 记录 child 按 blocker 规则把 issue 移到 `Human Review`；`16:26:04` 后 orchestrator 又按 commit handoff 记录 `In Progress -> AI Review -> Merging`，造成 Linear 面板看起来像多段重复流转。
  - 当前 `WORKFLOW.md` 同时要求 child 在实现完成后移动到 `AI Review`，而 `internal/orchestrator/orchestrator.go` 也会在 turn 结束后基于 HEAD 变化执行同一 handoff，存在状态 owner 重叠。
  - `internal/codex/runner.go` 只把 issue worktree 和 git metadata 加进 `workspaceWrite.writableRoots`；`Merging` prompt 要写 repo root 的 `main` checkout，权限边界不匹配。
- Optimization:
  - Workflow 层：明确 `In Progress` / `Rework` agent turn 只提交、验证和写 workpad handoff，不自行切 `AI Review` / `Merging`；状态推进由 orchestrator 统一负责。
  - 代码层：仅当 issue state 为 `Merging` 时，把 git common-dir 对应的主 checkout root 加入 Codex turn writable roots，让 child 可以自己完成 local main merge、验证和 push。
  - 流程层：把这次用户纠正写入 `lesson.md`，后续 `symphony-issue-run` 不能把父会话手工 merge 当成正常成功路径；若需要人接管，必须先分类为 framework gap 并优化。
- Files:
  - `WORKFLOW.md`
  - `internal/codex/runner.go`
  - `internal/codex/runner_test.go`
  - `docs/optimization/symphony-issue-run.md`
  - `lesson.md`
- Validation:
  - `git diff --check`
  - `./test.sh ./internal/codex ./internal/orchestrator`
  - `make build`
- Follow-up: 下次用真实 issue-run 验证 `Merging` 不再进入 `Human Review`；如果仍出现 root merge 卡点，应把 local merge 下沉为 orchestrator first-class action，而不是继续依赖 prompt 执行。

## 2026-05-02 16:05 +08 - ZEE-41

- Trigger: `ZEE-41` issue-scoped listener created the worktree but repeatedly failed before starting the Codex turn.
- Evidence: `.symphony/logs/run-20260502-155913.human.log` recorded `response timeout waiting for id=2`; `internal/codex/runner.go:279` sends `thread/start` with id 2; `internal/config/config.go:94` defaulted `codex.read_timeout_ms` to 5000; current `WORKFLOW.md` did not override it.
- Optimization: Set `codex.read_timeout_ms: 60000` in `WORKFLOW.md` so the app-server startup/thread handshake has enough room in real unattended runs.
- Files: `WORKFLOW.md`, `docs/optimization/symphony-issue-run.md`.
- Validation: `git diff --check`; `./test.sh ./internal/config ./internal/workflow`; `make build`.
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
  - 通过：`go test ./internal/config ./internal/orchestrator ./internal/workflow ./internal/types`
  - 通过：`CGO_ENABLED=0 go test ./...`
- Follow-up:
  - 下一次真实 issue-run 后，用该 issue 的 human log、JSONL log、Linear workpad 和 git evidence 追加一条运行级复盘记录。
