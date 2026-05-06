# Spec Gap Harness Rounds

本文件记录每轮 `SPEC.md` 差距驱动优化。每轮只选择一个通用框架 gap 修复，并用代码、
日志、测试或 smoke 结果证明更接近 Spec。

## Round ZEE-26: 持久化 Human Log 可读性验证

- **Spec 范围**：`SPEC.md:1249-1274`、`SPEC.md:1296-1302`、`SPEC.md:2086-2087`
- **Scenario**：`observability.human_log_persistence`
- **Replace**：`dimension`，用 SPEC observability 证据替代单纯 token/耗时指标判断。
- **本轮证据**：
  - `make zh-smoke-round CHANGE_NOTE="Spec gap 验证：检查持久化 human log 可读性"` 跑出 `ZEE-26`。
  - `go/.symphony/logs/run-20260501-212134.jsonl`：435 行，保留 debug 级 `codex_event`、`issue_id`、`issue_identifier`、`session_id` 等机器分析字段。
  - `go/.symphony/logs/run-20260501-212134.human.log`：12 行，只保留 `INFO` 关键时间线；没有 `codex_event`、`item/agentMessage/delta`、`\u003e` 噪音。
  - `go/internal/logging/jsonl.go:30-39` 定义结构化字段；`go/internal/logging/jsonl.go:115-139` 同时写 JSONL、human file 和 console sink；`go/cmd/symphony-go/main.go:117-122` 为每轮创建同名 human log。
- **Gap table**：
  - `conforms`：持久化 human log 与 JSONL 分层满足 operator-visible observability，且 human log 使用稳定 `key=value`。
  - `partial`：`SPEC.md:1273-1274` 要求 sink 失败时尽量继续运行并通过剩余 sink 告警；当前 human log 文件打开失败会让 `logging.New` 返回错误，随后 CLI fatal。
- **验证结果**：ZEE-26 `success=true`、`final_state=Done`、`changed_files=SMOKE.md`、`ai_review_result=passed`。
- **下一轮候选**：把 optional human sink 改成 best-effort：JSONL 主 sink 可用时，即使 human sink 打不开也继续运行，并通过 stderr/JSONL 记录 warning。

## Round ZEE-28: Human Log 交互细节与 Workpad 评论摘要

- **Spec 范围**：`SPEC.md:1249-1264`、`SPEC.md:2030-2036`
- **Scenario**：`observability.humanized_event_summaries`
- **Replace**：`dimension`，用“人类能直接复盘 Workpad 评论和 Codex turn”替代只看状态流转成功。
- **本轮证据**：
  - `427e53a`：把 `codex_event` 的关键 raw event 转成人类摘要，JSONL 原始事件不变。
  - `621e904`：给 `workpad_updated` 增加 `sections`、`comment_preview`、`task_progress`、`framework_progress`、`next_step`。
  - `go/internal/logging/jsonl.go:115-145`：先写原始 JSONL，再把 selected Codex event 投影为 human/console event。
  - `go/internal/logging/jsonl.go:182-295`：覆盖 session/turn lifecycle、agent message/final、command、file change、diff、plan update 等关键 Codex event class。
  - `go/internal/orchestrator/orchestrator.go:1264-1328`：Workpad 更新日志解析评论 sections、摘要和 checklist 进度。
  - `go/.symphony/logs/run-20260501-213900.human.log`：包含 `comment_preview`、`sections`、`codex_message`、`codex_command`、`codex_file_change`、`codex_final`。
- **Gap table**：
  - `conforms`：符合 `SPEC.md:1260-1264` 的稳定 `key=value`、动作 outcome、避免大 raw payload；humanized summaries 覆盖关键 wrapper/agent event class，且不改变 JSONL 或 orchestrator 行为。
  - `conforms`：Linear Workpad 写入不再只是 `bytes/lines/phase`，可看到评论结构、评论摘要、任务计划打勾进度和框架推进状态。
  - `partial`：同一 turn 内重复 `turn/diff/updated` 仍会产生多条相同 `codex_diff` 摘要；可读性已显著好于 raw JSON，但下一轮可做轻量去重。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/logging -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -run 'TestWorkpadLogSummary|TestRunAgentAIReviewAutoMergesWhenEnabled' -v`
  - `go test -ldflags='-linkmode=external' ./...`
  - `make build`
  - `git diff --check`
  - `make zh-smoke-round CHANGE_NOTE="human log 增加 Workpad 评论摘要"` 跑出 `ZEE-28`，`success=true`、`final_state=Done`、`changed_files=SMOKE.md`、`ai_review_result=passed`。
- **下一轮候选**：对重复 `codex_diff` 摘要做同 turn 轻量去重，或继续处理上一轮遗留的 optional human sink best-effort。

## Round ZEE-31: Issue Lifecycle Narrative

- **Spec 范围**：`SPEC.md:1249-1274`、`SPEC.md:2027-2036`、`SPEC.md:2086-2087`
- **Scenario**：`observability.issue_lifecycle_narrative`
- **Replace**：`dimension`，用“人能一眼复盘一个 issue 的全流程”替代单纯对齐 SPEC 的最低 logging 要求。
- **方向判断**：
  - `SPEC.md` 是下限：它要求结构化字段、稳定 `key=value`、operator-visible observability 和 humanized summaries 不影响正确性。
  - 当前可以比 SPEC 更好：human log 不只证明“有日志”，还应按 issue 讲清楚 hook、Linear Workpad、Codex 执行动作、AI Review、本地 merge 和最终 Done。
- **本轮证据**：
  - `3759009`：为 workspace hook 增加 `started/completed/failed/timed_out` 结构化事件，并带 `hook`、`command`、`workspace_path`、`duration_ms`、`output`。
  - `475cbf6`：把 AI commentary、计划、上下文读取等内部细节降为 `DEBUG`，保留真实执行命令、状态流转和 Workpad 为 `INFO`。
  - `a3e9ee3`：human log 隐藏成功 `workspace_cleaned` 噪声，原始 JSONL 仍持久化该事件。
  - `go/.symphony/logs/run-20260501-215731.human.log`：ZEE-31 开头包含 ZEE-30 `before_remove` 和 ZEE-31 `after_create` hook 命令、耗时、输出；随后包含 `workpad_updated`、Codex 命令、AI Review、local merge、Done。
  - `go/.symphony/logs/run-20260501-215731.human.log`：`codex_message` 和 context read 命令为 `DEBUG`，业务动作与流程推进为 `INFO`；没有 `workspace_cleaned` 行。
  - `go/internal/workspace/workspace.go:28-40` 定义 hook 事件和 observer；`go/internal/workspace/workspace.go:275-340` 在 hook 执行前后发事件。
  - `go/internal/orchestrator/orchestrator.go:531-563` 将 hook 事件转换成 issue-aware 日志；`go/internal/orchestrator/orchestrator.go:799-810` 给 create/before_run/after_run hook 注入 issue context。
  - `go/internal/logging/jsonl.go:182-245` 投影 Codex raw event；`go/internal/logging/jsonl.go:253-291` 将 AI message/context read 分到 `DEBUG`，将真实命令保留为 operator 可读事件；`go/internal/logging/jsonl.go:411-427` 识别 memory/skill/context reads。
  - `go/internal/logging/jsonl.go:182-185` human log 跳过成功 cleanup；`go/internal/logging/jsonl_test.go:286-315` 证明 human log 不显示成功 cleanup，但 JSONL 保留。
- **Gap table**：
  - `conforms`：满足 SPEC 的 issue/session context、stable `key=value`、action outcome 和 humanized event summaries。
  - `exceeds`：human log 已形成 issue 生命周期叙事：hook -> workpad -> codex turn -> validation/commit -> review -> merge -> done，能直接用于排障和复盘。
  - `partial`：human log 仍是扁平时间线；跨 issue 的 startup cleanup hook 会出现在下一轮开头，虽然现在不再有成功 cleanup 噪声，但后续可按 issue 分段或增加 section header。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/logging -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./internal/workspace -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -run 'TestRunAgentLogsWorkspaceHookEvents|TestRunAgentRunsBeforeAndAfterHooksAroundRunner|TestWorkpadLogSummary' -v`
  - `go test -ldflags='-linkmode=external' ./...`
  - `make build`
  - `git diff --check`
  - `make zh-smoke-round CHANGE_NOTE="human log 隐藏成功 cleanup 噪声"` 跑出 `ZEE-31`，`success=true`、`final_state=Done`、`changed_files=SMOKE.md`、`ai_review_result=passed`。
