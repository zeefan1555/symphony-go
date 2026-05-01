# Zh Smoke Harness Experiments

这个文件是 `zh-smoke-harness` 的人类可读实验账本。每一轮都记录：

- 本轮优化了哪个通用框架能力。
- 各维度指标是否符合预期。
- 本轮代码改动和验证证据。
- 下一轮只尝试一个最值得验证的通用优化点。

机器可读明细见同目录 `results.tsv`。

## 维度说明

| 维度 | 看什么 |
| --- | --- |
| 正确性 | `success`、`final_state`、`changed_files`、`ai_review_result` |
| Token | `last_total_tokens` 优先；`total_tokens` 只表示累计消耗 |
| 速度 | `turn_duration_ms`、`total_flow_ms`、`dispatch_latency_ms`、`post_turn_handoff_ms`、`ai_review_ms`、`merge_state_ms` |
| Agent 活动密度 | `token_events`、`command_events`、`plan_updates`、`diff_updates` |
| Issue 评论动作 | `workpad_updates`、`workpad_phases`、`workpad_bytes`、`workpad_lines` |
| TUI 可读性 | 是否有语义颜色、是否隐藏 raw JSON、状态/Backoff 是否一眼可读 |
| 结构化与审美 | Workpad、实验记录、最终摘要是否短、分区清楚、可 review |

## 实验记录

