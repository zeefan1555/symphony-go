# symphony-go 架构与运行原理

本文面向后续维护者，说明本仓如何从 Linear issue 驱动工作流，创建 per-issue worktree，调用 Codex app-server，执行 AI Review/Rework/Merging，并通过日志和验证命令证明运行结果。ZEE-41 执行时，根目录 `README.md` 不存在；`AGENTS.md:3` 也明确 `README.md` 只是项目说明参考，AI 执行规则优先看 `AGENTS.md`。

## 总体架构

Symphony Go 是一个长驻式 issue runner：它读取仓库内 `WORKFLOW.md`，用 Linear API 拉取 active issue，把每个 issue 映射到独立 workspace，再在该 workspace 中启动 Codex app-server session。SPEC 把核心模块拆成 Workflow Loader、Config Layer、Issue Tracker Client、Orchestrator、Workspace Manager、Agent Runner、Status Surface 和 Logging（`SPEC.md:71`、`SPEC.md:83`、`SPEC.md:89`、`SPEC.md:95`、`SPEC.md:101`、`SPEC.md:107`、`SPEC.md:111`）。

当前实现中的模块边界如下：

- 启动入口在 `cmd/symphony-go/main.go:100`，只支持 `symphony-go run`。
- workflow 解析在 `internal/workflow/workflow.go:18`，动态 reload 在 `internal/workflow/reloader.go:73`。
- 配置默认值、环境变量解析和校验在 `internal/config/config.go:42`、`internal/config/config.go:57`、`internal/config/config.go:108`、`internal/config/config.go:172`。
- Linear 适配器在 `internal/linear/client.go:18`、`internal/linear/client.go:133`、`internal/linear/client.go:200`。
- 调度状态机在 `internal/orchestrator/orchestrator.go:127`、`internal/orchestrator/orchestrator.go:160`、`internal/orchestrator/orchestrator.go:796`。
- workspace 生命周期在 `internal/workspace/workspace.go:190`、`internal/workspace/workspace.go:242`。
- Codex app-server 调用在 `internal/codex/runner.go:85`、`internal/codex/runner.go:156`、`internal/codex/runner.go:304`。
- 结构化日志和人类可读日志在 `internal/logging/jsonl.go:74`、`internal/logging/jsonl.go:163`、`internal/logging/jsonl.go:214`。

整体数据流是：

1. `Makefile:27` / `Makefile:31` 通过 `bin/symphony-go run --workflow ...` 启动服务。
2. `main` 加载 workflow、初始化 Linear client、workspace manager、Codex runner、logger 和 orchestrator（`cmd/symphony-go/main.go:110`、`cmd/symphony-go/main.go:115`、`cmd/symphony-go/main.go:132`、`cmd/symphony-go/main.go:140`、`cmd/symphony-go/main.go:142`）。
3. orchestrator 每个 poll tick 先 reload workflow，再 reconcile running issue，然后从 Linear 拉 active issue 并派发 eligible issue（`internal/orchestrator/orchestrator.go:131`、`internal/orchestrator/orchestrator.go:160`、`internal/orchestrator/orchestrator.go:166`、`internal/orchestrator/orchestrator.go:181`）。
4. worker 确保 workspace，渲染 prompt，启动 Codex session，跟踪 Codex 事件和 token，再根据 commit/review 结果推动状态（`internal/orchestrator/orchestrator.go:824`、`internal/orchestrator/orchestrator.go:846`、`internal/orchestrator/orchestrator.go:860`、`internal/orchestrator/orchestrator.go:903`、`internal/orchestrator/orchestrator.go:927`）。

## 启动入口

CLI 入口只接受 `run` 子命令；默认 workflow 是 `./WORKFLOW.md`，默认 TUI 在连续运行时开启，`--once` 默认关闭 TUI（`cmd/symphony-go/main.go:35`、`cmd/symphony-go/main.go:43`、`cmd/symphony-go/main.go:64`、`cmd/symphony-go/main.go:100`）。`Makefile` 把常用命令收敛成稳定入口：`make build` 调 `./build.sh`，`make test` 调 `./test.sh ./...`，`make run-once ISSUE=ZEE-41` 会构建后执行单轮 poll（`Makefile:21`、`Makefile:24`、`Makefile:31`）。

`build.sh` 和 `test.sh` 都先切到脚本所在 repo root，再执行 Go 命令，并默认使用 external link mode，避免本机裸 `go run` 临时二进制兼容性问题（`build.sh:4`、`build.sh:8`、`test.sh:4`、`test.sh:12`）。`AGENTS.md` 要求验证 Go 代码优先使用这两个根目录入口；文档或脚本改动至少运行 `git diff --check`（`AGENTS.md:106`、`AGENTS.md:118`）。

