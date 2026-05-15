---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "symphony-test-c2a66ab0f2e7"
  active_states:
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
  cwd: /Users/bytedance/bytecode
agent:
  max_concurrent_agents: 1
  max_turns: 6
  review_policy:
    mode: human
    allow_manual_ai_review: false
    on_ai_fail: hold
codex:
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=medium app-server
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

这是 `/Users/bytedance/bytecode` 的只读诊断 workflow：目标是根据 issue 问题、本地代码仓库和 bytedcli 能力完成排查结论，不实现代码、不修改配置、不建立 git worktree、不创建分支、不提交、不发 PR。

{% if attempt %}
续跑上下文：

- 这是第 #{{ attempt }} 次重试，因为 ticket 仍处于 active 状态。
- 从当前诊断 workpad 和已收集证据继续，不要从头重做。
- 只要 issue 仍处于 active 状态，不要提前结束；唯一例外是真正缺少必要权限、密钥或工具。
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

1. 目标代码根目录固定为 `/Users/bytedance/bytecode`。
2. 当前 cwd 就是目标代码根目录；不要为 issue 创建额外 workspace、git worktree 或 scratch checkout。
3. 不要执行 `git worktree add`、`git checkout -b`、`git commit`、`git push`、`gh pr create`、`gh pr merge`，不要打开 `commit`、`pull` 或 `pr` skill。
4. 不要修改 `/Users/bytedance/bytecode` 下任何业务仓文件。允许的写入只有 Linear workpad/status，以及 `/Users/bytedance/bytecode/.symphony/artifacts/{{ issue.identifier }}/` 下的只读诊断产物。
5. 目标目录按只读证据源使用；如某个命令会生成代码、更新依赖、写缓存到目标仓或修改配置，先停止并记录为需要人工确认的 proposed command，不要执行。
6. 所有对外可见文字使用中文；命令、路径、日志原文、字段名和错误原文可以保留英文。

## Linear 操作约定

1. 只使用注入的 `linear_graphql` 读写 Linear；不要使用 Linear MCP/app 工具或 `linear` CLI。
2. 开始时读取 issue 最新状态、描述和评论，查找标题为 `## Codex Workpad` 的未 resolved comment。
3. 如果没有 workpad，创建一个；如果已有，只更新同一个 comment。
4. workpad 必须包含：
   - 环境戳：`<host>:/Users/bytedance/bytecode@<time-or-sha>`
   - 问题复述
   - 排查计划
   - 证据清单：代码证据必须写成可跳转链接，例如 `[service/foo.go:123](/Users/bytedance/bytecode/Backend-Server/social_pet/service/foo.go:123)`；命令或日志证据写 artifact 路径、命令摘要和关键行号。
   - 当前结论
   - Blocker 或需要人工继续的动作
5. 诊断完成且证据充分时，把 issue 状态更新为 `Done`。
6. 如果缺少权限、token、bytedcli auth、日志访问或 issue 信息不足，更新 workpad 后把状态更新为 `Human Review`，并写清最小解除条件。

## 排查方法

1. 先识别问题归属：
   - 根据 issue 标题、描述、logid、PSM、接口名、配置名、MR/Codebase 链接、报警链接、TCC/CDS 名称或业务词，在 `/Users/bytedance/bytecode` 下定位相关仓库。
   - 优先使用 `rg`、`rg --files`、`find -maxdepth`、`git -C <repo> status --short --branch`、`git -C <repo> remote -v` 做最小范围定位。
2. 读取目标仓规则：
   - 如果候选仓库存在 `AGENTS.md`，先读取并遵守。
   - 如果存在 README、go.mod、package.json 或本地 docs，只读取当前问题需要的部分。
3. 使用 bytedcli：
   - 适用场景：Unified skill for the entire bytedcli command surface. Use when tasks involve ByteDance internal R&D platforms and the agent should prefer bytedcli through CLI, MCP, or bundled references instead of opening web pages or hand-writing internal API calls.
   - 一旦判断要使用 bytedcli，先路由到 [`.codex/skills/bytedcli/SKILL.md`](/Users/bytedance/bytecode/.codex/skills/bytedcli/SKILL.md)，按 skill 的 Route by task 或 subskills index 找真实 domain；不要按场景词硬猜同名 domain，例如 ByteTech 文章走 `insearch`，不是 `bytetech`。先跑 `bytedcli --json auth status` 或 `bytedcli <domain> --help`，不要猜命令。
   - 需要稳定证据时默认加 `--json` 且只取必要字段；npx 包装时使用临时 npm cache：`NPM_CONFIG_CACHE=/private/tmp/bytedcli-npm-cache bytedcli ...`。
   - 日志、trace、TCC/CDS 大对象或任何长输出必须先完整落盘到 `/Users/bytedance/bytecode/.symphony/artifacts/{{ issue.identifier }}/`，例如：
     ```bash
     mkdir -p /Users/bytedance/bytecode/.symphony/artifacts/{{ issue.identifier }}
     NPM_CONFIG_CACHE=/private/tmp/bytedcli-npm-cache bytedcli log get-logid-log ... > /Users/bytedance/bytecode/.symphony/artifacts/{{ issue.identifier }}/logid-<psm>.txt 2>&1
     ```
   - 落盘后只用 `rg -n`、`sed -n`、`head`、`tail`、`wc -l` 提取小片段；workpad 只记录 artifact 路径、命令摘要、关键行号和 1-3 行摘录。
   - bytedcli 不可用或未登录时，不要安装新工具；记录 blocker 和建议的人类验证命令。
4. 形成结论：
   - 结论必须绑定证据：本地代码可跳转链接、命令输出摘要、日志片段、bytedcli 查询结果或明确的反证。
   - 区分“已确认事实”“基于证据的推断”“仍需外部验证”。
   - 不要给实现补丁；可以给最小修复建议、风险和下一步验证命令。

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
- Done 或 Human Review，并说明原因。
```

如果诊断完成，最终回复不要以 `Review: PASS` 或 `Merge: PASS` 开头；本 workflow 不走 AI Review/Merging/PR 流程。
