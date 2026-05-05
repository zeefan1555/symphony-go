# Lessons

## 2026-05-05: Symphony Go 业务与运行支撑应归入 service-rooted 结构

### 用户纠正

- 用户指出：`/Users/bytedance/symphony-go/internal` 的当前文件结构不符合预期，Symphony Go 的业务逻辑应该放在 `internal/service/...` 下。
- 用户要求回看此前已经确认过的口径，不要把业务逻辑继续散放在 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 或 `internal/control` 这类顶层包里。
- 用户进一步明确：迁移完毕后，原顶层 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 只应作为迁移期兼容 shim 或最终消失；`internal/control/hertz*`、`internal/issuetracker`、`internal/config`、`internal/logging`、`internal/observability` 也不应继续作为顶层目录存在，目标是提升人类可读性。
- 用户明确否定：`internal/generated` 不需要，已经迁移到 `biz` 生成；`internal/types` 也不应长期存在，应该优先复用 `biz/model` 中的数据结构；`adapter` 这个目录名不好，`platform` 也看不出和 `service` 的区别。

### 错误模式

- 这是流程错误：`CONTEXT.md` 已经把 **业务服务层** 定义为 `internal/service/...`，我在后续结构讨论或实现时没有把它当作当前权威约束。
- 这是技术判断错误：把 `internal/service/control` 做成控制面 facade 后，容易误以为 `service` 只承载 HTTP control service，而忽略了它应是 orchestrator、workspace、codex、workflow、control 等核心业务逻辑的统一命名空间。
- 这是结构理解错误：把 config、logging、observability、issue tracker adapter 当成可以永久留在 `internal/` 顶层的基础设施包，会牺牲用户最重视的人类可读性。
- 这是命名判断错误：`adapter`、`platform` 这类通用分层名不够直观，不能清楚表达“外部系统集成”和“本地运行支撑”与业务 `service` 的区别。
- 这是文档一致性错误：`CONTEXT.md` 的“逐步迁入 service”与 `docs/internal-scaffold-hertz-idl.md` 中“第一版不迁移这些包”的旧口径存在张力，继续开发前必须先收敛。

### 防复犯规则

- 新增或迁移 Symphony Go 业务逻辑与运行支撑能力时，默认放入 `internal/service/<domain>/...`；顶层 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 等只允许作为兼容 shim 或待迁移遗留包存在。
- `internal/control/hertz*` 只承载传输适配；迁移完毕后不应继续占用 `internal/control` 顶层目录，HTTP handler 和 adapter 必须委托到 `internal/service/...` 内的协议无关服务。
- `internal/issuetracker/...`、`internal/config`、`internal/logging`、`internal/observability` 不应作为长期顶层目录存在；如果功能仍需要，应迁入 service-rooted 结构下的清晰子域。
- `internal/generated` 已被主 Hertz `biz/` 生成目录取代，迁移完毕后不应继续存在；当前残留的 `internal/generated/hertz/scaffold` 生成链需要先退役或迁到新的可读边界。
- `internal/types` 不应作为长期共享模型目录存在；删除前必须先区分可并入 `biz/model` 的控制面数据结构，以及仍需要本地 workflow/config 语义的运行时结构。
- 目录命名优先选人类一眼能读懂职责的词；不要默认使用 `adapter`、`platform` 这类过泛名称。
- `internal/hertzcontract`、`internal/tui`、`internal/app` 是否保留在 service 外，需要在迁移计划中逐个确认，不要默认扩大或遗漏。
- 迁移结构时必须先写清分批计划和 import 兼容策略，不要一次性大搬家导致行为回归或循环依赖。

### 固定动作

- 开始结构类改动前先确认当前边界：

```bash
rg -n "业务服务层|internal/service|逐步迁入|第一版不迁移" CONTEXT.md docs lesson.md
find internal -maxdepth 2 -type d | sort
```

- 新增或迁移逻辑前先检查是否误加到顶层包：