## 配置与 workflow 解析

`WORKFLOW.md` 分成 YAML front matter 和 Markdown prompt body。当前 front matter 指定 Linear tracker、active/terminal states、poll interval、workspace root、hooks、merge target、agent review policy 和 Codex app-server 配置（`WORKFLOW.md:1`、`WORKFLOW.md:6`、`WORKFLOW.md:13`、`WORKFLOW.md:19`、`WORKFLOW.md:21`、`WORKFLOW.md:23`、`WORKFLOW.md:32`、`WORKFLOW.md:34`、`WORKFLOW.md:41`）。正文是注入给 Codex 的 per-issue SOP，并通过模板变量填入 issue 字段（`WORKFLOW.md:54`、`WORKFLOW.md:65`、`WORKFLOW.md:72`）。

解析路径是：

- `workflow.Load` 读取文件、切分 front matter、用 YAML 解码 `types.Config`，再调用 `config.Resolve` 生成有效配置（`internal/workflow/workflow.go:18`、`internal/workflow/workflow.go:23`、`internal/workflow/workflow.go:30`）。
- `workflow.Render` 支持 `{{ issue.identifier }}` 等变量，以及 `{% if attempt %}` 重试区块（`internal/workflow/workflow.go:37`、`internal/workflow/workflow.go:78`）。
- `config.Resolve` 先套默认值，再解析 `$VAR`，规范化状态和 merge target，最后把相对 `workspace.root` 解析到 workflow 所在目录下的绝对路径（`internal/config/config.go:42`、`internal/config/config.go:57`、`internal/config/config.go:108`、`internal/config/config.go:141`）。
- `config.validate` 当前只支持 `tracker.kind=linear`，要求 Linear API key、project slug、非空 Codex command、正数 poll/hook/max-turn/retry 配置，并校验 `agent.review_policy.mode` 只能是 `human|ai|auto`（`internal/config/config.go:172`、`internal/config/config.go:197`、`internal/config/config.go:203`）。

动态 reload 是运行期能力：`workflow.Reloader` 会在初次加载时要求文件信息稳定，poll 前调用 `ReloadIfChanged` 检测 mtime/size，成功加载后只把新 workflow 放到 pending，等 orchestrator 重新创建依赖并 `CommitCandidate`（`internal/workflow/reloader.go:25`、`internal/workflow/reloader.go:42`、`internal/workflow/reloader.go:73`、`internal/workflow/reloader.go:96`、`internal/orchestrator/orchestrator.go:473`、`internal/orchestrator/orchestrator.go:504`）。SPEC 对此要求是 workflow 变更必须在不重启的情况下应用到后续 dispatch、retry、reconcile、hook 和 agent launch（`SPEC.md:532`、`SPEC.md:541`）。

## Linear 轮询与状态流转

Linear client 通过 GraphQL 查询项目内指定状态的 issue，并把 label、blocked-by、时间戳等字段归一化到 `types.Issue`（`internal/linear/client.go:18`、`internal/linear/client.go:133`、`internal/linear/client.go:386`、`internal/linear/client.go:394`）。更新状态时先按 issue team 查目标 state id，再调用 `issueUpdate`（`internal/linear/client.go:68`、`internal/linear/client.go:79`、`internal/linear/client.go:200`、`internal/linear/client.go:323`）。同一个 client 也有 `CreateComment` / `UpsertWorkpad`，可用 `commentCreate` / `commentUpdate` 维护 `## Codex Workpad`（`internal/linear/client.go:84`、`internal/linear/client.go:89`、`internal/linear/client.go:98`、`internal/linear/client.go:221`、`internal/linear/client.go:238`）。

调度循环在 `Orchestrator.Run` 中执行：每轮设置 polling 状态，调用 `pollDispatched`，`--once` 时等待本轮派发的 worker 结束，否则按 `polling.interval_ms` 继续下一轮（`internal/orchestrator/orchestrator.go:127`、`internal/orchestrator/orchestrator.go:131`、`internal/orchestrator/orchestrator.go:140`、`internal/orchestrator/orchestrator.go:143`）。`pollDispatched` 每轮先 reload workflow、reconcile running issue，再拉 active issues 并按优先级排序派发（`internal/orchestrator/orchestrator.go:160`、`internal/orchestrator/orchestrator.go:163`、`internal/orchestrator/orchestrator.go:166`、`internal/orchestrator/orchestrator.go:175`）。

