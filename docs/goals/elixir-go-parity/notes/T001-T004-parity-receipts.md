# Elixir -> Go parity receipts

## T001 Scout

结论：当前 Go 实现已经覆盖 SPEC 必需主链路；Elixir 参考实现仍有若干扩展/治理能力未完全进入 Go，其中 SSH worker extension 是最小、最清晰、最适合作为第一阶段的缺口。

### 已覆盖或已有等价实现

- 主调度/运行态/工作区/Codex app-server/Linear/HTTP control/TUI/日志观测已经在 Go 主体中实现；SPEC ledger 的 required checklist 已标为符合。
- Liquid prompt rendering、dispatch stale recheck、blocker refresh 属于前序 parity loop 已吸收的 Go 行为，不作为本阶段重复实现项。

### 缺口清单

- SSH worker extension：Elixir 有 `SymphonyElixir.SSH.run/3`、`start_port/3`、`remote_shell_command/1`、`SYMPHONY_SSH_CONFIG`、host:port/IPv6 解析和 quote 规则；Go ledger 仍记录 SSH host pool/remote workspace/host-aware scheduler 未实现。证据：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/ssh.ex:4-99`、`ref/elixir-symphony-example/elixir/test/symphony_elixir/ssh_test.exs:6-163`、`SPEC.md:2119-2179`、`docs/spec-conformance/conformance-ledger.md` 的 CHK-019-001-A。
- Config schema 扩展：Elixir schema 包含 `tracker.assignee`、`worker.ssh_hosts`、`worker.max_concurrent_agents_per_host`、`observability.*`、`server.host`；Go workflow config 当前没有 worker/observability/tracker assignee 字段，server workflow config 只保存 `port`。证据：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/config/schema.ex:50-62`、`:103-119`、`:250-273`、`:368-373`，`internal/runtime/config/types.go:10-19`、`:27-50`。
- Tracker adapter 扩展：Elixir 支持 `memory` tracker adapter 与 write APIs；Go 当前 runtime config 校验只接受 Linear，控制面有自己的 issue operations，但没有 workflow-level memory tracker parity。证据：`ref/elixir-symphony-example/elixir/lib/symphony_elixir/tracker.ex:8-44`、`internal/runtime/config/config.go:13-24`、`:169-172`。
- Phoenix Live dashboard：Elixir 有 Phoenix observability endpoint、LiveView dashboard、PubSub 和 snapshot tests；Go 已有 HTTP JSON/control、plain root dashboard 和 TUI，SPEC 把 richer UI 归为 optional observability/control surface，不作为第一阶段必需复制项。
- Mix tasks/governance：Elixir `pr_body.check`、`specs.check`、`workspace.before_remove` 属于仓库治理或 lifecycle 辅助，需按 Go 当前 workflow/CI 语义逐项判断，不适合作为第一阶段切片。

## T002 Judge

决策：第一阶段只实现 SSH helper parity，不接入 orchestrator 调度。

理由：

- 这是 Elixir 行为边界最独立的缺口，能用 fake `ssh` 完整验证命令构造、host:port 解析、quote 和 missing executable。
- 它是 SPEC Appendix A SSH worker extension 的前置能力，但不强迫当前 Go 架构一次性引入远端 workspace、host-aware scheduling 或 retry/failover 语义。
- 不触碰现有 config/orchestrator/codex runner，风险和 blast radius 最小。

允许文件：

- `internal/service/ssh/ssh.go`
- `internal/service/ssh/ssh_test.go`
- `docs/goals/elixir-go-parity/state.yaml`
- `docs/goals/elixir-go-parity/notes/T001-T004-parity-receipts.md`

验证：

- `./test.sh ./internal/service/ssh`
- `git diff --check`

stop_if：

- 需要修改 orchestrator/codex runner 才能让 helper 测试通过。
- SSH 行为与 SPEC Appendix A 或 Elixir 测试锚点冲突。
- 需要 stage unrelated dirty work。

## T003 Worker

实现：

