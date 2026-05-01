---
name: spec-gap-harness
description: "SPEC 场景差距驱动的 Symphony Go 优化 harness：按场景构造 workflow/issue，运行 smoke，对比 SPEC.md、代码和日志，每轮只替代一个指标、维度或场景并做最小修复。Use when user asks for Spark/SPEC conformance, scenario-driven workflow tests, spec gap loop, or optimize Symphony Go against SPEC.md."
---

# Spec Gap Harness

这个 skill 用来把 Symphony Go 的 smoke 优化从“看一组泛化指标”升级为“按 `SPEC.md`
场景逐项逼近规范”。

核心思想：`SPEC.md` 是目标合同；每一轮选择一个具体场景，构造或调整对应的
`WORKFLOW.md` 和测试 issue，跑真实流程，然后用代码、日志、运行结果对照该场景的
规范要求。下一轮只替代一个指标、一个维度，或一个场景，保证改动可归因、可验证。

## 成功标准

- 每轮都有明确的 `scenario_id`，对应 `SPEC.md:line` 或章节。
- 每轮只优化一个目标：一个指标、一个维度，或一个 SPEC 场景。
- workflow 和 issue 可以按场景手动修改；skill 要记录实际使用的版本和差异。
- 跑完后必须对比 `SPEC.md`、当前代码、最新日志和实验记录，形成 gap table。
- 修改代码前先选出一个最小可落地 gap；不要全量重构或顺手修多个问题。
- 修改后必须重新编译，并停掉旧进程后重新启动新一轮运行。
- 结论必须有证据：`SPEC.md:line`、代码 `file:line`、日志路径/事件、命令输出。

## 固定资产

- 规范文档：`SPEC.md`
- Go 实现：当前仓库根目录
- workflow：`WORKFLOW.md` 或本轮显式指定的 workflow 文件
- smoke 运行日志：`.symphony/logs/run-*.jsonl`
- zh-smoke 实验记录：`.codex/skills/zh-smoke-harness/experiments/rounds.md`
- Spec gap 人类记录：`.codex/skills/spec-gap-harness/experiments/rounds.md`
- Spec gap TSV：`.codex/skills/spec-gap-harness/experiments/gaps.tsv`
- 每轮运行总账：`.codex/skills/spec-gap-harness/experiments/run_log.tsv`
- SPEC 覆盖总账：`.codex/skills/spec-gap-harness/experiments/coverage.tsv`
- 原始运行日志：`.symphony/logs/run-*.jsonl` 和同名 `.human.log`

如果 `experiments/` 文件不存在，首次运行时创建。不要覆盖已有记录，只追加。

## 持久化记录

每轮信息必须落到固定文件，方便人工 review 和下一轮继续。

| 文件 | 用途 | 写入方式 |
| --- | --- | --- |
| `experiments/run_log.tsv` | 每轮总账：issue、场景、iteration、commit、workflow、日志、测试、结论、下一步 | 每轮追加 1 行 |
| `experiments/rounds.md` | 人类可读叙事：为什么测、怎么测、改了什么、证据是什么 | 每轮追加一个 `## Round ...` 段落 |
| `experiments/gaps.tsv` | 结构化 gap table：SPEC 要求、证据、verdict、next_fix、validation | 每个 gap 追加 1 行 |
| `experiments/coverage.tsv` | SPEC 覆盖 ledger：章节是否 untested/partial/covered | 每轮更新对应 `spec_range` 行 |
| `.symphony/logs/run-*.jsonl` | 机器可解析原始运行证据 | 运行自动生成，记录路径 |
| `.symphony/logs/run-*.human.log` | 人工可读时间线 | 运行自动生成，记录路径 |

`run_log.tsv` 表头：

```tsv
round_id	timestamp	issue	scenario_id	iteration	replace_kind	old_signal	new_spec_signal	workflow_path	workflow_ref	commit_before	commit_after	run_jsonl	human_log	tests	verdict	next_fix	next_scenario
```

字段约束：

