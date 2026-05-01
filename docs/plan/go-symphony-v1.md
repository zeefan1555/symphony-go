# Go Symphony v1 实现计划

> **给 agentic worker 的要求：** 实现本计划时必须使用 `superpowers:executing-plans`，按任务逐步执行。

**目标：** 在 `/Users/bytedance/symphony/go` 下实现第一个 Go 版本 Symphony。v1 里程碑要跑通最小但真实的端到端流程：`Linear -> worktree -> Codex app-server -> 本地 review/merge -> Done`。

**架构：** 单进程 Go daemon，组件保持小而明确：workflow loader、Linear tracker、orchestrator、workspace manager、Codex app-server runner、local merge handler、structured logs。

**技术栈：** Go 1.22+，优先使用 Go 标准库；`gopkg.in/yaml.v3` 用于解析 workflow front matter；Linear 通过 HTTP GraphQL 访问；workspace 使用本地 Git worktree；agent 执行使用真实 `codex app-server` 命令。

---

## 概要

- 在 `/Users/bytedance/symphony/go` 内创建 Go 实现。
- 第一版先交付与当前 Elixir smoke workflow 兼容的最小真实流程。
- v1 使用真实 Codex app-server 路径，不使用 fake runner，也不走简化版 `codex exec` 捷径。
- 中文 / UTF-8 支持是 v1 必需能力，因为早前 Elixir workflow 在中文内容进入 JSON 编码时暴露过失败。
- 完整 TUI、远程 worker、GitHub PR 自动化、复杂恢复机制，以及 `SPEC.md` 的全量能力放到后续迭代。

## 关键改动

### Go 工程骨架

- 新增以 `go/` 为根目录的 Go module。
- 提供 CLI 入口：
  - `go/cmd/symphony-go/main.go`
  - 主命令：`symphony-go run --workflow ../elixir/WORKFLOW.md`
  - smoke 辅助参数：`--once` 和 `--issue ZEE-8`
- 实现包保持小而直接：
  - `go/internal/workflow`
  - `go/internal/linear`
  - `go/internal/orchestrator`
  - `go/internal/workspace`
  - `go/internal/codex`
  - `go/internal/logging`

### Workflow Loader

- 解析带可选 YAML front matter 的 Markdown 文件。
- 将正文 prompt 保留为 template。
- 支持 v1 需要的配置章节：
  - `tracker`
  - `polling`
  - `workspace`
  - `hooks`
  - `agent`
  - `codex`
- 渲染当前 workflow 需要的 template 变量：
  - `issue.identifier`
  - `issue.title`
  - `issue.state`
  - `issue.labels`
  - `issue.url`
  - `issue.description`
  - `attempt`
- template engine 保持最小实现。只支持现有 workflow 需要的 `{{ ... }}` 替换，以及 `{% if attempt %}...{% endif %}` 条件块。

### 中文和 UTF-8 支持

- 所有 prompt 文本、Linear issue 字段、workpad/comment 正文、日志事件、Codex app-server 输入输出，都按 UTF-8 字符串处理。
- 所有 GraphQL 请求/响应体都使用 Go 标准库 `encoding/json` 编解码。禁止手写 byte/string 拼接 JSON。
- Linear GraphQL 请求必须设置：
  - `Content-Type: application/json; charset=utf-8`
  - `Accept: application/json`
- hooks 和 Codex 子进程继承父进程环境，并确保补齐默认值：
  - `LANG=en_US.UTF-8`
  - `LC_ALL=en_US.UTF-8`
- JSONL 日志按 UTF-8 写入。如果终端无法正确显示中文，只允许显示层降级，不得改变已存储的事件文本。
- 增加中文回归测试，覆盖中文 issue 标题、描述、prompt 内容、workpad comment 和日志 payload。

### Linear Tracker

- 优先读取 `tracker.api_key`，缺失时再读取 `LINEAR_API_KEY`。
- 查询配置项目下的 active issues。
- dispatch 前和 agent 运行期间刷新 issue 状态。
- 在 workflow 要求时更新 issue 状态，例如 `Todo -> In Progress`。
- 每个 issue 查找或创建一个持久 workpad comment。
- 后续进度更新都原地更新同一个 workpad comment。
- 状态匹配基于 workflow 配置，不硬编码英文状态名。

### Workspace Manager