- 新增 Go `internal/service/ssh` 包。
- 复刻 Elixir `run/3` 的 fake-ssh 可测行为，返回 stdout 和 exit code；非零 exit 保留为 process status，不当成 helper 启动错误。
- 新增 `StartPort`，为后续 Codex over SSH stdio 预留进程/stdin/stdout 边界。
- 新增 `Args`、`ParseTarget`、`RemoteShellCommand`，覆盖 `SYMPHONY_SSH_CONFIG`、`-T`、`-p`、bracketed IPv6、unbracketed IPv6、`user@host:port` 和单引号 shell escaping。

验证：

- PASS: `./test.sh ./internal/service/ssh`

## T004 Judge

slice_complete: true

full_outcome_complete: false

仍需后续切片：

- workflow config 增加 `worker.*` 并校验正整数 per-host cap。
- Codex runner 支持通过 SSH stdio 启动 app-server。
- Orchestrator 支持 host pool/capacity/ownership/cleanup observability。
- 再判断 `tracker.assignee`、memory tracker、observability config、Phoenix dashboard/governance tasks 的 Go parity 边界。

## T005 Worker

实现了两个后续小切片：

- `worker` workflow config：新增 `worker.ssh_hosts` 和 `worker.max_concurrent_agents_per_host`，默认空 host list，校验 blank host 和非正 per-host cap，workflow reloader 深拷贝 host slice。
- `tracker.assignee` routing：新增 config/env 解析，Linear issue assignee 归一化，`me` 通过 viewer query 解析；候选 issue 带 `AssignedToWorker` 标记，dispatch eligibility 跳过 assigned-away issue，running reconcile 发现 issue 改派后取消 worker。

Commits:

- `1afe971 feat: parse SSH worker config`
- `e5d7ace feat: honor Linear assignee routing`

验证：

- PASS: `./test.sh ./internal/runtime/config ./internal/service/workflow`
- PASS: `./test.sh ./internal/runtime/config ./internal/integration/linear ./internal/service/orchestrator`
- PASS: `./build.sh`
- PASS: `git diff --check`

## T006 Worker

实现了 SSH stdio app-server 启动切片：

- `codex.SessionRequest` 新增 `WorkerHost`，设置后 `Runner` 通过 `internal/service/ssh.StartPort` 启动远端 app-server，而不是本地 `bash -lc`。
- 远端启动命令与 Elixir 参考保持同形：`cd <workspace> && exec <codex.command>`，并复用 SSH helper 的 `host:port`、`SYMPHONY_SSH_CONFIG`、shell quote 规则。
- session/turn result 与事件 payload 带回 `worker_host`，便于后续 scheduler/observability 使用。
- remote turn sandbox roots 只绑定远端 workspace，不从本机路径推导 `.git` metadata roots，避免把本机 checkout 假设泄漏到远端 worker。

未做：

- 未实现 host pool 选择、per-host capacity、远端 workspace lifecycle、retry/failover 或 cleanup observability；这些仍需要单独切片。

验证：

- PASS: `./test.sh ./internal/service/codex`
- PASS: `git diff --check`

## T007 Judge

结论：剩余 Elixir 差异中，`memory` tracker、Phoenix/LiveView 形态、Mix task 形态不是当前 Go required parity；它们应作为 optional/future/out-of-scope 记录，而不是在没有新 PRD 的情况下硬搬。

判定：

- `memory` tracker：Elixir 用 `Application` env 注入测试/本地开发 issue，并发送 comment/state update 消息；Go 当前 `SPEC.md` 明确 `tracker.kind` 当前为 `linear`，future TODO 才是 pluggable tracker adapters beyond Linear。因此不新增无数据协议的 Go `memory` tracker。
- Phoenix/LiveView dashboard：Elixir 的 Phoenix dashboard 是具体 UI 形态；Go `SPEC.md` 把 status surface/HTTP dashboard 定义为 optional 且 implementation-defined，Go 已有 TUI、HTTP control surface 和 `/` human-readable dashboard，不需要迁移 Phoenix stack。
- Mix tasks：`mix pr_body.check`、`mix specs.check`、`mix workspace.before_remove` 是 Elixir 仓治理入口；Go 当前等价边界是 `./test.sh`、`./build.sh`、repo skills、以及 `hooks.before_remove` lifecycle hook，不迁移 Mix task 形态。
- SSH extension：这是 optional extension，但已分阶段吸收 helper/config/stdio app-server launch；仍未完成 host pool、remote workspace lifecycle、capacity、retry/failover、cleanup observability。