- `round_id` 使用 `<issue>-r<iteration>` 或 `<date>-<scenario>-r<iteration>`。
- `workflow_ref` 写 commit、diff 摘要或临时 workflow 路径；不能只写“见上文”。
- `commit_before` 是修复前 HEAD；`commit_after` 是本轮保留后的 HEAD，没有代码改动写 `none`。
- `tests` 写精确命令和结果摘要，例如 `go test ./...: pass; make build: pass`。
- `next_fix` 必须是下一轮可直接执行的最小动作。
- `next_scenario` 必须能映射到场景矩阵；如果继续同一场景，写同一个 `scenario_id`。

`coverage.tsv` 使用“更新行”语义：同一 `spec_range` 只保留最新状态。若无法安全原地更新，
先追加到 `rounds.md` 的 `Coverage ledger 更新`，再人工整理。

## 场景矩阵

优先从 `SPEC.md` 的验证矩阵和核心章节抽场景。不要一次审完整份 SPEC。

| `scenario_id` | SPEC 范围 | 典型 workflow/issue 构造 | 主要观察点 |
| --- | --- | --- | --- |
| `contract_scope` | `1`, `2`, `3`, `18` | 不跑 issue；读取代码结构、README、CLI help、配置入口 | 实现边界是否仍是 long-running automation service，是否避免非目标功能扩张 |
| `domain_model` | `4`, `4.2` | synthetic issue/run attempt/session/retry 数据 | stable identifiers、workspace key、state normalization、runtime state 字段 |
| `workflow_config` | `5`, `6`, `17.1` | 改 workflow front matter、路径、非法 YAML、动态 reload | reload 是否生效，错误是否阻断新 dispatch 且保留 last known good |
| `workspace_safety` | `9`, `15.2`, `17.2` | 构造带特殊字符的 issue identifier、相对 workspace root、hooks | workspace key、root containment、cwd、hook 失败语义 |
| `tracker_selection` | `4.1.1`, `8.2`, `11`, `17.3` | 构造 active/terminal/non-active/blocker issue | candidate fetch、state normalization、Todo blocker、project scope |
| `orchestrator_state` | `7`, `8`, `14`, `17.4` | 多 issue、并发限制、失败重试、状态漂移 | claimed/running/retry/completed 是否单一权威且不重复 dispatch |
| `agent_runner` | `10`, `12`, `17.5` | 控制 prompt、continuation、approval/input-required、工具调用 | app-server cwd、prompt 渲染、continuation、fail-fast/stall 行为 |
| `observability` | `13`, `17.6` | 运行成功、失败、retry、reload、hook 失败 | structured log 字段、人类可读摘要、token/session/runtime 统计 |
| `security_ops` | `15`, `18.3` | 构造 token/env、恶意 workspace key、hook 输出、日志检查 | secret 不泄露、权限边界、trusted environment 假设和 hardening 文档 |
| `cli_lifecycle` | `16`, `17.7`, `18` | 显式 workflow 路径、缺省路径、startup cleanup、once run | CLI 错误面、启动验证、terminal workspace cleanup |
| `real_integration` | `17.8`, `18.3` | 隔离测试 issue 和 workspace，真实 Linear + Codex | 真实外部依赖下是否可复现且清理边界清楚 |
| `ssh_worker_optional` | `Appendix A` | 只在实现 SSH worker 时启用；构造 host/workspace 调度场景 | remote workspace、host capacity、host failure、cleanup observability |

选场景规则：

1. 用户指定场景时优先用户场景。
2. 没有指定时，从上一轮 gap table 中最高优先级且最小可验证的场景开始。
3. 如果旧指标已经无法解释 SPEC 差距，选择一个 SPEC 场景替代该指标。
4. 如果场景过大，拆成一个可单轮验证的子维度，例如 `observability.session_context_fields`。
5. `Appendix A` 是可选扩展；只有代码实现或用户要求 SSH worker 时才进入 `ssh_worker_optional`。

## SPEC 覆盖 Ledger

