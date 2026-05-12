---
name: pet-prd-issue-run
description: 面向 social_pet 仓库的 PRD 子 issue 闭环技能。用于已有 PRD 父 issue 和子 issue 时，循环分诊可执行子任务，用 pet-local-tdd 做 TDD 和本地证据闭环，再参考 bytedcli-codebase 创建到指定目标分支的 Codebase MR。
---

# pet-prd-issue-run

## 适用范围

本 skill 用于 `social_pet` 仓库的 PRD 推进流程。

当用户给出一个已有 PRD 父 issue，且它已经有子 issue、显式切片或明确 checklist 时，使用本 skill 持续推进：

```text
分诊下一个可执行子 issue
用 pet-local-tdd 做 TDD 和本地验证
参考 bytedcli-codebase 创建或更新到指定目标分支的 Codebase MR
推动子 issue 的 Linear 状态流转并更新 Workpad
回到父 PRD 判断是否还有未完成子 issue
```

默认目标是把已有子 issue 做完并证明完成，不从宽泛 PRD 描述里自行发明新需求。

## 必读 Skill

执行前直接读取这些文件，以文件内容为准：

- `pet-local-tdd`: `/Users/bytedance/.skills-manager/skills/pet-local-tdd/SKILL.md`
- `bytedcli-codebase`: 本地对应 `/Users/bytedance/.skills-manager/skills/bytedance-codebase/SKILL.md`；如果运行环境暴露的是 `bytedance-codebase`，以该 Codebase 专项 skill 为准
- `bytedcli`: `/Users/bytedance/.skills-manager/skills/bytedcli/SKILL.md`
- `triage`: 如果本次使用 Linear/issue tracker 分诊，读取当前可用的 `triage` skill；在 `/Users/bytedance/symphony-go` 中通常是 `.codex/skills/triage/SKILL.md`
- Linear MCP/app：如果当前运行环境暴露 Linear MCP 或 Linear app 工具，可以用它读取 issue、更新状态、更新评论和维护 Workpad

如果用户同时指定了其它仓库内规则，例如 `AGENTS.md`、`CLAUDE.md`、`docs/agents/*`，先读这些规则再写 issue comment、改代码或建 MR。

## 必填输入

开始前必须明确这些信息：

- PRD 父 issue：例如 Linear issue id 或可读 URL。
- `social_pet` 工作区路径：若用户未指定，先从当前目录、常用 worktree 或 issue 描述里确认。
- MR 目标分支：用户调用本 skill 时应指定，例如 `online`、`release/...` 或其它分支。

如果缺少 MR 目标分支，停止并向用户确认。不要默认猜 `main`、`master` 或 `online`。

## 核心规则

- 默认用中文写 Workpad、issue comment、MR 标题和 MR 描述。
- 默认最小影响：只改当前子 issue 必需的代码、测试和少量证据文档。
- 默认根因导向：bug 类子 issue 先复现或定位失败证据，再做最小修复。
- TDD 必须交给 `pet-local-tdd` 的规则执行：先 RED，再 GREEN，再用 `ci/run.sh` 或指定命令沉淀证据。
- `social_pet` 本地单测很耗时，必须把测试时间当成 PRD 调度成本管理；不要在每次小编辑后都跑仓库脚本。
- Codebase/MR 操作必须优先参考 `bytedcli-codebase` / `bytedance-codebase`；底层仍使用 `bytedcli codebase ...`，不要先开网页或手写内部 API。
- MR 必须创建到用户指定的目标分支，按 Codebase 专项 skill 和当前 help 使用 `bytedcli codebase mr create --base <TARGET_BRANCH>`。
- 默认每个可执行子 issue 使用一条独立 source branch；MR 的 head 是该子 issue 分支，base 是用户指定的 `<TARGET_BRANCH>`。
- 父 PRD 本身不创建实现分支，除非父 PRD 有自己的明确代码验收项。
- 不自动合并 MR，除非用户明确要求本 skill 负责 merge。
- 不删除、reset、覆盖已有脏区、stash 或其他 agent 的改动。
- 父 PRD 下已有子 issue 全部完成后，才关闭父 issue；不要因为 PRD 还有宽泛未来想法就自行新增切片。