证据：

- `SPEC.md:140`：当前 specification version 的 issue tracker API 是 Linear。
- `SPEC.md:583` / `SPEC.md:1953`：`tracker.kind` 当前为 `linear`，conformance 校验也只要求支持 Linear。
- `SPEC.md:1306-1363` / `SPEC.md:1383-1389`：human-readable status surface 和 HTTP dashboard 是 optional/implementation-defined。
- `SPEC.md:390-402` / `SPEC.md:872-894`：Go 的 workspace hook contract 覆盖 `before_remove` lifecycle 语义。
- `SPEC.md:2119-2179`：SSH worker extension 是 optional，但定义了完整 host/workspace/scheduler/observability 风险边界。
- `docs/contract-scope.md:26-30`：Go 不把 rich web UI 或 distributed job scheduler 纳入 core service boundary。

## T900 PM

已按阶段提交并只 stage 当前切片相关文件：

- `0c400c6 feat: add SSH worker helper`
- `1afe971 feat: parse SSH worker config`
- `e5d7ace feat: honor Linear assignee routing`

保留未纳入 commit 的既有脏区：

- `lesson.md`
- `.codex/skills/pet-prd-issue-run/`
- `.codex/skills/spec-conformance-loop/`
- `.idea/`
- `docs/goals/`

## T999 Final Audit

complete: complete

full_outcome_complete: true

完成标准对照：

- Source-backed inventory：T001 建立了 Elixir feature inventory，T005/T006/T007 继续按 live SPEC 复核和裁剪。
- Required/core Go behavior：主调度、工作区、Codex app-server、Linear tracker、HTTP/TUI/logging、Liquid prompt、stale dispatch revalidation、blocker refresh 等已有实现或前序 parity commit 证据。
- Implemented safe slices：T003/T005/T006 已用 staged commits/验证覆盖 SSH helper、worker config、Linear assignee routing、Codex SSH stdio app-server launch。
- Explicitly equivalent/out-of-scope：T007 将 `memory` tracker、Phoenix/LiveView 形态、Mix task 形态裁剪为 optional/future/out-of-scope 或 Go 已有等价边界。
- Remaining SSH host-pool semantics：`SPEC.md` Appendix A 明确为 OPTIONAL；本目标不继续实现 remote workspace lifecycle、host pool/capacity、retry/failover、cleanup observability，后续如要启用应走 `GAP-ssh-worker-extension-001` 或单独 PRD。

已复刻：

- SSH helper 底层进程/参数/quote 行为。
- SSH worker config schema 前置能力。
- Linear assignee worker routing。
- Codex app-server SSH stdio 启动入口。

裁剪/等价确认：

- `memory` tracker adapter：当前 SPEC 只支持 Linear，pluggable tracker adapters 是 future TODO。
- Phoenix LiveView dashboard：Go 已有 optional TUI/HTTP dashboard surface，不迁移 Phoenix stack。
- Mix tasks：Go 以 repo scripts/skills/hooks 覆盖对应治理和 lifecycle 边界，不迁移 Mix task 形态。
- SSH host pool/capacity/retry/failover/cleanup observability 与 remote workspace lifecycle：Appendix A optional extension 剩余项，已记录为 `GAP-ssh-worker-extension-001`，不作为当前 required parity 继续实现。

最终验证：

- PASS: `./test.sh ./internal/service/codex`
- PASS: `./build.sh`
- PASS: `git diff --check`

Commits:

- `0c400c6 feat: add SSH worker helper`
- `1afe971 feat: parse SSH worker config`
- `e5d7ace feat: honor Linear assignee routing`
- `47c50e2 feat(parity): 支持 Codex SSH stdio 启动`
