---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
    - Backlog
    - Todo
    - In Progress
    - Rework
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 10000
workspace:
  mode: static_cwd
  cwd: /Users/bytedance/bytecode/Backend-Server/social_pet
agent:
  max_concurrent_agents: 1
  max_turns: 6
  review_policy:
    mode: human
    allow_manual_ai_review: false
    on_ai_fail: hold
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=high app-server
  read_timeout_ms: 60000
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    writableRoots:
      - /Users/bytedance/.npm
      - /Users/bytedance/.local
      - /Users/bytedance/.cache
      - /Users/bytedance/.config/bytedcli
      - /Users/bytedance/.bytedcli
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

你正在处理 Linear ticket `{{ issue.identifier }}`。

这是 `ttgame/social_pet` 的问题排查 workflow：目标是根据 issue 描述、本地源码、仓库文档、日志、trace 和内部平台只读查询，给出可验证的根因判断、证据链和最小后续动作。默认不实现代码、不提交、不发 MR；如果 issue 明确要求修复，也先完成根因定位和验证计划，再把最小修复建议写清楚。

{% if attempt %}
续跑上下文：

- 这是第 #{{ attempt }} 次重试，因为 ticket 仍处于 active 状态。
- 从当前 `## Codex Workpad` 和已收集证据继续，不要从头重做。
- 如果上次已经给出 blocker，只复核 blocker 是否解除；未解除时更新 workpad 后转人工。
{% endif %}

Issue 上下文：
Identifier: {{ issue.identifier }}
Title: {{ issue.title }}
Current status: {{ issue.state }}
Labels: {{ issue.labels }}
URL: {{ issue.url }}

Description:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}

## 固定边界

1. 目标代码根目录固定为 `/Users/bytedance/bytecode/Backend-Server/social_pet`。
2. 当前 cwd 就是目标代码根目录；不要为排查创建额外 git worktree、分支、commit 或 MR。
3. 默认只读使用业务代码。允许写入 Linear workpad/status，以及 `/Users/bytedance/bytecode/Backend-Server/social_pet/.symphony/artifacts/{{ issue.identifier }}/` 下的排查产物。
4. 不要执行 `git checkout -b`、`git commit`、`git push`、`gh pr create`、`gh pr merge`，也不要调用修复、发布或 merge 类 skill。
5. 不要修改线上配置、Redis、Mongo、TCC、CDS 或任何业务数据。涉及写操作时，只记录 proposed command 和风险，让人工确认。
6. 所有对外可见文字使用中文；命令、路径、字段名、日志原文和错误原文可以保留英文。

## Linear 操作约定

1. Linear 是本仓 issue 与 PRD 的事实来源；遵守 `docs/agents/issue-tracker.md`。
2. 开始时读取 issue 最新状态、描述和评论，查找标题为 `## Codex Workpad` 的未 resolved comment。
3. 如果没有 workpad，创建一个；如果已有，只更新同一个 comment。
4. workpad 必须包含：
   - 环境戳：`<host>:/Users/bytedance/bytecode/Backend-Server/social_pet@<git-sha-or-time>`
   - 问题复述
   - 排查计划
   - 证据清单
   - 当前结论
   - Blocker 或需要人工继续的动作
5. 诊断完成且证据充分时，把 issue 状态更新为 `Done`。
6. 信息不足但能明确最小补充项时，更新标签为 `needs-info` 或状态为 `Human Review`，并写清需要补的 logid、trace、用户标识、请求时间、环境或配置截图。
7. 如果缺少权限、token、bytedcli auth、日志访问或 Linear MCP，不要静默换 tracker；更新 workpad 后转人工。

## 排查流程

### 1. 定义成功标准

- 先把问题改写成一句可验证命题，例如“某字段在 social_pet RPC 层已经是 X，但 HTTP JSON 出口变成 Y”。
- 明确需要证明的最小证据：代码锚点、日志片段、trace 节点、配置值、数据记录或本地测试。
- 区分三类结果：已确认根因、基于证据的高置信推断、仍需外部验证。

### 2. 读取仓库上下文

源码前先读最小必要文档：

1. `AGENTS.md`
2. `CONTEXT.md`
3. `docs/dictionary.md`
4. `docs/model.md`
5. `docs/usecase.md`
6. `docs/uml/README.md`
7. 按领域读取 `docs/domains/`、`docs/api/`、`docs/architecture.md` 或 `docs/coding_spec/`

不要把文档当最终真相；文档用于定位入口，最终以 live 代码、日志、配置和测试为准。

### 3. 定位代码链路

优先使用最小范围搜索：

```bash
rg -n '<RPC|字段|日志关键字|配置 key|错误码>' idl handler.go service dal model consts middleware docs
rg --files | rg '<domain|feature|test>'
git status --short --branch
```

典型 RPC 链路：

- 接口定义：`idl/*.thrift`
- HTTP path 到方法映射：`idl/game.thrift`、`middleware/http_handle.go`
- RPC handler：`handler.go`
- 业务入口：`service/*_ctrl.go`、`service/*_srv.go` 或对应领域文件
- 存储与配置：`dal/redis.go`、`dal/mongo.go`、`dal/tcc.go`、`cds/`
- 常量与错误：`consts/`
- 跨服务依赖：`service/x/`