- **下一轮候选**：不要只继续补字段；优先做 issue 分段/生命周期视图，让 human log 在视觉上更像“一个 issue 的处理流水账”，同时保留 JSONL 作为机器分析底座。

## Round ZEE-32: Issue Start Boundary

- **Spec 范围**：`SPEC.md:1260-1264`、`SPEC.md:2027-2036`
- **Scenario**：`observability.issue_start_boundary`
- **Replace**：`dimension`，用“当前 issue 的 human log 开头必须对应当前 issue 生命周期”替代“所有 hook 都平铺展示”。
- **问题判断**：
  - 一个 issue 的人工流水账应该从当前 issue 的 `after_create` 开始。
  - ZEE-31 暴露的问题是：启动时 `StartupCleanup()` 会先清理上一轮 terminal issue，触发上一轮的 `before_remove` hook；这属于启动前清理，不属于当前 issue 的生命周期。
- **本轮修复**：
  - `cf96f79`：给 startup cleanup 触发的 hook 增加 `source=startup_cleanup`。
  - human log 隐藏成功的 startup cleanup hook started/completed；失败 hook 仍然按 `workspace_hook_failed` 可见。
  - 原始 JSONL 不过滤，仍保留 `before_remove` hook 和 `workspace_cleaned` 证据。
- **本轮证据**：
  - `go/internal/orchestrator/orchestrator.go:251-255`：`StartupCleanup()` 使用 `workspace.WithHookSource(ctx, "startup_cleanup")`。
  - `go/internal/orchestrator/orchestrator.go:531-563`：hook 日志带 `source` 字段。
  - `go/internal/workspace/workspace.go:28-40`、`go/internal/workspace/workspace.go:58-64`、`go/internal/workspace/workspace.go:340-349`：hook event 支持 source context。
  - `go/internal/logging/jsonl.go:182-204`：human log 跳过成功 startup cleanup hook，但不跳过失败 hook。
  - `go/.symphony/logs/run-20260501-221046.human.log`：第一行是 `ZEE-32 Todo -> In Progress`，第二/三行是 `ZEE-32 after_create` started/completed，没有上一轮 `before_remove`。
  - `go/.symphony/logs/run-20260501-221046.jsonl`：第 1-2 行仍保留 `ZEE-31 before_remove`，并带 `source=startup_cleanup`。
- **Gap table**：
  - `conforms`：当前 issue human log 不再被上一轮 cleanup hook 抢占开头，符合人工复盘的 issue 生命周期模型。
  - `conforms`：JSONL 仍保留 startup cleanup 的完整机器证据，debug/replay 能力没有丢。
  - `partial`：human log 仍然缺少显式 section header；目前靠 issue 字段和事件顺序阅读，后续可以继续做视觉分段。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/logging -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./internal/workspace -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -run 'TestStartupCleanupRemovesTerminalWorkspaces|TestRunAgentLogsWorkspaceHookEvents' -v`
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -v`
  - `go test -ldflags='-linkmode=external' ./...`
  - `make build`
  - `git diff --check`
  - `make zh-smoke-round CHANGE_NOTE="human log 从 after_create 开始"` 跑出 `ZEE-32`，`success=true`、`final_state=Done`、`changed_files=SMOKE.md`、`ai_review_result=passed`。
- **下一轮候选**：如果继续做人类可读性，优先加 issue section header 或 summary footer，而不是再扩大过滤规则。

## Round ZEE-33: Issue Section Headers

- **Round goal**：替代 `dimension`，旧信号 `flat current-issue human log`，新 SPEC 信号 `human-readable status surface uses stable key=value issue boundaries without changing orchestrator behavior`。
- **Iteration**：2 patch。
- **Spec 范围**：`SPEC.md:1260-1264`、`SPEC.md:2027-2036`。
- **Workflow / Issue**：`WORKFLOW.zh-smoke.md`，本轮无 workflow diff；真实隔离 issue `ZEE-33`，从 `Todo` 跑到 `Done`。
- **本轮运行**：先确认无旧 `symphony-go` poller；`make zh-smoke-round CHANGE_NOTE="human log 增加 issue 分段"` 生成 `go/.symphony/logs/run-20260501-222142.jsonl` 和 `go/.symphony/logs/run-20260501-222142.human.log`。
- **Commit**：`before=c89288e629d531b4b796039e1f8f6969e66757da`，`after=f17f76b5a9ba31551aba539e951725ae26e0b079`，`fix(logging): add issue section headers to human log`。
- **Gap table**：
  - `conforms`：`go/.symphony/logs/run-20260501-222142.human.log:1` 是 `event=issue_section issue=ZEE-33 msg="Issue ZEE-33"`，第 2 行才进入 `state_changed`。
  - `conforms`：`rg -n 'issue_section' go/.symphony/logs/run-20260501-222142.jsonl` 无匹配，说明 section header 只存在于 human/console 展示层，不污染 JSONL。
  - `conforms`：`go/internal/logging/jsonl.go:17-29` 只记录每个 sink 的 last issue；`go/internal/logging/jsonl.go:136-160` 写 display line 前按 issue 插入 header；`go/internal/logging/jsonl.go:172-184` 构造 display-only `issue_section`。
