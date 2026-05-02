# Symphony Issue Run Optimization Log

本文件记录 `symphony-issue-run` 流程每次保留下来的优化点。每条记录必须能回答：
这次卡在哪里、证据是什么、改了 Skill / Workflow / 代码的哪一层、以及怎么验证。

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