## Preflight

在 `social_pet` repo root 执行：

```bash
pwd
git status --short --branch
git stash list
git remote -v
git branch --show-current
```

确认目标分支和远端状态：

```bash
git fetch origin <TARGET_BRANCH>
git rev-parse --verify origin/<TARGET_BRANCH>
```

确认 Codebase 专项 skill、bytedcli 可用和认证状态：

```bash
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest --json auth status
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest codebase auth --help
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest codebase mr create --help
```

脏区处理规则：

1. 如果当前分支有未提交改动，先判断是否属于本次 PRD 前置记录、用户改动、还是上一个子 issue 的残留。
2. 前置记录可以按用户意图提交或记录；实现残留不能拖进新子 issue。
3. stash 只能读取和记录；只有明确属于本轮需要恢复的内容，且用户同意时才 apply。
4. 开始子 issue 前，确保本次工作分支或 worktree 的基线来自 `origin/<TARGET_BRANCH>`。

## 父 PRD 评估

读取 PRD 父 issue、子 issue、评论、关系和已有 Workpad，整理一个当前事实表：

- 子 issue id、标题、状态、标签、负责人。
- blocker 关系，优先使用 issue tracker 里的 `Blocked by`。
- 每个子 issue 的验收标准、预期修改范围、测试范围。
- 已有分支、MR、Workpad 和验证证据。

调度规则：

- 先处理没有未完成 blocker 的子 issue。
- 如果多个子 issue 独立且修改范围不冲突，可以并行交给子代理；否则顺序推进。
- 如果某个子 issue 会改公共 IDL、公共 helper、公共 mock 或共享测试基建，优先串行处理。
- 每完成一个子 issue，就重新读取父 PRD 和子 issue 图，不沿用旧判断。
- 如果剩余 issue 都需要人工信息或业务决策，把 blocker 写回父 PRD Workpad 后停止。

## Linear 流转

Linear 状态流转是本 skill 的交付物之一。不要只提交代码或创建 MR 后停止。

执行前先读取真实项目状态名和既有 Workpad；不同项目可以把状态命名为 `Todo`、`In Progress`、`AI Review`、`Merging`、`Done` 或其它中文/英文等价状态。不要凭记忆硬编码不存在的状态名。

pet 流程允许使用 Linear MCP/app 能力更新状态和评论。优先级：

1. 当前 runtime 暴露 Linear MCP/app 工具时，优先用 MCP/app 读取 issue、更新状态、更新 Workpad/comment。
2. 如果 MCP/app 不可用或当前会话无法完成写入，再退回项目约定的 CLI、GraphQL 或人工提示路径。
3. 每次 MCP/app 写入后必须重新读取该 issue，确认状态、评论或 Workpad 已真实更新。

默认流转门禁：

1. 父 PRD：在仍有未完成子 issue 时保持打开，并在父 Workpad 记录当前 DAG、已完成子 issue、活跃分支/MR 和 blocker。
2. 子 issue 开始前：确认无未完成 blocker；如果缺信息，转到等待信息/人工处理状态并写清问题。
3. 子 issue 可执行时：写 agent brief 或执行计划，分配给当前 agent，并移动到实现中状态。
4. 本地 TDD 和验证通过后：更新子 issue Workpad，写入 RED/GREEN/VERIFY、日志路径、分支、commit 和 MR 目标分支。
5. MR 创建或更新后：把子 issue 移到 review/待审状态，记录 MR URL、CI 查询方式和当前检查结果。
6. MR 进入待合入或用户明确要求合入时：移动到 Merging/待合入状态，并继续跟踪 Codebase MR 状态。
7. 只有 MR 已合入目标分支，且重新读取 Linear 后验收项都可证明完成，才把子 issue 移到 Done。
8. 所有列出的子 issue 都 Done 后，重新读取父 PRD，写 closure comment，再关闭父 PRD。

每次状态变化都要留下证据：Linear 当前状态、评论/Workpad 链接或摘要、MR URL、验证命令和关键日志路径。

## 子 Issue 执行循环

### 1. 分诊和计划

读取子 issue 的描述、评论、标签、blocker 和 Workpad。

输出本子 issue 的成功标准：