- **已修复内容**：只改 `go/internal/logging/jsonl.go` 和 `go/internal/logging/jsonl_test.go`；新增 `TestHumanLogWritesIssueSectionHeaders`，证明每个 issue 只写一条 section header，且 JSONL 不包含 `issue_section`。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/logging -count=1 -run TestHumanLogWritesIssueSectionHeaders -v`：先失败，缺少 header；实现后通过。
  - `go test -ldflags='-linkmode=external' ./internal/logging -count=1 -v`：通过。
  - `go test -ldflags='-linkmode=external' ./...`：第一次 `TestSnapshotTracksCodexEventTokens` 等待 runner event 超时；单独复跑该 case 通过；第二次全量通过。
  - `make build`：通过。
  - `git diff --check`：通过。
  - `make zh-smoke-round CHANGE_NOTE="human log 增加 issue 分段"`：`ZEE-33 success=true`、`final_state=Done`、`changed_files=SMOKE.md`、`ai_review_result=passed`。
- **Darwin 判定**：keep。该改动让 human log 明确出现 issue 分段，同时保留 JSONL 原始事件合同。
- **Coverage ledger 更新**：`13 observability` 保持 `partial`，证据更新到 `ZEE-33`；`17-18 real_integration` 保持 `partial`，证据更新到真实 Linear smoke `ZEE-33`。
- **下一轮候选**：`observability.sink_failure`，把 optional human sink 改成 best-effort，并验证 sink 失败时 JSONL/console 仍能给 operator warning。

## Round 2026-05-01: workflow_config.dynamic_reload_last_known_good - baseline

- **Round goal**：替代 `scenario`，旧信号 `untested workflow_config coverage`，新 SPEC 信号 `dynamic reload keeps last known good on invalid reload and reapplies valid config without restart`。
- **Iteration**：1 baseline。
- **Spec 范围**：`SPEC.md:524-540`、`SPEC.md:1931-1943`。
- **Workflow / Issue**：`WORKFLOW.md`，本轮无 workflow diff；使用 synthetic tests 和 config-only startup check，不创建真实 Linear issue。
- **本轮运行**：不启动常驻 poller，不改 workflow 文件；读取 reloader/orchestrator 实现并跑 focused tests。
- **Commit**：`before=f17f76b5a9ba31551aba539e951725ae26e0b079`，`after=none`，本轮 baseline 未改代码。
- **Gap table**：
  - `conforms`：`go/internal/workflow/reloader.go:73-106` 只有在 `CommitCandidate()` 后才替换 `current`，无效 reload 返回 error 且不替换 last known good。
  - `conforms`：`go/internal/orchestrator/orchestrator.go:472-499` reload 失败会 `workflow_reload_failed` 并保留现有 runtime；依赖重建成功后才 commit candidate、替换 tracker/workspace/runner 和 polling interval。
  - `conforms`：`go/internal/workflow/reloader_test.go:13-81` 覆盖 invalid edit 保留 last good 和 valid edit apply；`go/internal/orchestrator/orchestrator_test.go:1579-1806` 覆盖 reload error 可见、dependency rebuild、factory failure 不半应用、下次无文件触碰仍可重试。
- **已修复内容**：无。该子场景 baseline 已符合 SPEC。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/workflow -count=1 -run 'TestReloaderKeepsLastGoodWorkflowAfterInvalidEdit|TestReloaderAppliesValidEdit' -v`：通过。
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -run 'TestPollKeepsReloadErrorVisibleAfterTrackerSuccess|TestRefreshWorkflowRebuildsDependenciesFromFactories|TestRefreshWorkflowFactoryFailureDoesNotHalfApply|TestRefreshWorkflowRetriesFactoryFailureWithoutFileTouch' -v`：通过。
  - `LINEAR_API_KEY="${LINEAR_API_KEY:-lin_fake_for_config_check}" ./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue DOES-NOT-EXIST`：通过。
- **Darwin 判定**：keep baseline，无需 patch。
- **Coverage ledger 更新**：`5-6 workflow_config` 从 `untested` 变 `partial`。
- **下一轮候选**：切到 `workspace_safety`，优先验证 workspace key/root containment/hook cwd；如果必须继续 workflow_config，则只补一个 live file-change smoke 证明真实日志中出现 `workflow_reload_failed` / `workflow_reloaded`。

## Round 2026-05-01: workspace_safety.root_and_hook_boundaries - baseline

- **Round goal**：替代 `scenario`，旧信号 `untested workspace_safety coverage`，新 SPEC 信号 `workspace path stays inside root, sanitized workspace key, hooks run with workspace cwd, agent launch uses workspace cwd`。
- **Iteration**：1 baseline。
- **Spec 范围**：`SPEC.md:814-905`、`SPEC.md:1619-1623`、`SPEC.md:1952-1965`。
- **Workflow / Issue**：`WORKFLOW.md`，本轮无 workflow diff；使用 synthetic unit tests，不创建真实 Linear issue。
- **本轮运行**：不启动常驻 poller；读取 workspace manager、orchestrator runAgent、Codex runner 实现，并跑 focused tests。
- **Commit**：`before=f17f76b5a9ba31551aba539e951725ae26e0b079`，`after=none`，本轮 baseline 未改代码。
- **Gap table**：
  - `conforms`：`go/internal/workspace/workspace.go:72-88` 归一化 root 并用 `SafeIdentifier` 派生 per-issue workspace path；`go/internal/workspace/workspace.go:273-281` 只保留 `[A-Za-z0-9._-]`，其余字符替换为 `_`。
  - `conforms`：`go/internal/workspace/workspace.go:91-142` 同时做 lexical root containment 和 symlink-aware containment；`go/internal/workspace/workspace.go:190-239` 创建/复用 workspace 前后校验路径；`go/internal/workspace/workspace.go:242-270` cleanup 避免跟随逃逸 symlink。
  - `conforms`：`go/internal/workspace/workspace.go:170-187`、`go/internal/workspace/workspace.go:284-335` 在 hook 执行前校验 path，并用 `cmd.Dir = cwd` 运行 hook。
  - `conforms`：`go/internal/orchestrator/orchestrator.go:803-817` 在 agent 运行前先 `Ensure` + `BeforeRun`；`go/internal/orchestrator/orchestrator.go:843-889` 把同一个 `workspacePath` 传给 Codex runner；`go/internal/codex/runner.go:156-190` app-server 进程 `cmd.Dir = workspacePath`，thread/turn start 也携带 cwd；`go/internal/codex/runner.go:199-220` 默认 turn sandbox writable root 包含 workspace path。
- **已修复内容**：无。该子场景 baseline 已符合 SPEC。
- **验证结果**：
  - `go test -ldflags='-linkmode=external' ./internal/workspace -count=1 -run 'TestPathForIssueStaysInsideRoot|TestPathForIssueSanitizesDotDotIdentifierInsideRoot|TestBeforeRunAndAfterRunHookSemantics|TestValidateWorkspacePathRejectsEscape|TestEnsureReplacesSymlinkWorkspaceEscapingRoot|TestValidateAndBeforeRunRejectSymlinkWorkspaceEscapingRoot|TestRemoveSymlinkWorkspaceRemovesOnlyLink' -v`：通过。
  - `go test -ldflags='-linkmode=external' ./internal/orchestrator -count=1 -run 'TestRunAgentRunsBeforeAndAfterHooksAroundRunner|TestRunAgentLogsWorkspaceHookEvents' -v`：通过。
  - `go test -ldflags='-linkmode=external' ./internal/codex -count=1 -run TestRunnerSendsChinesePromptAndGitWritableRoots -v`：通过。
- **Darwin 判定**：keep baseline，无需 patch。
- **Coverage ledger 更新**：`9 workspace_safety` 从 `untested` 变 `covered`。
- **下一轮候选**：切到 `agent_runner`，优先验证 continuation/session/cwd 行为；或切到 `tracker_selection` 验证 Todo blocker 和 active/terminal selection。

## Round ZEE-35: workspace_safety.real_issue_hook_cwd - real scenario

- **Round goal**：替代 `scenario`，旧信号 `synthetic workspace_safety evidence`，新 SPEC 信号 `real Linear issue and real workflow prove hooks and agent launch use the per-issue workspace cwd`。
- **Iteration**：2 real scenario。
- **Spec 范围**：`SPEC.md:872-877`、`SPEC.md:890-895`、`SPEC.md:1952-1965`。
- **Workflow / Issue**：新增本地场景 workflow `go/WORKFLOW.workspace-safety.md`；真实 Linear issue `ZEE-34` 首轮用于暴露 workflow 构造错误，真实 Linear issue `ZEE-35` 用修正后的 workflow 验证成功。
- **本轮运行**：
  - `make run-once WORKFLOW=./WORKFLOW.workspace-safety.md ISSUE=ZEE-34 MERGE_TARGET=zh-smoke-harness-loop`：失败，`after_create` 里 bootstrap worktree 后没有 `cd "$workspace"`，导致 marker 文件写入旧 cwd 失败。
  - 修正 workflow：`symphony_after_create.sh` 后显式 `cd "$workspace"` 再写 `.symphony-safety/after_create.cwd`。
  - `make run-once WORKFLOW=./WORKFLOW.workspace-safety.md ISSUE=ZEE-35 MERGE_TARGET=zh-smoke-harness-loop`：成功进入 `Human Review`。
- **Commit**：`before=f17f76b5a9ba31551aba539e951725ae26e0b079`，`after=none`，本轮没有代码提交；agent 在 issue worktree 中创建本地提交 `7a58ac0 ZEE-35: workspace safety proof`。
- **Gap table**：
  - `missing`：`ZEE-34` 证明初版场景 workflow 有构造错误，hook bootstrap 后继续用旧 cwd 写 marker，真实运行失败。这不是 Go 实现 gap，但说明真实场景比代码阅读更有效。
  - `conforms`：`go/.symphony/logs/run-20260501-224057.human.log:3-6` 证明 `after_create`、`before_run` hook 的 `workspace_path` 是 `/Users/bytedance/symphony/go/.worktrees-safety/ZEE-35`。
  - `conforms`：`go/.symphony/logs/run-20260501-224057.human.log:12-14` 证明 agent 执行 `pwd -P`、读取 `after_create.cwd`、读取 `before_run.cwd`，三个输出完全一致。
  - `conforms`：`go/.symphony/logs/run-20260501-224057.human.log:23-25` 证明两条 cwd equality test 通过，`WORKSPACE_SAFETY_PROOF.md` 命中 `workspace safety proof: passed`。
  - `conforms`：`go/.worktrees-safety/ZEE-35/WORKSPACE_SAFETY_PROOF.md` 记录 `pwd -P output`、`after_create.cwd content`、`before_run.cwd content` 三者都等于 `/Users/bytedance/symphony/go/.worktrees-safety/ZEE-35`。
- **已修复内容**：只修场景 workflow，不改 Go 代码。
- **验证结果**：
  - `ZEE-35` 状态：`In Progress -> Human Review`。
  - 本地 issue worktree 提交：`7a58ac0 ZEE-35: workspace safety proof`。
  - proof 文件：`go/.worktrees-safety/ZEE-35/WORKSPACE_SAFETY_PROOF.md`。
  - run log：`go/.symphony/logs/run-20260501-224057.jsonl` 和 `go/.symphony/logs/run-20260501-224057.human.log`。
- **Darwin 判定**：keep evidence。真实场景确认 workspace_safety 核心行为符合 SPEC；不需要代码 patch。
- **Coverage ledger 更新**：`9 workspace_safety` 继续 `covered`，证据从单测升级到真实 issue `ZEE-35`。
- **下一轮候选**：切到 `agent_runner`，用同样方式构造真实 issue/workflow 验证 continuation/session/cwd 行为。
## Round ZEE-42: real_integration.auto_merge_no_human_review - listener-owned terminal flow

- **Round goal**：替代 `scenario`，旧信号是“父会话手工 merge 也算完成”，新 SPEC 信号是 listener 在真实 Linear + Codex + git worktree 场景中自己完成 `Todo -> In Progress -> AI Review -> Merging -> Done`，且不进入 `Human Review`。
- **Iteration**：1 baseline/ratchet for the ZEE-41 fixes。上一轮 ZEE-41 已暴露 root checkout writable root 与状态 owner 重叠问题，本轮用新真实 issue 验证修复后的 SPEC signal。
- **Spec 范围**：`SPEC.md:900-968`、`SPEC.md:1033-1064`、`SPEC.md:1935-1936`、`SPEC.md:2027-2036`。
- **Workflow / Issue**：`WORKFLOW.md@975aac0`；Linear issue `ZEE-42`，初始 `Todo`，目标 `Done`；issue 只允许创建 `SPEC_SMOKE.md` 并写入 `SPEC real integration smoke: 2026-05-02 16:50:23 +0800`。
- **本轮运行**：`make build` 后启动 issue-scoped listener，PID `61357`，daemon log `.symphony/logs/ZEE-42-20260502-165107.out`，JSONL `.symphony/logs/run-20260502-165107.jsonl`，human log `.symphony/logs/run-20260502-165107.human.log`。
- **Commit**：`before=975aac0`，issue commit `03e3263 ZEE-42 记录 SPEC 真实集成冒烟`，merge commit `after=96220b6 合并 symphony-go/ZEE-42 到 main`。
- **Gap table**：`real_integration.auto_merge_no_human_review` 判定 `conforms`；`agent_runner.linear_graphql_tool_contract` 判定 `unclear`，因为真实 run 中 `linear_graphql` 不在 PATH/tool surface，自动化靠 `linear` CLI fallback 成功。
- **已修复内容**：本轮不修改代码；验证前一轮已保留的 `Merging` writable-root 和 workflow handoff 修复。
- **验证结果**：ZEE-42 到 `Done`；human log line 101-103 证明 orchestrator 自动 `In Progress -> AI Review -> Merging`；line 164 证明 child 自己在 repo root 执行 `git merge --no-ff` 成功；line 178 证明 `git push origin main` 成功；line 197 证明 `before_remove` cleanup 运行；`rg -n "SPEC real integration smoke: 2026-05-02 16:50:23 \+0800" SPEC_SMOKE.md` 通过；`git diff --check` 通过；`git status --short --branch` 为 `## main...origin/main`。
- **Darwin 判定**：keep。真实 run 已证明 ZEE-41 的两个修复让全自动流更接近 SPEC：没有默认 Human Review，没有父会话手工 merge，terminal cleanup 也完成。
- **Coverage ledger 更新**：`17-18 real_integration` 从 `partial` 更新为 `covered`；`10 agent_runner` 和 `12 agent_runner` 更新为 `partial`，下一步聚焦 `linear_graphql` optional tool contract。
- **下一轮候选**：`agent_runner.linear_graphql_tool_contract`。最小动作是先判断 `linear_graphql` 是计划实现的 client-side tool 还是文档/Workflow 遗留期望；若计划实现，补 tool advertising/structured failure 的 synthetic test，再用真实 issue 验证一次 comment/status 写入。

