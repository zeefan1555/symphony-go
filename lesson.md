# Lessons

## 2026-05-07: social_pet 不要默认推荐 bare direct go test

### 用户纠正

- 用户指出：`social_pet` 的开发内循环里，`go test ./service -run '^TestName$'` 这种 bare direct 形式跑不通，不要再推荐这种默认写法。

### 错误模式

- 这是流程错误：我把 `pet-local-tdd` 中可作为特定对照组的 direct `go test` 示例，误提升成了 `pet-prd-issue-run` 的默认开发内循环。
- 这是技术判断错误：没有区分 `social_pet` 当前可用的仓库测试入口和理论上的 Go 定向测试命令，导致 skill 会诱导后续 agent 反复跑已知不可用路径。

### 防复犯规则

- 在 `social_pet` PRD/issue 编排里，不要把 bare direct `go test ./service -run '^TestName$'` 作为默认快速回路。
- direct `go test` 只能在当前仓库、当前分支已确认可跑时作为显式对照组；如果用户或历史经验说明该形式跑不通，就不要再写进默认命令。
- 默认收口证据优先走 `pet-local-tdd` 中 repo-native 的 `./ci/run.sh` + `CI_RUN_LOG_DIR` 路径；快速验证也必须写成“已确认可用的最小命令”，不要猜裸命令。

### 固定动作

- 修改 `social_pet` 相关 skill 或 runbook 时检查是否重新引入裸 direct `go test` 默认路径：

```bash
rg -n "go test ./service|-run '\\^TestName\\$'|direct go test" .codex/skills docs lesson.md
```

- 文档改动后至少运行：

```bash
git diff --check
```

## 2026-05-06: AI Review 后应直接进入 PR skill 快路径

### 用户纠正

- 用户指出：简单 smoke 跑到 10 分钟不合理；此前已经写过 `pr` skill，`AI Review` 完成后应该直接使用该 skill，而不是继续走更重的 merge/land 流程。
- 用户进一步指出：agent 在 issue worktree 目录下处理，没有稳定权限写 repo-root `main` checkout，因此 `pr` skill 不应该承诺同步 repo-root `main`。

### 错误模式

- 这是流程错误：workflow 回退成实现阶段先创建 PR、先做完整 feedback sweep，`Merging` 再走 `land` skill，导致 PR/check/review 相关动作被拆散且重复。
- 这是技术判断错误：`pr` skill 应该封装 push、PR 创建/更新、checks 和 squash merge；把 repo-root `main` sync 放进 child agent 会把权限边界问题变成假 blocker。

### 防复犯规则

- `AI Review` 阶段只审本地 commit、diff、workpad 和验证证据；review 通过后进入 `Merging`，直接执行 `.codex/skills/pr/SKILL.md` 的 PR merge flow。
- `Merging` 阶段不要重新展开实现、review 或完整历史 workpad；脚本前只做最小状态检查和 PR title/body 准备。
- PR feedback sweep、remote checks、PR metadata 和 squash merge 归 `pr` skill/script 统一处理，不要在移动到 `AI Review` 前提前重复执行。
- issue worktree agent 不负责写 repo-root `main` checkout；root sync 只能由 orchestrator/operator 在 repo-root context 中确认安全后执行。

### 固定动作

- 修改 workflow merge 路径时先核对：

```bash
rg -n "pr/SKILL|pr_merge_flow|land/SKILL|Merging 快路径|AI Review" WORKFLOW.md internal/service/workflow/workflow_test.go
```

- 验证 workflow 合同时至少运行：

```bash
./test.sh ./internal/service/workflow
git diff --check
```

## 2026-05-05: Symphony Go 业务与运行支撑应归入 service-rooted 结构

### 用户纠正

- 用户指出：`/Users/bytedance/symphony-go/internal` 的当前文件结构不符合预期，Symphony Go 的业务逻辑应该放在 `internal/service/...` 下。
- 用户要求回看此前已经确认过的口径，不要把业务逻辑继续散放在 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 或 `internal/control` 这类顶层包里。
- 用户进一步明确：迁移完毕后，原顶层 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 只应作为迁移期兼容 shim 或最终消失；`internal/control/hertz*`、`internal/issuetracker`、`internal/config`、`internal/logging`、`internal/observability` 也不应继续作为顶层目录存在，目标是提升人类可读性。
- 用户明确否定：`internal/generated` 不需要，Hertz 生成代码应收敛到 `gen/hertz`；`internal/types` 也不应长期存在，应该优先复用 `gen/hertz/model` 中的数据结构；`adapter` 这个目录名不好，`platform` 也看不出和 `service` 的区别。

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
- `internal/generated` 已被主 Hertz `gen/hertz/` 生成目录取代，迁移完毕后不应继续存在；当前残留的 `internal/generated/hertz/scaffold` 生成链需要先退役或迁到新的可读边界。
- `internal/types` 不应作为长期共享模型目录存在；删除前必须先区分可并入 `gen/hertz/model` 的控制面数据结构，以及仍需要本地 workflow/config 语义的运行时结构。
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
rg -n "symphony-go/internal/(orchestrator|workspace|codex|workflow|control|issuetracker|config|logging|observability)" internal cmd biz docs
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

## 2026-05-06: IDL 迁移不得倒逼手写代码改名

### 用户纠正

- 用户指出：迁移 Thrift 到 Proto 的目标是让 Hertz 使用 Proto 生成模型，并让原有代码继续引用生成模型；不应该因为生成器产物变成 `main.pb.go` 或字段命名变化，就直接修改原始手写代码来适配。

### 错误模式