派发前的 eligibility 检查要求 issue 字段完整、状态属于 active 且不属于 terminal、不在 human review hold、不重复 claimed/running、没有未完成 blocker，并且有全局和 per-state 并发槽位（`internal/orchestrator/eligibility.go:25`、`internal/orchestrator/eligibility.go:29`、`internal/orchestrator/eligibility.go:32`、`internal/orchestrator/eligibility.go:38`、`internal/orchestrator/eligibility.go:46`、`internal/orchestrator/eligibility.go:53`）。

状态流转由 worker 和 workflow prompt 共同完成：

- `Todo` 会由 orchestrator 自动更新为 `In Progress`（`internal/orchestrator/orchestrator.go:800`、`internal/orchestrator/orchestrator.go:802`）。
- `Human Review` / `In Review` 被视为人工等待状态，不启动 Codex worker（`internal/orchestrator/orchestrator.go:808`）。
- `AI Review` 会走机器复核路径（`internal/orchestrator/orchestrator.go:812`、`internal/orchestrator/orchestrator.go:979`）。
- 当前 `WORKFLOW.md` 把默认目标流程定义为 `In Progress -> AI Review -> Merging -> Done`，并明确 `Human Review` 只用于真实外部 blocker（`WORKFLOW.md:123`、`WORKFLOW.md:124`、`WORKFLOW.md:128`、`WORKFLOW.md:136`）。

## workspace 与 worktree 生命周期

SPEC 要求 per-issue workspace 路径为 `<workspace.root>/<sanitized_issue_identifier>`，workspace 会跨 run 复用，成功 run 不自动删除（`SPEC.md:820`、`SPEC.md:828`、`SPEC.md:832`）。实现里 `workspace.Manager.PathForIssue` 用 `SafeIdentifier` 计算路径，`Ensure` 创建 root 和 issue directory，且只在新建目录时执行 `after_create` hook（`internal/workspace/workspace.go:80`、`internal/workspace/workspace.go:190`、`internal/workspace/workspace.go:205`、`internal/workspace/workspace.go:234`、`internal/workspace/workspace.go:273`）。

安全边界在 workspace 层做：`ValidateWorkspacePath` 同时做 lexical 检查和 symlink-aware 检查，确保 workspace 不逃逸 root；hook 执行也先校验路径（`internal/workspace/workspace.go:91`、`internal/workspace/workspace.go:119`、`internal/workspace/workspace.go:136`、`internal/workspace/workspace.go:170`、`internal/workspace/workspace.go:180`）。SPEC 对同一不变量的表述是 agent 只能在 per-issue workspace 运行、workspace 路径必须 stay inside root、identifier 只能保留 `[A-Za-z0-9._-]`（`SPEC.md:900`、`SPEC.md:905`、`SPEC.md:911`）。

本仓把 VCS/bootstrap 放在 workflow hook 和脚本里，而不是硬编码进 workspace manager。当前 `WORKFLOW.md` 的 `after_create` 会从 workspace 推导 repo root 并调用 `scripts/symphony_after_create.sh`，`before_remove` 调用 `scripts/symphony_before_remove.sh`（`WORKFLOW.md:23`、`WORKFLOW.md:28`）。`symphony_after_create.sh` 先回到 repo root，删除占位 workspace，然后创建或复用 `symphony-go/<issue>` 本地分支的 git worktree，并设置 origin remote；`symphony_before_remove.sh` 则从 repo root 执行 `git worktree remove --force`（`scripts/symphony_after_create.sh:4`、`scripts/symphony_after_create.sh:11`、`scripts/symphony_after_create.sh:14`、`scripts/symphony_after_create.sh:20`、`scripts/symphony_before_remove.sh:4`、`scripts/symphony_before_remove.sh:8`）。

terminal cleanup 有两条路径：启动时清理所有 terminal state issue 的 workspace，运行中发现 running issue 变 terminal 时先 cancel，再在 worker 退出后清理（`internal/orchestrator/orchestrator.go:242`、`internal/orchestrator/orchestrator.go:339`、`internal/orchestrator/orchestrator.go:637`、`internal/orchestrator/orchestrator.go:670`）。

## Codex app-server 调用

Codex runner 从 `types.CodexConfig` 读取命令、approval policy、thread sandbox、turn sandbox policy 和 timeout 等字段（`internal/types/types.go:98`）。不要把某个具体模型写进流程语义；当前流程只把模型选择当作 `codex.command` 的一部分（`WORKFLOW.md:41`、`WORKFLOW.md:85`）。