## Round 2026-05-06: agent_runner.linear_graphql_tool_contract - patch

- **Round goal**：替代 `scenario`，旧信号是 ZEE-42 真实流程里 `command -v linear_graphql` 为空且靠 CLI fallback 成功，新 SPEC 信号是 optional client-side tool 在 Codex app-server 协议层被 advertised、可执行、可失败返回且不会卡住 session。
- **Iteration**：2 patch。
- **Spec 范围**：`SPEC.md:1057-1084`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；本轮使用 synthetic app-server 和 fake GraphQL client，不创建真实 Linear issue。
- **本轮运行**：读取当前实现后确认工具已经通过 app-server dynamic tool 而不是 shell PATH 注入：`internal/service/codex/runner.go:278-289` 在 `thread/start` 发送 `dynamicTools`，`internal/service/codex/runner.go:392-418` 在 `item/tool/call` 返回 tool result，`internal/app/run.go:134` 和 `internal/app/run.go:151` 把 Linear client 注入 `NewDynamicToolExecutor`。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `conforms`：`internal/service/codex/runner_test.go:251-324` 覆盖 `thread/start` advertised `linear_graphql`，并证明 `item/tool/call` 会执行 fake GraphQL client、把 successful payload 返回给 app-server。
  - `conforms`：`internal/service/codex/dynamic_tool_test.go:10-24` 覆盖 tool spec 名称和 required query schema。
  - `conforms`：`internal/service/codex/dynamic_tool_test.go:80-135` 覆盖 unsupported tool failure、0/多个 operation 拒绝、1 个 operation 加 fragment/string 通过。
  - `patch`：`internal/service/codex/dynamic_tool.go:122-229` 新增 `query` 必须 exactly one GraphQL operation 的校验，补齐 `SPEC.md:1082-1084` 的输入约束。
