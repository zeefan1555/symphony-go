# Triage 标签

技能使用五个标准 triage 角色表达 issue 状态。本文件把这些角色映射到本仓 issue 跟踪系统实际使用的标签字符串。

| mattpocock/skills 中的标签 | 本仓跟踪系统中的标签 | 含义 |
| -------------------------- | -------------------- | ---- |
| `needs-triage`             | `needs-triage`       | 需要维护者评估 |
| `needs-info`               | `needs-info`         | 等待反馈者补充信息 |
| `ready-for-agent`          | `ready-for-agent`    | 信息完整，可交给无人值守 agent 处理 |
| `ready-for-human`          | `ready-for-human`    | 需要人类实现 |
| `wontfix`                  | `wontfix`            | 不会处理 |

这些是 triage 标签，不是 Linear 执行状态。不要把它们映射到 `Todo`、`In Progress`、`AI Review`、`Merging`、`Rework` 或 `Done` 这类 workflow 状态。