每次开始前维护一个轻量覆盖表，避免误以为“跑通 smoke”就等于覆盖了整份 SPEC。

```tsv
spec_range	scenario_id	status	last_round	evidence	next_gap
1-3	contract_scope	untested|partial|covered	-	-	-
4	domain_model	untested|partial|covered	-	-	-
5-6	workflow_config	untested|partial|covered	-	-	-
7-8	orchestrator_state	untested|partial|covered	-	-	-
9	workspace_safety	untested|partial|covered	-	-	-
10	agent_runner	untested|partial|covered	-	-	-
11	tracker_selection	untested|partial|covered	-	-	-
12	agent_runner	untested|partial|covered	-	-	-
13	observability	untested|partial|covered	-	-	-
14	orchestrator_state	untested|partial|covered	-	-	-
15	security_ops	untested|partial|covered	-	-	-
16	cli_lifecycle	untested|partial|covered	-	-	-
17-18	real_integration	untested|partial|covered	-	-	-
Appendix A	ssh_worker_optional	not_applicable|untested|partial|covered	-	-	-
```

`covered` 只表示该范围已有当前证据支持，不表示永久完成；代码或 workflow 变更后可以降回
`partial`。

## 三轮迭代协议

结合 Darwin skill 的思路，每次验证同一个 SPEC signal，连续迭代三轮。三轮不是固定要修
三个不同问题，而是对同一个目标执行“基线、修复、复验/泛化”。

| 轮次 | 目标 | 允许改动 | 必须产物 |
| --- | --- | --- | --- |
| Round 1 Baseline | 选定一个 `scenario_id` 和一个 SPEC signal，跑出现状 | workflow/issue 构造可以改；代码只读，除非无法运行 | baseline 日志、gap table、一个 selected fix |
| Round 2 Patch | 只修 Round 1 selected fix | 最小代码改动；必要时同步最小测试 | 测试、构建、新日志、同一 signal 的前后对比 |
| Round 3 Ratchet | 用同一 signal 复验，必要时换一个等价 issue/workflow 泛化 | 只允许补测试、记录或极小修正 | keep/revert 决策、coverage ledger 更新、下一场景候选 |

Darwin-style keep/revert 判定：

- 如果 Round 2/3 让 SPEC signal 更接近规范，保留并记录 `keep`。
- 如果改动只改善旧指标但没有改善 SPEC signal，记录 `revert_candidate`，不要继续基于它扩展。
- 如果出现回归，优先用反向最小补丁修复；只有用户要求时才做 git revert。
- 每轮结束都要写 `old_signal`、`new_spec_signal` 和证据，避免自己评自己时漂移。

## 每轮输入

每轮开始前先写清楚：

```text
Round goal: <本轮替代的指标/维度/场景>
Iteration: 1|2|3
Scenario: <scenario_id 或自定义子场景>
SPEC anchors: SPEC.md:<line> ...
Workflow under test: <path>
Issue under test: <Linear issue key/id 或 synthetic issue 描述>
Code anchors: <go/file:line ...>
Success signal: <日志事件、状态变化、测试断言或代码行为>
Stop condition: <本轮不继续扩展的边界>
```

如果 workflow 或 issue 需要手动修改，先记录修改意图，再执行运行。workflow 可以是正式
`WORKFLOW.md`，也可以是一个场景专用临时 workflow；但临时 workflow 必须写入记录，避免
后续无法复现。

## 每轮流程

### 1. 选择一个可替代目标

本轮必须只替代以下一种：

- `metric`：旧 smoke 指标不足，例如 token、耗时、command 数，替换成 SPEC 可观察要求。
- `dimension`：某个横向维度，例如 observability、人类 handoff、state reconciliation。
- `scenario`：一个 SPEC 场景，例如 workflow dynamic reload 或 workspace safety。

选择后把它写成：

```text
Replace: metric|dimension|scenario
Old signal: <旧指标或旧判断方式；没有则写 none>
New SPEC signal: <来自 SPEC 的要求和可观察证据>
Three-round plan: baseline -> patch -> ratchet
```

