# Symphony Issue Run Optimization Log

本文件记录 `symphony-issue-run` 流程每次保留下来的优化点。每条记录必须能回答：
这次卡在哪里、证据是什么、改了 Skill / Workflow / 代码的哪一层、以及怎么验证。

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