- **已修复内容**：`linear_graphql` 动态工具在发起 Linear 请求前拒绝空 operation、多个 operation，以及 `variables` 非 object 的输入；unsupported tool 名称继续返回失败 payload，不让 session 卡住。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/service/codex`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该 patch 让 synthetic app-server tool contract 更接近 SPEC；ZEE-42 的 PATH 判断已被澄清为错误观察面，因为工具不是 shell binary。
- **Coverage ledger 更新**：`10 agent_runner` 与 `12 agent_runner` 仍为 `partial`，但 evidence 更新为 dynamic tool code/test；距离 `covered` 还缺一轮真实 issue ratchet。
- **下一轮候选**：`agent_runner.linear_graphql_tool_real_issue_ratchet`，用一个隔离 Linear issue 验证派生 Codex session 通过 injected `linear_graphql` 更新 workpad 或状态，并记录 `.jsonl` / `.human.log`。

## Round 2026-05-06: agent_runner.user_input_and_approval_policy - patch

- **Round goal**：替代 `scenario`，旧信号是 approval / user-input policy 只散落在 workflow 和 runner 错误分支里，且 `turn/input_required` 等 app-server 事件可能一直等到 turn timeout；新 SPEC 信号是实现明确记录 approval/sandbox/operator-confirmation policy，并且 targeted user-input / approval request 不会让 unattended run 无限等待。
- **Iteration**：2 patch。
- **Spec 范围**：`SPEC.md:1033-1041`、`SPEC.md:1099-1105`、`SPEC.md:2014-2024`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不跑真实 Linear issue，只用 synthetic app-server 事件测试。
- **本轮运行**：fake app-server 在 turn start 后分别发出 `turn/input_required`、`turn/approval_required`、`item/tool/requestUserInput`、`item/commandExecution/requestApproval` 和 `item/fileChange/requestApproval`。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `patch`：`internal/service/codex/runner.go:389-396` 新增 fail-fast 分支，避免 user-input / approval request 事件落入等待 turn completion 的路径。
  - `conforms`：`internal/service/codex/runner_test.go:328-378` 证明五类 user-input / approval request 都在 2 秒内失败返回，而不是等到 `turn_timeout_ms`。
  - `conforms`：`docs/runtime-policy.md:5-42` 记录 trust boundary、approval/sandbox policy、user-input failure policy 和 dynamic tool semantics。
- **已修复内容**：runner 对 app-server user-input / approval request 事件 fail-fast；新增集中运行策略文档；`docs/agents/domain.md` 把该文档加入本仓领域文档入口。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/service/codex`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该 patch 让 `SPEC.md` 的 implementation-defined policy 和 no-stall 要求有代码与文档双重证据。
- **Coverage ledger 更新**：`10 agent_runner` 与 `12 agent_runner` 仍为 `partial`，但 evidence 更新为 input/approval fail-fast 与 runtime policy 文档；按用户要求不再把真实 issue ratchet 作为下一步。
- **下一轮候选**：`tracker_selection`，只做静态 SPEC 对照和本地测试，不创建真实 issue。