### 2. 构造场景 workflow 和 issue

先决定 issue 是否必须真实存在：

- 需要验证 Linear 查询、状态流转、评论、workpad、handoff 时，使用真实隔离 issue。
- 只验证 workflow parsing、config、workspace path、prompt rendering、CLI 错误面时，优先使用
  synthetic issue 或 config-only startup check。
- 如果真实 issue 会产生外部写入，必须记录 issue key、初始状态、预期结束状态和清理方式。

workflow 构造原则：

- workflow 要最小化，只包含验证该场景所需的 front matter 和 prompt。
- 允许手动修改 workflow；修改后记录 `git diff -- <workflow>` 或复制关键片段。
- 不为单个 smoke issue 写死代码；issue-specific 内容只能放在 workflow/prompt/测试数据里。
- 如果本轮验证 dynamic reload，必须保留旧进程运行，再改 workflow；如果验证代码修复，必须停旧进程后重新启动。

### 3. 运行基线并收集证据

常用命令：

```bash
cd /Users/bytedance/symphony-go
git status --short
git diff --check
make build
make run
```

如果已有常驻进程，先确认是不是旧轮次残留：

```bash
ps -ef | rg 'symphony-go|codex app-server' | rg -v rg
```

需要重启验证代码改动时，停掉旧的 `symphony-go` 或旧 `codex app-server`，再启动新的二进制。
不要把旧进程日志当成新代码证据。

抽取最新日志：

```bash
latest=$(ls -t .symphony/logs/run-*.jsonl | head -n 1)
rg -n '"event":"(state_changed|workflow_|workspace_|dispatch_|retry_|reconcile_|workpad_|ai_review_|local_merge_|.*error|.*failed)"' "$latest"
```

如果本轮关注人类可读性，也要观察 TUI/console/human log，而不只看 JSONL。

### 4. 对比 SPEC、代码和运行结果

每条结论必须同时回答：

- SPEC 要求是什么：`SPEC.md:<line>`。
- 当前代码在哪里实现或缺失：`go/<file>:<line>`。
- 本轮运行是否体现该行为：日志路径、事件、issue 状态或命令输出。
- 这个证据对应 `conforms`、`partial`、`missing` 还是 `unclear`。

不要把“流程跑完了”当作符合 SPEC。必须回到具体字段、状态、错误面、hook 语义、
workspace 边界、prompt 输入或日志上下文。

### 5. 建立 Gap Table

TSV 字段：

```tsv
round	scenario_id	replace_kind	spec_section	requirement	evidence_kind	evidence	verdict	gap	impact	next_fix	validation
```

每条 gap 的文本格式：

```text
Scenario: <scenario_id>
Replace: metric|dimension|scenario - <本轮替代目标>
SPEC: SPEC.md:<line> <requirement>
Evidence: <file:line / log event / issue state / command output>
Verdict: conforms|partial|missing|unclear
Gap: <当前实现和 SPEC 的具体差距>
Impact: <对正确性、操作员判断或调试的影响>
Next fix: <下一步最小代码或 workflow 改动>
Validation: <修完后如何证明更接近 SPEC>
```

优先级排序：

1. `missing` 且影响 correctness、安全边界、状态机或外部写入边界。
2. `missing` 或 `partial` 且导致操作员无法判断运行状态。
3. `partial` 且已有日志/代码证据证明字段、状态、错误面不稳定。
4. `unclear` 且可通过补日志、测试或场景 workflow 快速澄清。
5. `conforms` 只记录，不作为下一轮优化目标。

### 6. 选择一个最小修复

选择规则：

- 只修本轮最高优先级 gap 中最小可验证的一项。
- 能解释为通用框架能力，不为单个 smoke issue 写死。
- 能用测试、构建和下一轮场景 smoke 证明。
- 不关闭有用能力；如果能力影响调试，应补可见性、配置或错误面。
- 如果发现更小的 workflow/issue 构造即可验证，不先改代码。

修改前写一句：