```bash
rg -n "github.com/zeefan1555/symphony-go/internal/(orchestrator|workspace|codex|workflow|control|issuetracker|config|logging|observability)" internal cmd biz docs
```

- 文档或迁移计划改动后至少运行：

```bash
git diff --check
```

## 2026-05-02: symphony-issue-run 必须看着框架自己跑完

### 用户纠正

- 用户指出：ZEE-41 处理太久，状态流转进了 `Human Review`，不符合 `Todo -> In Progress -> AI Review -> Merging -> Done` 的全自动目标。
- 用户强调：`symphony-go` 应该由 listener / workflow / orchestrator 自己完成，不应该由父会话手工接管 `Merging`；我的职责是观察它完成并总结经验。

### 错误模式

- 这是流程错误：我把父会话手工 merge 当成完成任务的收口方式，弱化了 `symphony-go` 自动化框架本身的验收标准。
- 这是技术判断错误：`Human Review` 虽然由 child 记录为权限 blocker，但根因是 runner 的 writable roots 没覆盖 `Merging` 所需的 repo root main checkout，属于框架可修复问题，不应该默认交给人。
- 这是职责边界错误：child prompt 和 orchestrator 同时尝试推进 `AI Review` / `Merging`，状态 owner 重叠会让 Linear 时间线出现重复流转。
- 这是沟通错误：最终汇报只说已经完成，没有先指出“这次完成包含人工接管，不满足全自动闭环”的事实。

### 防复犯规则

- `symphony-issue-run` 的成功标准必须是 listener 自己把 issue 跑到 terminal；父会话只能监控、诊断和优化框架，不能把手工 merge 当作正常路径。
- `In Progress` / `Rework` child 只负责提交、验证和 workpad handoff；`AI Review` / `Merging` 状态切换由 orchestrator 统一负责，避免重复切状态。
- 任何进入 `Human Review` 的默认自动流程都要当成异常复盘：先查 `.human.log` / JSONL / workpad，分类为 Skill、Workflow、Code 或 Environment gap。
- 如果 `Merging` 因写 repo root、git metadata、push 或 auth 卡住，优先修 workflow/runner/orchestrator 的自动化边界；只有真实外部 blocker 才允许停在人审。

### 固定动作

- 复盘状态流：

```bash
rg -n "state_changed|Human Review|AI Review|Merging|Done|blocked|error|Operation not permitted" .symphony/logs/run-*.human.log
```

- 复盘权限边界：

```bash
rg -n "writableRoots|workspaceWrite|gitMetadataRoots|Merging" internal/codex internal/orchestrator WORKFLOW.md
```

- 记录优化：

```bash
$EDITOR docs/optimization/symphony-issue-run.md
$EDITOR lesson.md
```

## 2026-05-02: macOS `missing LC_UUID load command`

### 用户纠正

- 用户指出：不要每次都因为同一个 Go 测试命令踩 `missing LC_UUID load command`，应该写固定脚本，以后每次跑脚本。
- 用户要求：根目录 `build.sh` / `test.sh` 要高内聚，不要依赖隐藏的二级脚本。

### 错误模式

- 这是流程错误：仓库 `Makefile` 已经有 `-linkmode=external` 约定，但我手动运行裸 `go test`，绕过了仓库入口。
- 这是环境判断错误：`internal/linear` 的失败来自本机 macOS dyld 对 Go 临时测试二进制的加载限制，不是测试断言失败。
- 这是脚本设计错误：根目录入口如果只是跳到 `scripts/test.sh`，核心逻辑仍然不够高内聚，后续维护容易分叉。

### 防复犯规则

- 本仓 Go 测试不要直接运行裸 `go test`，除非目标就是验证裸 Go 工具链行为。
- 根目录 `build.sh` / `test.sh` 必须自包含关键逻辑：定位 repo root、设置 external linker、执行对应 Go 命令。
- 面向人和 AI 的稳定入口优先放在根目录；`scripts/` 只放内部辅助脚本，不承载必须记住的核心入口。

### 固定动作