## Round 2026-05-06: tracker_selection.candidate_and_linear_contract - static baseline

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `tracker_selection` 仍为 `untested`；新 SPEC 信号是本地测试覆盖 candidate eligibility、排序、Linear 查询语义、分页、鉴权、状态刷新和 normalized issue model。
- **Iteration**：1 baseline + focused test ratchet。
- **Spec 范围**：`SPEC.md:727-745`、`SPEC.md:1147-1208`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不访问真实 Linear。
- **本轮运行**：使用 `httptest` 验证 Linear HTTP/GraphQL contract，使用 orchestrator unit tests 验证 candidate selection。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `conforms`：`internal/service/orchestrator/eligibility_test.go:10-140` 覆盖 priority / created_at / identifier 排序、Todo blocker、review wait state、AI Review eligibility 和 per-state concurrency slots。
  - `conforms`：`internal/integration/linear/client_test.go:15-45` 覆盖 UTF-8 JSON body 和 `Authorization` header；`internal/integration/linear/client_test.go:113-135` 覆盖 default endpoint、env API key、project slug 和 30s timeout。
  - `conforms`：`internal/integration/linear/client_test.go:81-110`、`internal/integration/linear/client_test.go:177-255` 覆盖 candidate pagination、provided states、inverseRelations blockers、lowercase labels、parsed timestamps、`[ID!]` state refresh 和 id variables。
- **已修复内容**：补齐 Linear adapter 的 SPEC 锚点测试：`Authorization` header、默认 endpoint、环境 token、project slug 和 30s timeout。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/integration/linear ./internal/service/orchestrator`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮把 tracker_selection 从 `untested` 推进到本地 deterministic evidence，不依赖真实 Linear 环境。
- **Coverage ledger 更新**：`11 tracker_selection` 从 `untested` 更新为 `covered`。
- **下一轮候选**：`cli_lifecycle` 或 `contract_scope`，继续只做 SPEC 静态对照和本地验证。

## Round 2026-05-06: cli_lifecycle.positional_and_startup_contract - static patch

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `cli_lifecycle` 仍为 `untested`，且 `SPEC.md:2048-2056` 要求 CLI 支持位置参数 `path-to-WORKFLOW.md`，当前实现只支持 `--workflow`。
- **Iteration**：1 static patch。
- **Spec 范围**：`SPEC.md:2048-2056`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不跑真实 listener，只对照 SPEC 和本地代码/测试。
- **本轮运行**：补齐 CLI 解析层对 default workflow、位置参数、`--workflow` 显式优先级和多位置参数拒绝的测试，并复用 app runtime 测试覆盖 missing workflow、service error、HTTP startup success 和 bind failure。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `patch`：`cmd/symphony-go/main.go:33-68` 新增位置参数 workflow path 解析，`cmd/symphony-go/main.go:107-110` 同步 usage。
  - `conforms`：`cmd/symphony-go/main_test.go:5-68` 覆盖默认 `./WORKFLOW.md`、位置参数、`--workflow` 优先级和多位置参数错误。
  - `conforms`：`internal/app/run_test.go:87-202` 覆盖 missing workflow load error、runtime service error、HTTP startup success 和 bind failure。
- **已修复内容**：CLI 现在支持 `symphony-go run path-to-WORKFLOW.md`，同时保留 `--workflow` 兼容路径；当两者同时出现时以显式 flag 为准。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./cmd/symphony-go ./internal/app`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮用最小改动补齐 SPEC 中明确的 CLI 位置参数契约，没有扩大到真实运行场景。
- **Coverage ledger 更新**：`16 cli_lifecycle` 从 `untested` 更新为 `covered`。
- **下一轮候选**：`contract_scope` 或 `domain_model`，继续只做 SPEC 静态对照和本地验证。

## Round 2026-05-06: contract_scope.static_boundary_contract - static patch

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `contract_scope` 仍为 `untested`；新 SPEC 信号是 `SPEC.md:16-143` 和 `SPEC.md:2078-2098` 的服务边界、目标/非目标、组件分层和必需能力有本地文档与可执行测试证据。
- **Iteration**：1 static patch。
- **Spec 范围**：`SPEC.md:16-143`、`SPEC.md:2078-2098`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不跑真实 listener，只对照 SPEC、代码和文档。
- **本轮运行**：读取 SPEC 1-3 后确认本轮不需要真实场景；补 `docs/contract-scope.md` 记录 core service boundary、policy boundary 和 optional surfaces；补 app 层测试锁住 runtime assembly。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `conforms`：`docs/contract-scope.md:1-28` 记录 Symphony Go 是 long-running tracker scheduler/runner，核心组件映射到当前 `internal/service`、`internal/runtime`、`internal/integration` 和 operator surfaces。
  - `conforms`：`internal/app/run.go:72-166` 装配 workflow loader、Linear tracker、workspace manager、Codex runner、orchestrator、logging、HTTP control server 和 TUI，符合 SPEC 的 main components。
  - `conforms`：`internal/app/contract_scope_test.go:10-68` 检查 contract scope 文档和 app runtime assembly，并防止 app 入口层直接拥有 workpad/comment/PR 业务逻辑。