- issue workspace 路径为 `<workspace.root>/<sanitized_issue_identifier>`。
- workspace 缺失时创建目录。
- 只有新建 workspace 时才运行 `hooks.after_create`。
- 清理前运行 `hooks.before_remove`。
- 复用现有 hook scripts 创建和清理 Git worktree。
- 不硬编码特定 issue 的 Git metadata 路径，例如 `.git/worktrees/ZEE-8`。

### Codex App-Server Runner

- 启动配置中的 `codex ... app-server` 命令作为子进程。
- 将渲染后的 prompt 发送给 app-server session。
- 当 Linear issue 仍处于 active state 时继续 turn，直到达到 `agent.max_turns`。
- 尽可能记录 session id、turn count、last event、exit reason、token usage。
- 为本地 Git worktree 动态加入 writable sandbox roots：
  - workspace 本身。
  - `git rev-parse --git-dir`
  - `git rev-parse --git-common-dir`
- 不要求 workflow 作者手动列出 `.git/worktrees/<issue>` 这类 per-issue 路径。

### Orchestrator

- 按 `polling.interval_ms` 轮询。
- 只 dispatch active issues，跳过 terminal states。
- 遵守 `agent.max_concurrent_agents`。
- 同一个 issue 同时只允许一个 worker。
- worker 正常退出但 issue 仍处于 active state 时，安排短间隔 continuation retry。
- issue 进入 non-active 或 terminal state 时停止或跳过工作。
- v1 status output 保持简单：每次 poll 输出一条简洁 stdout 快照，同时写 JSONL 日志。

### Local Merge Flow

- issue 进入 `Merging` 状态时，使用 `.codex/skills/local-merge/SKILL.md` 中定义的 local merge 契约。
- 将 issue worktree branch 合入当前外层仓库目标分支，初始目标为 `feat_zff`。
- v1 不创建临时 clone fallback。
- 如果发生 merge conflict，在 workpad 里记录 blocker，并让 issue 保持可 review 的状态，不隐藏冲突。
- local merge 成功后，将 issue 移动到 `Done`。

## 测试计划

### 单元测试

- Workflow parser：
  - 能解析 `elixir/WORKFLOW.md` front matter。
  - 能保留并渲染 prompt template 文本。
  - 能渲染中文 prompt 内容且不产生 byte 损坏。
- Linear client：
  - 请求体使用 `encoding/json` 生成。
  - 请求设置 `Content-Type: application/json; charset=utf-8`。
  - 通过 `httptest` GraphQL server 处理中文 title、description 和 comment 字段。
- Workspace manager：
  - 在临时 repo 中创建 per-issue workspace。
  - `after_create` 只对新 workspace 运行。
  - cleanup 时运行 `before_remove`。
- Codex runner：
  - 测试中启动 fake app-server process。
  - 将中文 prompt 作为合法 UTF-8 发送。
  - 动态发现 Git metadata writable roots。
- Logging：
  - 写入中文 JSONL events。
  - 使用 Go JSON decoder 重新读取这些 JSONL events。

### 集成 Smoke

- 创建一个带中文标题或中文描述的 Linear smoke issue。
- 任务内容可以是只修改 README 文档，例如写入 `zeefan 中文 smoke test`。
- 运行：
  - `symphony-go run --workflow ../elixir/WORKFLOW.md --issue ZEE-8`
- 验证：
  - issue 从 `Todo` 移动到 `In Progress`。
  - 在配置的 workspace root 下创建本地 worktree。
  - Codex app-server 收到渲染后的 prompt。
  - worktree branch 生成本地 commit。
  - 流程进入 review 状态。
  - `Merging` 将 branch 合入 `feat_zff`。
  - issue 最终到达 `Done`。

### 回归验证

- ASCII-only issue 仍能端到端跑通。
- 中文 issue 内容不会触发 JSON encoding failure。
- 日志可以被 `jq` 或 Go JSON decoder 重新读取。
- workflow config 不需要配置 `.git/worktrees/ZEE-8` 这类 per-issue writable root。

## 假设

- v1 优先优化真实 smoke workflow，而不是完整覆盖 `SPEC.md` 的所有功能。
- 当前 Elixir 实现仍然是 orchestration、workspace hooks 和 app-server 使用方式的行为参考。
- 中文支持是 v1 正确性要求，不是未来增强项。
- local merge 初始合入当前外层仓库分支，预期为 `feat_zff`；后续可以配置化。
- 现有 `WORKFLOW.md` hook scripts 仍然是本地 worktree 创建和清理的事实来源。