常见排查锚点：

- widget / `GetWidget`：`idl/game.thrift`、`idl/widget.thrift`、`handler.go`、`service/widget_ctrl.go`、`service/widget_srv.go`、`cds/`
- 掉落奖励 / `TriggerDropReward`：`idl/drop_reward.thrift`、`handler.go`、`service/drop_reward.go`、`service/drop_reward_test.go`、`dal/tcc.go`、`util/trace/pet_home.go`
- pet home：`idl/pet_home.thrift`、`service/pet_home*.go`、`dal/tcc_pet_home.go`
- feed：`idl/feed.thrift`、`service/feed*.go`、`docs/domains/feed.md`

### 4. 使用日志、trace 和平台数据

涉及 logid、trace、TCC、CDS、Codebase、MR、CI、APM、Slardar、Live Trace 或线上配置时，优先用只读 `bytedcli` / 内部平台命令取证。

先确认工具可用：

```bash
bytedcli --json auth status
```

如果 `bytedcli` 不在 PATH 或 npm cache 权限异常，使用临时 cache 和内部 registry：

```bash
NPM_CONFIG_CACHE=/private/tmp/bytedcli-npm-cache \
NPM_CONFIG_REGISTRY=http://bnpm.byted.org \
npx -y @bytedance-dev/bytedcli@latest --json auth status
```

大输出必须先落盘，再小片段提取：

```bash
mkdir -p /Users/bytedance/bytecode/Backend-Server/social_pet/.symphony/artifacts/{{ issue.identifier }}
NPM_CONFIG_CACHE=/private/tmp/bytedcli-npm-cache bytedcli log trace-tree --log-id '<logid>' \
  > /Users/bytedance/bytecode/Backend-Server/social_pet/.symphony/artifacts/{{ issue.identifier }}/trace-tree.txt 2>&1
rg -n '<method|psm|error|field>' /Users/bytedance/bytecode/Backend-Server/social_pet/.symphony/artifacts/{{ issue.identifier }}/trace-tree.txt
```

日志排查规则：

- 先验 `trace.method` / RPC 方法，确认 logid 落在哪条调用链。
- `trace-tree` 只做预览；如果需要证明具体服务行为，继续查目标 PSM 的原始日志。
- `social_pet` 主 PSM 优先看 `ttgame.social.pet`。
- 如果问题穿过 HTTP widget、IM bridge、网关或其他服务，不要停在本仓；继续把下游仓和 PSM 串进证据链。
- TCC/CDS 争议必须同时核对 env、region、deployment 和 key/path，不要只看代码默认值。

### 5. 本地复现和验证

只在能缩小问题或验证结论时运行本地测试。优先局部、可等待、有日志产物的命令：

```bash
run_dir="docs/test/log/run-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$run_dir/cmd" "$run_dir/artifacts"
CI_RUN_LOG_DIR="$PWD/$run_dir/artifacts" ./ci/run.sh '<TestRegex>'
```

读取顺序：

1. `script.verdict.txt`
2. `script.result.log`
3. `script.focus.log`
4. `script.raw.log`

等待规则：

- 初始至少等待 180 秒；如果脚本仍在跑，每 60 秒复查，最多 600 秒。
- 不要把初始化噪音、中间 FAIL 片段或其他包的 PASS/FAIL 当目标测试结论。
- 如果 `script.raw.log` 已出现目标测试自己的 `RUN/PASS/FAIL`、`panic:` 或 `fatal error:`，可以用 raw-first 收口。
- 10 到 12 分钟仍没有目标测试终态时，跑同名 direct `go test` 对照，区分脚本环境卡住和测试本身失败。

仅当触碰以下内容时，才升级到 build-coupled 验收：`build.sh`、`ci/run.sh`、`tool/metrics_cfg_checker`、`script/`、`conf/`、`go.mod`、`go.sum`、IDL 生成代码或构建流程。

### 6. 形成结论

结论必须绑定证据：

- 本地代码：`file:line`
- 命令输出：命令摘要 + 关键行
- 日志/trace：artifact 路径 + 命中行号 + 1 到 3 行摘录
- 配置/数据：env、region、key/path、查询时间和只读结果摘要
- 测试：命令、日志目录、目标测试终态

不要只说“可能是配置问题”或“怀疑下游转义”。要写清：

- 问题在哪一层首次出现
- 哪些层已被反证不是根因
- 为什么当前证据足以支持判断
- 最小修复或验证动作是什么

## 最终回复

最终回复必须使用以下结构：

```text
问题：
- ...

根因 / 当前判断：
- ...

证据：
- ...

建议：
- ...

状态：
- Done / Human Review / needs-info，并说明原因。
```

如果诊断完成，最终回复不要以 `Review: PASS` 或 `Merge: PASS` 开头；本 workflow 不走 AI Review、Merging 或 MR 流程。