- **已修复内容**：新增 contract scope 文档，把 HTTP/TUI 和 Linear write helpers 明确归为 operator/diagnostic/runner support surface，不让它们混淆核心 scheduler/runner 边界；新增本地测试保护该边界。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/app`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮把 SPEC 1-3 的边界从人工阅读变成可 review、可测试的本地合同，没有扩大真实场景或引入新运行依赖。
- **Coverage ledger 更新**：`1-3 contract_scope` 从 `untested` 更新为 `covered`。
- **下一轮候选**：`domain_model`，继续只做 SPEC 静态对照和本地验证。

## Round 2026-05-06: domain_model.issue_workspace_session_retry_contract - static patch

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `domain_model` 仍为 `untested`；新 SPEC 信号是 `SPEC.md:146-288` 的 issue、workflow/config、workspace、run attempt、live session、retry entry 和 runtime state 字段有代码与本地测试证据。
- **Iteration**：1 static patch。
- **Spec 范围**：`SPEC.md:146-288`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不跑真实 listener，只对照 SPEC、代码和本地测试。
- **本轮运行**：现有 `Issue` / workspace / config 已基本匹配 SPEC；本轮发现 live session 和 run attempt 投影缺少独立 `thread_id`、`turn_id` 与 running attempt，于是补齐内部 observability/control projection。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `conforms`：`internal/service/issue/types.go:5-23` 覆盖 normalized issue、blocker refs、labels、created/updated timestamps。
  - `conforms`：`internal/service/workspace/workspace_test.go:15-110` 覆盖 sanitized workspace key、workspace path 留在 root 内和 first ensure created flag。
  - `patch`：`internal/runtime/observability/snapshot.go:21-47` 给 running entry 增加 `attempt`、`thread_id`、`turn_id`，并保留 retry attempt/due/error/workspace。
  - `patch`：`internal/service/orchestrator/agent_session.go:22-65` 和 `internal/service/orchestrator/orchestrator.go:955-991` 在 running state 中保留 attempt，并从 Codex events 采集 session/thread/turn identity。
  - `conforms`：`internal/service/orchestrator/orchestrator_test.go:110-140`、`internal/service/orchestrator/orchestrator_test.go:3120-3131` 和 `internal/service/control/service_test.go:46-84` 覆盖 identity、attempt 和 token projection。
- **已修复内容**：running state 现在保留 SPEC live session 的 `thread_id` / `turn_id`，并在 retry/continuation attempt 时保留 `attempt`；control service projection 同步暴露这些内部字段。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/runtime/observability ./internal/service/orchestrator ./internal/service/control`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮修的是 SPEC domain model 的真实字段缺口，且只影响本地 runtime state/projection。
- **Known gap**：Hertz generated HTTP model 目前仍只有 `session_id`，没有单独 `thread_id` / `turn_id`；本轮不跑 IDL 生成，先把内部域模型补齐。
- **Coverage ledger 更新**：`4 domain_model` 从 `untested` 更新为 `covered`，但外部 HTTP IDL 可作为后续细化项。
- **下一轮候选**：`orchestrator_state`，继续只做 SPEC 静态对照和本地验证。

## Round 2026-05-06: orchestrator_state.reload_failure_skips_dispatch_after_reconcile - static patch

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `7-8 orchestrator_state` 与 `14 failure_model` 仍为 `untested`；新 SPEC 信号是 `SPEC.md:608-818` 和 `SPEC.md:1527-1611` 的 poll tick、claim/running/retry 状态、reconciliation、dispatch validation failure 与 retry 行为有代码和测试证据。
- **Iteration**：1 static patch。
- **Spec 范围**：`SPEC.md:608-818`、`SPEC.md:1527-1611`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不跑真实 listener，只对照 SPEC、代码和本地测试。
- **本轮运行**：发现 `pollDispatched` 在 workflow reload / validation failure 后仍继续 `FetchActiveIssues`，不符合 “reconciliation first, dispatch skipped for that tick”。补最小分支：reconciliation 后若 reload failed，直接返回，不做候选抓取/dispatch。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `patch`：`internal/service/orchestrator/orchestrator.go:162-176` 在 reload failure 后跳过 candidate fetch 和 dispatch，同时保留前置 `reconcileRunning`。
  - `conforms`：`internal/service/orchestrator/orchestrator.go:207-290` 覆盖 stall detection、terminal/non-active/active state reconciliation。
  - `conforms`：`internal/service/orchestrator/orchestrator.go:637-676` 和 `internal/service/orchestrator/retry.go:17-75` 覆盖 worker exit、release、failure retry、continuation retry 和 capped backoff。
  - `conforms`：`internal/service/orchestrator/orchestrator_test.go:2262-2305` 证明 reload failure tick 仍做 state refresh，但不会 fetch active candidates 或 dispatch。
- **已修复内容**：invalid workflow reload / dispatch validation failure 不再基于旧 config 继续启动新 worker；已有 running worker 仍按 tick 顺序先 reconciliation。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/service/orchestrator`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮修复的是 SPEC 8.1 / 14.2 的真实状态机偏差，且改动只在调度入口处短路 dispatch。
- **Coverage ledger 更新**：`7-8 orchestrator_state` 与 `14 orchestrator_state` 从 `untested` 更新为 `covered`。
- **下一轮候选**：`security_ops`，继续只做 SPEC 静态对照和本地验证。

## Round 2026-05-06: security_ops.runtime_policy_and_baseline_guards - static patch

- **Round goal**：替代 `scenario`，旧信号是 coverage ledger 里 `15 security_ops` 仍为 `untested`；新 SPEC 信号是 `SPEC.md:1612-1684` 的 trust boundary、approval/sandbox posture、filesystem safety、secret handling、hook safety 和 hardening guidance 有文档与本地测试证据。
- **Iteration**：1 static patch。
- **Spec 范围**：`SPEC.md:1612-1684`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；按用户要求不创建真实 issue，不跑真实 listener，只对照 SPEC、代码和本地测试。
- **本轮运行**：现有代码已有 workspace containment、runtime settings 不暴露 secret、hook preview 截断、approval/input fail-fast 等测试；缺口是 runtime policy 没有集中写清 secret handling、hook safety 和 harness hardening。补文档并用 app 层测试锁住。
- **Commit**：`before=ebcda9ab91c350620fc1a81b41be6e325d54e59d`，`after=uncommitted`，本轮保留工作区改动但未提交。
- **Gap table**：
  - `patch`：`docs/runtime-policy.md:23-60` 新增 secret handling、hook safety 和 harness hardening，并明确 `linear_graphql` 由配置的 Linear credential 形成边界。
  - `conforms`：`internal/app/contract_scope_test.go:61-84` 锁住 runtime policy 的 trust/sandbox/secret/hook/hardening 文案。
  - `conforms`：`internal/service/workspace/workspace_test.go:86-165` 覆盖 sanitized identifier 和 workspace path escape rejection。
  - `conforms`：`internal/service/orchestrator/orchestrator.go:542-560` 对 hook command/output/error 做 preview 截断。
  - `conforms`：`internal/service/control/service_test.go:235-270` 与 `internal/transport/hertzserver/server_test.go:348-400` 覆盖 runtime settings 不暴露 API key。
  - `conforms`：`internal/service/codex/runner_test.go:328-378` 覆盖 user-input / approval request fail-fast。
- **已修复内容**：把安全姿态从零散实现补成集中 policy，并让测试防止 future edit 删除关键安全说明。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/app ./internal/service/control ./internal/service/workspace ./internal/service/codex`：通过。
  - `GOCACHE=$PWD/.gocache ./build.sh`：通过。
  - `git diff --check`：通过。