- 这是技术判断错误：我把 Protobuf 生成器的默认输出形态当成必须接受的边界，扩大了迁移影响面。
- 这是流程错误：没有先确认现有 `gen/hertz/...` 文件名和 `IssueID` / `SessionID` 这类 Go API 是否属于下游兼容契约。

### 防复犯规则

- IDL 或代码生成迁移必须优先保护既有手写代码的导入路径、文件名和公开 Go 标识符。
- 生成器默认产物和现有仓库契约冲突时，优先在生成脚本里做稳定化处理，避免让业务代码跟着生成器抖动。
- 自动生成代码可以提交，但普通业务改动不应无理由重新生成；只有 IDL、生成脚本或生成器版本变化时才更新 `gen/hertz/...`。

### 固定动作

- IDL 迁移后检查是否误生成新文件名：

```bash
rg --files gen/hertz/model | rg '\.pb\.go$'
```

- 检查手写代码是否被迫改成生成器的新命名：

```bash
git diff -- internal/transport internal/service internal/app
```

- 重新生成后必须跑契约和边界检查：

```bash
make hertz-generate
scripts/check_generated_hertz_boundary.sh
./test.sh ./internal/hertzcontract ./internal/transport/hertzserver
```

## 2026-05-06: 单个 issue 默认只用一个 agent

### 用户纠正

- 用户指出：希望一个 issue 由一个 agent 处理，不要在处理同一个 issue 时多开其他 agent。

### 错误模式

- 这是流程错误：原规则只写了子代理使用条件，没有明确单个 issue 的默认执行边界，容易在 issue run 中过早拆出并行 agent。

### 防复犯规则

- 处理单个 Linear issue 时，默认由当前 agent 端到端负责规划、执行、验证和收口。
- 不要为同一个 issue 额外多开子代理、reviewer 或 worker；只有用户明确要求并行 agent，或任务被明确拆成多个独立 issue 时，才考虑多 agent。

### 固定动作

- 开始 issue run 前检查子代理策略是否仍保留单 agent 约束：

```bash
rg -n "单个 Linear issue|同一个 issue|多开子代理|单 agent" AGENTS.md lesson.md
```

## 2026-05-06: 语义评论仍由 AI 写

### 用户纠正

- 用户指出：监听服务不应该替 AI 写评论；监听服务不会分析评论内容是否完整，语义判断和 evidence 仍应由 AI 完成。

### 错误模式

- 这是技术判断错误：我把 workpad/comment 的样板化成本和语义完成性判断混在一起，过早建议把评论生成下沉到 orchestrator。
- 这是边界错误：orchestrator 可以做确定性状态收口，但不应承担理解 issue、判断验收是否完成、生成交接证据的职责。

### 防复犯规则

- AI 负责分析 issue、更新 workpad/comment、写最终 evidence 和判断是否满足验收。
- Orchestrator 只根据窄机器契约做确定性续航或收口，例如 `Review: PASS`、`Merge: PASS`。
- 不要让监听服务替 AI 判断评论内容是否充分，也不要让监听服务生成语义 handoff 评论。

### 固定动作

- 修改 workflow 或 orchestrator 状态收口前，先检查是否越过 AI/comment 边界：

```bash
rg -n "Merge: PASS|Review: PASS|workpad|comment|Done" WORKFLOW.md internal/service/orchestrator .codex/skills
```

## 2026-05-12: 人工启动 workflow 默认保留 TUI

### 用户纠正

- 用户指出：专门启动排查问题 workflow 的 `make` 命令应该能看到 TUI。

### 错误模式

- 这是沟通和技术判断错误：我把长期 listener 的后台可脚本化需求默认套到了人工启动入口上，忽略了用户当前是想手动观察运行过程。

### 防复犯规则

- 面向人工启动、观察和调试的 workflow 入口默认使用 `--tui`。
- 只有用户明确要求后台运行、非交互 shell、脚本化执行或一次性 smoke 时，才默认使用 `--no-tui`。

### 固定动作

- 新增或调整 workflow `make` 入口时，先按用途区分人工观察和脚本运行：

```bash
make -n <target>
```

## 2026-05-12: 高频 Make 入口要短

### 用户纠正

- 用户指出：`bytecode-triage` 作为日常启动命令太长，应该直接用 `make bytecode`。

### 错误模式

- 这是沟通错误：我按语义完整性命名入口，但忽略了高频命令更需要短、好记、低输入成本。

### 防复犯规则

- 面向日常高频使用的 Make target 优先短命名；语义较长的名字可以作为兼容别名或说明，不应成为唯一入口。

### 固定动作

- 新增高频 Make target 后检查 dry-run 是否能用最短命令触发：

```bash
make -n bytecode
```

## 2026-05-12: social_pet 验证不要用裸 go test

### 用户纠正

- 用户指出：`social_pet` 仓库裸 `go test` 会缺少很多环境变量，应该用仓库脚本 `.ci/run.sh` / `ci/run.sh`。

### 错误模式

- 这是流程错误：我用通用 Go 验证习惯替代了目标仓库自己的 CI 入口，导致验证方式不符合仓库环境约定。

### 防复犯规则

- 排查或验证 `social_pet` 时，优先使用仓库脚本 `ci/run.sh`；不要把裸 `go test` 作为默认结论。
- 如果只需要类型检查或窄验证，也应通过 `ci/run.sh <regex>` 这类仓库认可入口运行，并把日志落到任务目录。

### 固定动作

- 在 `social_pet` 仓库验证前先确认脚本入口：

```bash
find . -maxdepth 3 -path '*/run.sh' -print
```