```text
Selected fix: <file/function or workflow section>
Why minimal: <为什么这是最小通用修复>
Verification: <测试命令 + 场景 smoke 观察>
```

### 7. 修改、编译、重启、复跑

常用验证：

```bash
cd /Users/bytedance/symphony-go
go test -ldflags='-linkmode=external' ./...
make build
git diff --check
```

如果改了 Python 辅助脚本：

```bash
python3 -m py_compile scripts/smoke_metrics.py scripts/zh_smoke_round.py
```

复跑要求：

- 停掉旧 `symphony-go` / `codex app-server` 后再启动新二进制。
- 使用同一个 `scenario_id` 的 workflow/issue 再跑一轮。
- 对比新旧日志中同一个 SPEC signal，而不是换一个无法比较的指标。
- 如果场景需要 dynamic reload，记录 reload 前后 workflow 片段和日志事件。

### 8. 记录本轮

每轮结束必须先写记录，再开始下一轮。记录顺序：

1. 追加 `experiments/run_log.tsv`：一行总账，串起 issue、commit、workflow、日志和下一步。
2. 追加 `experiments/rounds.md`：人类可读过程和判断。
3. 追加 `experiments/gaps.tsv`：本轮发现的每个 gap。
4. 更新或记录 `experiments/coverage.tsv`：本轮覆盖了哪些 SPEC 范围。
5. 确认 `.symphony/logs/run-*.jsonl` 和 `.human.log` 路径已经写进记录。

`rounds.md` 格式：

```markdown
## Round <date or issue>: <scenario_id> - <替代目标>

- **Round goal**：替代 <metric|dimension|scenario>，旧信号 `<old>`，新 SPEC 信号 `<new>`
- **Iteration**：1 baseline / 2 patch / 3 ratchet
- **Spec 范围**：`SPEC.md:<line>` ...
- **Workflow / Issue**：workflow 路径、关键 diff、issue key/id、初始/结束状态
- **本轮运行**：命令、日志路径、进程重启说明
- **Commit**：`before=<sha>`，`after=<sha|none>`，commit message 或未提交原因
- **Gap table**：列出 verdict / gap / next_fix
- **已修复内容**：本轮只写一个最小修复；若未改代码，写明原因
- **验证结果**：测试、构建、场景 smoke 结果
- **Darwin 判定**：keep / revert_candidate / needs_more_evidence
- **Coverage ledger 更新**：哪些 `spec_range` 从 untested 变 partial/covered
- **下一轮候选**：一个最高优先级 SPEC gap 或下一个场景
```

提交规则：

- 如果本轮修改代码并验证通过，创建本地 commit 后再写 `commit_after`。
- 如果本轮只改 workflow/issue 或只做 baseline，不强制 commit，但必须在 `commit_after` 写 `none`，并说明原因。
- 如果验证失败，不要假装完成；`verdict` 写 `failed` 或 `needs_more_evidence`，`next_fix` 写下一步最小动作。
- 下一轮开始前，先读 `run_log.tsv` 最后一行和 `rounds.md` 最新段落，确认 continuation point。

## 与 zh-smoke-harness 的关系

- `zh-smoke-harness` 可以负责跑一轮和收集通用数据。
- `spec-gap-harness` 只负责把这些数据解释成 SPEC 场景差距，并决定下一轮替代哪个指标、
  维度或场景。
- 不要继续用 token、耗时、命令数作为主优化目标；它们只能作为辅助解释。

## 反模式

- 不要全量扫 `SPEC.md` 后一次性修很多 gap。
- 不要只看最新日志，不回到本轮场景和 SPEC anchor。
- 不要用“run completed”替代字段、状态、错误面、workspace 边界等具体证据。
- 不要把 JSONL 当作唯一人类展示；结构化日志和可读展示应分层。
- 不要在 orchestrator 中写死某个 Linear issue 的业务规则。
- 不要让旧进程、旧日志或旧 workflow 混入新一轮验证。
- 不要因为一次真实 issue 跑通，就认为 synthetic/config-only 场景也符合 SPEC。