- 跑完整测试：

```bash
./test.sh
```

- 跑局部包或单测：

```bash
./test.sh ./internal/orchestrator ./internal/linear
./test.sh ./internal/orchestrator -run TestName
```

- 构建：

```bash
./build.sh
```

- Makefile 也必须委托到根目录入口：`make test` 调 `./test.sh`，`make build` 调 `./build.sh`。

## 2026-05-02: 合入分支不要从当前 Git 分支推导

### 用户纠正

- 用户指出：Makefile 里 `MERGE_TARGET ?= $(if $(CURRENT_BRANCH),$(CURRENT_BRANCH),feat_zff)` 会让当前 `feat_zff` 分支变成合入目标，但 workflow 预期应该合入 `main`。
- 用户要求：合入分支应该能在 workflow 里指定。

### 错误模式

- 这是流程错误：把运行所在 Git 分支当成业务合入目标，会让本地开发分支污染 workflow 语义。
- 这是配置边界错误：合入目标属于 workflow 策略，不应该由 Makefile 根据当前 shell 环境猜测。

### 防复犯规则

- 合入目标默认由 `WORKFLOW.md` 的 `merge.target` 指定。
- Makefile 只能在用户显式传入 `MERGE_TARGET=...` 时覆盖 workflow，不得读取当前 Git 分支自动推导。
- 命令行 `--merge-target` 只作为显式覆盖入口；未传时应使用 workflow 配置。

### 固定动作

- 查看当前 workflow 合入目标：

```bash
rg -n "merge:|target:" WORKFLOW.md
```

- 使用 workflow 默认目标运行：

```bash
make run-once ISSUE=<issue>
```

- 临时覆盖合入目标：

```bash
make run-once ISSUE=<issue> MERGE_TARGET=<branch>
```

## 2026-05-03: 实验架构文档不能作为权威参考

### 用户纠正

- 用户指出：`docs/architecture/symphony-go-architecture.md` 是之前做实验用的文档，不要作为参考。

### 错误模式

- 这是事实错误：我把历史实验文档当成维护中的架构参考源。
- 这是流程错误：配置 domain docs 时没有先区分权威契约、纠错记录和实验记录。

### 防复犯规则

- 配置或消费 domain docs 时，默认以 `SPEC.md`、`WORKFLOW.md`、`AGENTS.md`、`lesson.md` 和必要 ADR 为准。
- `docs/architecture/symphony-go-architecture.md` 不作为本仓默认参考源；只有用户明确要求回看实验记录时才打开，并标注其可能过期。

### 固定动作

- 检查默认参考源是否误引用实验架构文档：

```bash
rg -n "symphony-go-architecture|docs/architecture|架构文档" AGENTS.md docs/agents lesson.md
```

## 2026-05-04: Linear issue 评论默认中文

### 用户纠正

- 用户指出：issue 的评论以后都搞成中文的。

### 错误模式

- 这是沟通错误：我把 triage comment 的主体内容写成英文，虽然本仓已经明确对外可见文本默认使用中文。
- 这是流程错误：发布 Linear comment 前没有按 `WORKFLOW.md` 和 `docs/agents/issue-tracker.md` 复核外显语言约束。

### 防复犯规则

- 本仓所有写到 Linear issue、workpad、交接记录、review 回复或其他外部系统的新增可见文本默认使用中文。
- 命令、路径、错误原文、代码标识符和第三方固定模板字段可以保留英文。
- 如果某个 skill 强制要求固定英文 disclaimer，例如 triage comment 的 `> *This was generated by AI during triage.*`，只保留该固定行，其余正文仍使用中文。

### 固定动作

- 发布或更新 Linear comment 前先检查正文语言：

```bash
rg -n "对外可见文本默认使用中文|Linear workpad|交接记录" WORKFLOW.md docs/agents/issue-tracker.md lesson.md
```

- 若发现已发布的 issue comment 正文误用英文，立即用 `linear issue comment update <comment_id> --body ...` 原地改成中文。