`Runner.RunSession` 是一次 app-server session 的主入口：它要求至少一个 prompt，启动 app-server 后复用同一个 thread 逐 turn 执行，必要时通过 `AfterTurn` 追加 continuation turn（`internal/codex/runner.go:85`、`internal/codex/runner.go:102`、`internal/codex/runner.go:144`、`internal/codex/runner.go:151`）。`startSession` 用 `bash -lc <codex.command>` 在 workspace 目录启动进程，初始化 app-server，随后 `thread/start`（`internal/codex/runner.go:156`、`internal/codex/runner.go:157`、`internal/codex/runner.go:190`、`internal/codex/runner.go:258`、`internal/codex/runner.go:279`）。

每个 turn 通过 `turn/start` 发送 prompt、cwd、title、approvalPolicy 和 sandboxPolicy；title 是 `<issue.identifier>: <issue.title>`（`internal/codex/runner.go:304`、`internal/codex/runner.go:309`、`internal/codex/runner.go:312`）。workspace-write 时，runner 会把 workspace path 和 git metadata roots 都加入 writable roots，避免 agent 能改文件却不能写 worktree git metadata（`internal/codex/runner.go:199`、`internal/codex/runner.go:211`、`internal/codex/runner.go:214`、`internal/codex/runner.go:459`）。

`awaitTurn` 持续读取 app-server JSON 行，遇到 `turn/completed` 成功返回，`turn/failed` / `turn/cancelled` 返回错误；如果 Codex 请求交互式 MCP approval，则直接失败，避免无人值守 run 卡住（`internal/codex/runner.go:331`、`internal/codex/runner.go:356`、`internal/codex/runner.go:361`）。SPEC 对 `linear_graphql` 的定位是可选 client-side tool：复用 Symphony 的 Linear auth 执行单个 GraphQL operation，不能要求 agent 自己读取 token（`SPEC.md:1057`、`SPEC.md:1066`、`SPEC.md:1089`）。

## AI Review、Rework 与 Merging

worker 的 handoff 不是单纯看 Codex turn 是否完成，而是比较 turn 前后的 git HEAD。`runAgentWith` 在 workspace 准备后记录 `baseHead`，turn 完成后 `handoffAfterTurn` 读取当前 HEAD；如果 HEAD 没变，继续留在 active flow，不移动 review 状态（`internal/orchestrator/orchestrator.go:839`、`internal/orchestrator/orchestrator.go:865`、`internal/orchestrator/orchestrator.go:927`、`internal/orchestrator/orchestrator.go:933`）。

review policy 的唯一语义入口是 `agent.review_policy.mode`。实现里 `effectiveReviewPolicy` 把空 mode 兼容到 legacy `ai_review`，非 `ai|auto` 的 mode 会降到 human；`mode=auto` 时，AI Review 通过后的 next state 是 `Merging`（`internal/orchestrator/orchestrator.go:1040`、`internal/orchestrator/orchestrator.go:1046`、`internal/orchestrator/orchestrator.go:1090`、`internal/orchestrator/orchestrator.go:1094`）。当前 workflow 配置为 `mode: auto`、`on_ai_fail: rework`（`WORKFLOW.md:37`、`WORKFLOW.md:40`）。

AI Review 的实际检查是轻量机器门禁：

- worktree 必须没有未提交变更（`internal/orchestrator/orchestrator.go:1130`）。
- 如果能拿到 base/head，运行 `git diff --check base..head`（`internal/orchestrator/orchestrator.go:1134`）。
- 如果 policy 配了 expected files，则 HEAD 变更文件必须完全匹配（`internal/orchestrator/orchestrator.go:1140`）。

`handoffAfterTurn` 在有 commit 且 policy 要求机器复核时，先把 issue 移到 `AI Review`，执行门禁；通过后移到 `Merging`，失败且 `on_ai_fail=rework` 时移到 `Rework`（`internal/orchestrator/orchestrator.go:940`、`internal/orchestrator/orchestrator.go:941`、`internal/orchestrator/orchestrator.go:949`、`internal/orchestrator/orchestrator.go:957`）。如果 issue 本来就在 `AI Review`，`reviewIssueState` 会基于 `HEAD~1..HEAD` 重新复核，通过后进入 next state；当 next state 是 `Merging` 时，会立即以 `Merging` 状态再次调用 `runAgentWith`（`internal/orchestrator/orchestrator.go:979`、`internal/orchestrator/orchestrator.go:989`、`internal/orchestrator/orchestrator.go:999`、`internal/orchestrator/orchestrator.go:1007`）。