- 要证明的行为变化。
- 最小 RED 测试名或复现命令。
- 需要修改的文件或模块范围。
- 最终验证命令和证据路径。
- MR 目标分支。

如果 issue 仍缺关键业务决策，写清阻塞点并停止，不带着猜测写代码。

### 2. 创建工作分支或 worktree

默认每个子 issue 一条独立 source branch。先检查该子 issue 是否已有可复用分支或 MR；如果已有且仍对应当前目标分支，继续复用，不重复创建。

从指定目标分支创建本子 issue 的工作分支：

```bash
git fetch origin <TARGET_BRANCH>
git switch -c pet/<ISSUE_ID> origin/<TARGET_BRANCH>
```

如果用户要求 worktree，使用独立 worktree，但基线仍必须是 `origin/<TARGET_BRANCH>`：

```bash
git worktree add .worktrees/<ISSUE_ID> -b pet/<ISSUE_ID> origin/<TARGET_BRANCH>
```

分支命名可按仓库现有习惯调整，但必须能从名称或 MR 描述追溯到 issue。

分支关系必须保持清楚：`pet/<ISSUE_ID>` 是 MR head/source branch，`<TARGET_BRANCH>` 是 MR base/target branch。

### 3. TDD 实现

严格按 `pet-local-tdd` 执行：

1. 写一个最小 RED 测试或最小复现。
2. 确认 RED 指向目标行为缺失；如果是生成物缺失，先按 `pet-local-tdd` 判断是否需要 `rgo generate`。
3. 做最小 GREEN 实现。
4. 用最小范围命令快速回归。
5. 当前子 issue 的实现、测试和自查都完成后，再跑一轮收口验证并沉淀证据。

耗时管理规则：

- 本地单测可能长时间卡初始化或等待目标测试终态；按 `pet-local-tdd` 的等待窗口执行，不要因为 1 到 3 分钟没有结果就提前下结论。
- 不要把 bare direct `go test ./service -run '^TestName$'` 写成默认开发内循环；这个形式在 `social_pet` 当前链路里跑不通时就不要再推荐。
- 开发内循环只跑当前仓库和 `pet-local-tdd` 已确认可用的最小命令；如果没有可靠的快速命令，就先完成本子 issue 的实现自查，再进入收口验证。
- 单个子 issue 默认在“全部代码和测试都改完”后跑一轮收口验证；优先用 `./ci/run.sh '<TestNameOrRegex>'` 和 `CI_RUN_LOG_DIR` 模式沉淀 repo-native 证据。
- 同一批无冲突子 issue 如果都只改独立范围，可以先分别用已确认可用的最小命令完成 RED/GREEN，再在这一批全部实现完成后合并成一轮 `./ci/run.sh '<CombinedRegex>'`；Workpad 要写清这一轮覆盖了哪些 issue。
- 不建议等整个 PRD 所有 issue 都改完才做第一次验证；每个子 issue 至少要有自己的最小 RED/GREEN 证据，否则最后定位失败成本会很高。
- 所有子 issue 都完成并准备关闭父 PRD 前，再跑一轮更广的最终验证；范围按改动面选择，可以是合并后的测试正则、相关包，或用户明确要求的全量单测。
- 不要默认跑整包、整目录、`build.sh` 或全量测试；只有改到构建敏感文件、生成链路、配置、脚本、`go.mod/go.sum`，或验收明确要求时才扩大验证。
- 多个子 issue 并行时，不要无脑同时启动多条 `ci/run.sh`；先评估机器资源和目标测试范围，避免本地测试互相拖慢或日志难以归档。
- 如果 `ci/run.sh` 到 10 到 12 分钟仍没有目标测试终态，按 `pet-local-tdd` 写 `UNKNOWN/HANG` 并保留 raw 证据；只有存在已确认可用的对照命令时才补对照组。
- Linear Workpad 和最终报告必须写清测试耗时状态：`ci/run.sh=PASS/FAIL/UNKNOWN`、最小对照命令是否存在及结果、日志目录和是否因耗时扩大/收窄验证范围。

最终证据至少记录：