- **Darwin 判定**：keep。该轮没有引入新 sandbox 或新依赖，只把 SPEC 15 的 implementation-defined 安全边界写清并用本地测试固定。
- **Known gap**：`linear_graphql` 仍是 raw GraphQL extension，安全边界来自 Linear credential 和 tracker configuration；更窄的 project/team scoped enforcement 可作为后续增强。
- **Coverage ledger 更新**：`15 security_ops` 从 `untested` 更新为 `covered`。
- **下一轮候选**：先做 coverage audit；剩余 `workflow_config`、`agent_runner`、`observability` 仍为 `partial`。

## Round 2026-05-06: workflow_config.dynamic_reload_static_completion - static audit

- **Round goal**：替代 `scenario`，旧信号是 `workflow_config` 仍为 `partial`，原因是旧轮次建议再跑 live file-change smoke；本轮按用户要求不做真实场景，只用 SPEC、代码和本地测试完成静态收口。
- **Iteration**：2 static audit。
- **Spec 范围**：`SPEC.md:524-540`、`SPEC.md:1931-1946`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；不创建真实 issue，不启动 listener。
- **Gap table**：
  - `conforms`：`internal/service/workflow/reloader.go:73-106` 只在 `CommitCandidate` 后替换当前 workflow。
  - `conforms`：`internal/service/workflow/reloader_test.go:13-81`、`83-131` 覆盖 invalid reload 保留 last known good、valid reload apply 和 initial load 稳定重试。
  - `conforms`：`internal/service/orchestrator/orchestrator.go:477-502` 与 `internal/service/orchestrator/orchestrator_test.go:2079-2338` 覆盖 reload error 可见、dependency rebuild、factory failure 不半应用、reload failure tick 跳过 dispatch，以及无文件触碰后的重试。
- **已修复内容**：本轮未改 workflow 代码；把旧的 live-proof 要求降为静态目标下的非阻塞项。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/service/workflow ./internal/service/orchestrator -run 'TestReloaderKeepsLastGoodWorkflowAfterInvalidEdit|TestReloaderAppliesValidEdit|TestNewReloaderRetriesWhenFileChangesDuringInitialLoad|TestReloaderCurrentClonesTurnSandboxPolicy|TestPollKeepsReloadErrorVisibleAfterTrackerSuccess|TestRefreshWorkflowRebuildsDependenciesFromFactories|TestRefreshWorkflowFactoryFailureDoesNotHalfApply|TestPollSkipsDispatchWhenWorkflowReloadFailsAfterReconcile|TestRefreshWorkflowRetriesFactoryFailureWithoutFileTouch'`：通过。
- **Darwin 判定**：keep。静态 deterministic tests 已覆盖 SPEC reload semantics；真实 operator log proof 不属于本轮目标。
- **Coverage ledger 更新**：`5-6 workflow_config` 从 `partial` 更新为 `covered`。

## Round 2026-05-06: agent_runner.app_server_and_tool_contract - static completion

- **Round goal**：替代 `scenario`，旧信号是 `agent_runner` 因缺 live `linear_graphql` issue 证明仍为 `partial`；本轮按静态目标补齐 missing auth 和 transport failure 测试，并用 fake app-server 固定协议行为。
- **Iteration**：3 static completion。
- **Spec 范围**：`SPEC.md:1115-1138`、`SPEC.md:1210-1220`、`SPEC.md:2020-2035`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；不创建真实 issue，不写真实 Linear。
- **Gap table**：
  - `conforms`：`internal/service/codex/runner.go:279-417` 覆盖 initialize、`thread/start.dynamicTools`、`turn/start`、event forwarding、tool call response 和 input/approval fail-fast。
  - `conforms`：`internal/service/codex/dynamic_tool.go:24-85`、`122-229` 覆盖 `linear_graphql` tool spec、configured Linear client execution、exactly-one-operation validation 和 structured failure payload。
  - `patch`：`internal/service/codex/dynamic_tool_test.go:68-95` 增加 missing auth 与 transport failure 断言，补齐 `SPEC.md:2033-2035`。
  - `conforms`：`internal/service/codex/runner_test.go:245-378` 和 `internal/service/codex/dynamic_tool_test.go:10-160` 覆盖广告、执行、GraphQL errors、invalid args、unsupported tool、missing auth、transport failure 和 unattended approval/input fail-fast。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/service/codex`：通过。
- **Darwin 判定**：keep。fake app-server + fake GraphQL client 是本轮静态合同测试，不产生外部 Linear 副作用。
- **Coverage ledger 更新**：`10 agent_runner` 和 `12 agent_runner` 从 `partial` 更新为 `covered`。

## Round 2026-05-06: observability.sink_failure - static patch

- **Round goal**：替代 `scenario`，旧信号是 `observability` 仍有 `sink_failure` partial：human log 打不开会让 `logging.New` 返回错误并阻断启动。
- **Iteration**：2 static patch。
- **Spec 范围**：`SPEC.md:1277-1284`。
- **Workflow / Issue**：`WORKFLOW.md` 无 diff；不启动 listener，不跑真实 smoke。
- **Gap table**：
  - `patch`：`internal/runtime/logging/jsonl.go:74-109` 把 `human_file` 视为 best-effort sink；`MkdirAll` 或 `OpenFile` 失败时不关闭 JSONL 主 sink，而是写 `log_sink_failed` warn event。
  - `conforms`：`internal/runtime/logging/jsonl_test.go:131-172` 证明坏 human log 路径下 `New` 仍成功，JSONL 记录 `log_sink_failed` 和后续 `state_changed`，console 也显示 warning 与后续事件。
  - `conforms`：JSONL 主 sink 仍是必需本地结构化日志；如果主 sink 自身打不开，`New` 仍返回错误。
- **已修复内容**：optional human log sink 失败不再阻止 orchestration 在 JSONL 可用时启动，并通过剩余 sink 发出 operator-visible warning。
- **验证结果**：
  - `GOCACHE=$PWD/.gocache ./test.sh ./internal/runtime/logging`：通过。
- **Darwin 判定**：keep。改动只改变 optional display sink 的失败语义，符合 SPEC 13.2 的 should-continue 行为。
- **Coverage ledger 更新**：`13 observability` 从 `partial` 更新为 `covered`。