Merging 阶段没有硬编码 PR land 或独立 merge skill；代码会继续用同一份 workflow prompt 让 agent 执行阶段 SOP。测试明确覆盖了 `Merging` 状态使用 workflow prompt，且不应注入 `.codex/skills/land/SKILL.md`（`internal/orchestrator/orchestrator_test.go:1479`、`internal/orchestrator/orchestrator_test.go:1501`、`internal/orchestrator/orchestrator_test.go:1507`、`internal/orchestrator/orchestrator_test.go:1510`）。当前 `WORKFLOW.md` 的 Merging 协议要求直接把 issue worktree 分支合入 repo root 的本地 `main`，验证后 `git push origin main`，不创建 PR（`WORKFLOW.md:198`、`WORKFLOW.md:207`、`WORKFLOW.md:208`、`WORKFLOW.md:210`、`WORKFLOW.md:211`、`WORKFLOW.md:219`）。

## 日志、TUI 与验证路径

运行日志默认落在 workflow 所在目录的 `.symphony/logs/run-<timestamp>.jsonl`，同时生成同名 `.human.log`（`cmd/symphony-go/main.go:124`、`cmd/symphony-go/main.go:125`、`internal/logging/jsonl.go:163`、`internal/logging/jsonl.go:168`）。JSONL 记录完整事件，human log 会过滤部分低价值事件，并把 Codex protocol 事件转成 `codex_session_started`、`codex_turn_started`、`codex_turn_completed`、`codex_plan`、`codex_diff`、`codex_command`、`codex_file_change` 等人类可读事件（`internal/logging/jsonl.go:117`、`internal/logging/jsonl.go:214`、`internal/logging/jsonl.go:241`、`internal/logging/jsonl.go:253`、`internal/logging/jsonl.go:321`、`internal/logging/jsonl.go:341`）。

runtime snapshot 维护 running、retrying、token totals、rate limits、polling 状态和 last error，TUI 每秒渲染一次 snapshot，展示 active agents、token、rate limits、backoff queue 和 next refresh（`internal/observability/snapshot.go:5`、`internal/tui/dashboard.go:19`、`internal/tui/dashboard.go:27`、`internal/tui/dashboard.go:71`、`cmd/symphony-go/main.go:164`、`cmd/symphony-go/main.go:183`）。Codex 事件进入 orchestrator 后会更新 running entry 的 last event、session id、pid、token delta 和 rate limit（`internal/orchestrator/orchestrator.go:1307`、`internal/orchestrator/orchestrator.go:1325`、`internal/orchestrator/orchestrator.go:1332`、`internal/observability/token.go:5`）。

常用验证入口如下：

- 构建：`make build`，对应 `Makefile:21` 和 `build.sh:11`。
- 全量测试：`make test`，对应 `Makefile:24` 和 `test.sh:14`。
- 单 issue 本地运行：`make run-once ISSUE=ZEE-41`，对应 `Makefile:31`。
- 文档/脚本类最小门禁：`git diff --check`，这是 `AGENTS.md:118` 要求，也是本 ticket 指定验证项。
- 本 ticket 还要求用 `rg -n "总体架构|运行原理|Linear|workspace|Codex|AI Review|Merging" docs/architecture/symphony-go-architecture.md` 证明文档覆盖关键主题。

`scripts/smoke_metrics.py` 可以从 `.symphony/logs/run-*.jsonl` 提取状态流、token、AI Review、merge 时长、workpad 更新次数和 changed files，并追加到 TSV/Markdown 实验记录（`scripts/smoke_metrics.py:228`、`scripts/smoke_metrics.py:239`、`scripts/smoke_metrics.py:245`、`scripts/smoke_metrics.py:279`、`scripts/smoke_metrics.py:293`、`scripts/smoke_metrics.py:326`、`scripts/smoke_metrics.py:360`）。

## 维护注意事项

- 本仓长期规则入口是 `AGENTS.md`，README 不应作为 AI 执行规则的唯一来源（`AGENTS.md:91`、`AGENTS.md:95`）。
- 新增行为应优先改 workflow/config/agent prompt 还是 Go 代码，要先看边界：ticket 写入和 PR/comment 业务逻辑通常在 workflow prompt 和 agent tooling 中，调度、workspace、Codex protocol、observability 才在 Go 实现里（`SPEC.md:36`、`SPEC.md:63`）。
- documentation-only 任务不要修改 Go 行为代码；当前最小证明是只新增/更新 `docs/architecture/symphony-go-architecture.md`，并运行 ticket 指定的三个验证命令。