| 时间 | Issue | 优化动作 | 状态 | 成功 | Token | Events | Flow ms | Turn ms | AI Review | Merge ms | Workpad | 文件 | 日志 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2026-05-01T19:58:08 | ZEE-17 | 基线：AI Review 自动流转与本地 merge | Done | true | 241220 (last 35274, events 7) | tok 7, cmd 11, plan 0, diff 6 | 64153 | 48514 | passed | 117 | 0 | SMOKE.md | `.symphony/logs/run-20260501-195659.jsonl` |
| 2026-05-01T20:00:52 | ZEE-18 | 缩短 zh-smoke workflow prompt | Done | true | 491239 (last 36427, events 14) | tok 14, cmd 13, plan 5, diff 10 | 87161 | 71452 | passed | 90 | 0 | SMOKE.md | `.symphony/logs/run-20260501-195920.jsonl` |
| 2026-05-01T20:07:03 | ZEE-19 | 修正 token 口径并隔离每轮新进程/新日志 | Done | true | 410279 (last 35293, events 12) | tok 12, cmd 10, plan 5, diff 9 | 88175 | 74399 | passed | 95 | 0 | SMOKE.md | `.symphony/logs/run-20260501-200530.jsonl` |
| 2026-05-01T20:39:35 | ZEE-20 | 验证 workpad 更新结构化指标 | Done | true | 411033 (last 35310, events 12) | tok 12, cmd 10, plan 4, diff 9 | 72020 | 56807 | passed | 93 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-203818.jsonl` |
| 2026-05-01T20:43:53 | ZEE-21 | 修正 Markdown 指标行插入位置 | Done | true | 447886 (last 35726, events 13) | tok 13, cmd 11, plan 5, diff 9 | 85475 | 70552 | passed | 64 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-204223.jsonl` |
| 2026-05-01T20:50:34 | ZEE-22 | Workpad 计划逐步打勾 | Done | true | 457526 (last 36869, events 18) | tok 18, cmd 27, plan 5, diff 10 | 83843 | 70147 | passed | 84 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-204906.jsonl` |
| 2026-05-01T20:58:43 | ZEE-23 | Workpad 任务计划具体化 | Done | true | 420971 (last 36299, events 12) | tok 12, cmd 11, plan 5, diff 0 | 75433 | 60402 | passed | 81 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-205723.jsonl` |
| 2026-05-01T21:13:30 | ZEE-24 | Spec 对齐：彩色日志 level 与 Spec gap skill | Done | true | 205508 (last 35188, events 6) | tok 6, cmd 8, plan 0, diff 5 | 66236 | 50226 | passed | 72 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-211219.jsonl` |
| 2026-05-01T21:19:25 | ZEE-25 | Spec 对齐：持久化 human log 并聚焦 info 以上 | Done | true | 452398 (last 36219, events 13) | tok 13, cmd 12, plan 5, diff 10 | 83154 | 67701 | passed | 106 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-211757.jsonl` |
| 2026-05-01T21:22:57 | ZEE-26 | Spec gap 验证：检查持久化 human log 可读性 | Done | true | 451050 (last 36113, events 13) | tok 13, cmd 12, plan 5, diff 9 | 79604 | 66197 | passed | 91 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-212134.jsonl` |
| 2026-05-01T21:36:12 | ZEE-27 | human log 记录 Workpad 进度与 Codex turn 细节 | Done | true | 240235 (last 35411, events 7) | tok 7, cmd 10, plan 0, diff 5 | 75978 | 61890 | passed | 98 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-213452.jsonl` |
| 2026-05-01T21:40:14 | ZEE-28 | human log 增加 Workpad 评论摘要 | Done | true | 312467 (last 36336, events 9) | tok 9, cmd 11, plan 0, diff 7 | 69868 | 54401 | passed | 94 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-213900.jsonl` |
| 2026-05-01T21:52:48 | ZEE-29 | human log 增加 hook 生命周期并分层 AI debug | Done | true | 277114 (last 36360, events 14) | tok 14, cmd 25, plan 1, diff 6 | 72710 | 58703 | passed | 142 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-215132.jsonl` |
| 2026-05-01T21:55:55 | ZEE-30 | human log 将上下文读取降为 DEBUG | Done | true | 387295 (last 36480, events 11) | tok 11, cmd 9, plan 5, diff 0 | 73179 | 59136 | passed | 89 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-215438.jsonl` |
| 2026-05-01T21:58:50 | ZEE-31 | human log 隐藏成功 cleanup 噪声 | Done | true | 319658 (last 37037, events 9) | tok 9, cmd 13, plan 0, diff 7 | 74856 | 58653 | passed | 90 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-215731.jsonl` |
| 2026-05-01T22:12:09 | ZEE-32 | human log 从 after_create 开始 | Done | true | 414458 (last 35846, events 12) | tok 12, cmd 11, plan 5, diff 9 | 78038 | 63303 | passed | 138 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-221046.jsonl` |
| 2026-05-01T22:23:04 | ZEE-33 | human log 增加 issue 分段 | Done | true | 381432 (last 36136, events 11) | tok 11, cmd 12, plan 4, diff 8 | 77043 | 61616 | passed | 120 | 4 (initial,handoff,ai_review,merge_completed) | SMOKE.md | `.symphony/logs/run-20260501-222142.jsonl` |

## 轮次复盘

### ZEE-17

- 本轮目标：建立固定中文冒烟基线，验证 `Todo -> In Progress -> AI Review -> Merging -> Done` 可以全自动闭环。
- 优化判断：正确性通过，`changed_files=SMOKE.md`，`ai_review_result=passed`，本地 merge 成功。
- 主要问题：当时指标只看累计 `total_tokens`，无法区分单次上下文 token 和事件累计 token。
- 证据：log `.symphony/logs/run-20260501-195659.jsonl`，commit `07ea5bd`。

### ZEE-18

- 本轮目标：缩短 issue 内 workflow prompt，观察 token 与耗时是否下降。
- 优化判断：没有达到预期；`last_total_tokens` 与基线接近，但 `token_events` 从 7 增至 14，`turn_duration_ms` 变慢。
- 主要问题：慢点不在 merge，而在 Codex turn 内部的计划、diff 和命令事件密度。
- 证据：log `.symphony/logs/run-20260501-195920.jsonl`，commit `586f5db`。

### ZEE-19

- 本轮目标：修正指标口径，确保每轮新开 Symphony Go 进程，并只解析本轮新日志。
- 优化判断：指标可信度提升；`last_total_tokens=35293` 接近基线，证明只看 `total_tokens` 会误判。
- 主要问题：历史日志没有 `workpad_updated` 事件，所以 comment 动作维度暂时为 0；下一轮新日志会开始记录。
- 证据：log `.symphony/logs/run-20260501-200530.jsonl`，commit `cc2802a`。

## 下一轮候选

优先继续做日志人类可读性的收敛，而不是关闭 memory，也不要只停在 SPEC 的最低字段要求：

- human log 已能从当前 issue 的 `after_create` 开始；下一步可以补 issue section header 或 summary footer，让它更像一份分段处理报告。
- 保留 memory lookup；上下文读取已经降为 `DEBUG`，后续重点看 INFO 主线是否足够像一份可复盘的处理流水账。
- 对同一 turn 内重复的 `codex_diff` 摘要做轻量去重，作为次优先级的扫描噪声优化。