- RED 命令和失败摘要。
- GREEN 改动摘要。
- VERIFY 命令。
- `docs/test/log/run-<timestamp>/` 或用户指定日志目录。
- `script.raw.log`、`script.verdict.txt` 中的 PASS/FAIL/UNKNOWN 依据。

### 4. 提交和 MR

提交前检查：

```bash
git status --short --branch
git diff --check
```

提交信息要能关联 issue，例如：

```bash
git commit -m "fix: handle <behavior> for <ISSUE_ID>"
```

推送当前分支：

```bash
git push -u origin HEAD
```

创建 MR 前，先按 `bytedcli-codebase` / `bytedance-codebase` 的 MR 流程检查现有 MR 和当前命令 help，不猜参数：

```bash
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest codebase mr create --help
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest codebase mr update --help
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest --json codebase mr list -H <SOURCE_BRANCH> -B <TARGET_BRANCH> -L 20
```

没有现有 MR 时，创建到指定目标分支：

```bash
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest --json codebase mr create \
  --base <TARGET_BRANCH> \
  --title "<中文 MR 标题>" \
  --body "<中文 MR 描述，包含 issue id、摘要、验证命令和日志路径>"
```

如果不在 code.byted.org 或 code-tx.byted.org 仓库目录内，或远端无法自动推断仓库，补 `-R <repo/path>`。如果源分支不是当前分支，补 `--head <SOURCE_BRANCH>`。

如果已存在当前源分支到目标分支的 MR，按 Codebase 专项 skill 使用 `bytedcli codebase mr update` 更新 body、工作项关联或 target branch，不重复创建：

```bash
NPM_CONFIG_REGISTRY=http://bnpm.byted.org npx -y @bytedance-dev/bytedcli@latest --json codebase mr update <MR_ID> \
  --base <TARGET_BRANCH> \
  --body "<更新后的中文 MR 描述>"
```

MR 描述至少包含：

```markdown
## 摘要
- <本次行为变化>

## 验证
- `git diff --check`
- `<pet-local-tdd 验证命令>`
- 日志：`docs/test/log/run-<timestamp>/...`

Issue: <ISSUE_ID>
Target branch: <TARGET_BRANCH>
```

创建或更新后，按 Codebase 专项 skill 用 `bytedcli codebase mr get/list/status` 和 `bytedcli codebase checks mr` 读取 MR 状态和 CI 证据；不要重复创建多个 MR。

### 5. 子 Issue 收口

更新子 issue Workpad 或评论，记录：

- 当前分支和 MR URL。
- RED/GREEN/VERIFY 证据。
- 已证明的验收项。
- 未覆盖的风险或人工验证项。
- MR 目标分支。

只有当前代码、测试和 MR 都能证明验收项时，才把子 issue 移到 review/待审状态。MR 通过 review 或进入合入队列后，再移动到 Merging/待合入状态。

如果本 skill 不负责合入，子 issue 不应直接 Done；保留在 review/待合入状态，并在 Workpad 写清“等待 MR 合入到 `<TARGET_BRANCH>`”。只有 MR 已合入目标分支并完成回读验证，才移动到 Done。

## 父 PRD 收口

每个子 issue 收口后回到父 PRD：

1. 重新读取父 PRD、子 issue 状态、评论和 MR 状态。
2. 更新父 PRD Workpad：已完成子 issue、MR 链接、目标分支、验证证据、剩余 blocker。
3. 如果还有未完成子 issue，继续选择下一个无 blocker 子 issue。
4. 如果所有列出的子 issue 都完成，并且父 PRD 的关闭条件已被证明，写父 PRD closure comment。
5. 如果父 PRD 只有宽泛未来想法，没有明确子 issue 或 checklist，不把它们当作本轮 blocker。

父 PRD closure comment 应包含：

- 已完成子 issue 列表。
- 每个子 issue 的 MR 链接和目标分支。
- 关键验证命令和日志路径。
- 为什么无需继续拆本轮子 issue。

## 最终报告

最终回复使用中文，包含：

- PRD 父 issue id 和当前状态。
- 本轮完成的子 issue。
- MR URL、目标分支和提交 hash。
- RED/GREEN/VERIFY 摘要。
- 实际运行过的验证命令和结果。
- 剩余 blocker、未合入 MR 或需要人工处理的事项。
