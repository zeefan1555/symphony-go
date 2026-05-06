# 领域文档

本文件说明工程技能在探索代码库时应如何消费本仓领域文档。

## 布局

本仓是单上下文仓库（single-context repo）。

## 探索前先读这些

- 根目录 `CONTEXT.md`，如果存在。
- `SPEC.md`，用于理解本仓产品与架构契约。
- `docs/contract-scope.md`，用于确认本仓实现边界、非目标和可选 operator surface。
- `WORKFLOW.md`，用于理解当前 Linear 驱动的执行工作流。
- `docs/runtime-policy.md`，用于理解 approval、sandbox、user-input-required 和 dynamic tool 策略。
- `docs/adr/`，用于读取架构决策，如果存在。
- 排查重复工作流失败或用户纠正时，读取 `docs/optimization/` 和 `lesson.md`。
- 不要把 `docs/architecture/symphony-go-architecture.md` 作为默认参考源；它是历史实验文档，不是当前权威架构说明。
- `docs/plan/` 和 `docs/superpowers/plans/` 是历史计划归档；除非任务明确要求复盘旧计划，不要把其中的旧目录名或旧 import 示例当作当前权威结构。

如果某个可选文件不存在，静默继续；除非当前任务确实需要，否则不要主动建议创建。

## 使用本仓术语

输出中提到领域概念时，优先使用 `SPEC.md`、`WORKFLOW.md`、`CONTEXT.md` 和 `lesson.md` 里的术语。不要为工作流状态、workpad、issue worktree 或 Linear 交接行为另造名称。

## 标出冲突

如果某个建议和 `SPEC.md`、`WORKFLOW.md`、ADR 或 `AGENTS.md` 冲突，先明确指出冲突，再提出修改方案。
