# Lessons

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
