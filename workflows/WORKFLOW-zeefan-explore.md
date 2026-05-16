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
  cwd: /Users/bytedance/zeefan-explore
agent:
  max_concurrent_agents: 10
  max_turns: 20
  review_policy:
    mode: human
    allow_manual_ai_review: false
    on_ai_fail: hold
codex:
  command: codex --config shell_environment_policy.inherit=all --sandbox danger-full-access --config 'model="gpt-5.5"' --config model_reasoning_effort=medium app-server
  read_timeout_ms: 60000
  approval_policy: never
  thread_sandbox: danger-full-access
  turn_sandbox_policy:
    type: dangerFullAccess
---

 `{{ issue.identifier }}`，是你正在处理 Linear ticket

这是 `/Users/bytedance/zeefan-explore` 的探索 workflow：目标是围绕 Twitter、ByteTech、公司内网文章、外部工具、CLI、MCP、技能包和临时 PoC 做分析或试用，并把最终结论沉淀回 Linear issue 的单一 `## Codex Workpad`。本 workflow 默认不做业务代码实现、不创建分支、不提交、不发 PR；默认直接使用本机环境，只有安装、clone、下载或 PoC 需要落盘时才按需隔离。

{% if attempt %}
续跑上下文：

- 这是第 #{{ attempt }} 次重试，因为 ticket 仍处于 active 状态。
- 从当前 `## Codex Workpad` 和 `.explore/issues/{{ issue.identifier }}/` 证据继续，不要从头重做。
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

1. 目标工作区固定为 `/Users/bytedance/zeefan-explore`；当前 cwd 就是该目录。
2. 不要为探索任务创建 git worktree、分支、commit、push 或 PR。
3. 默认只写入 Linear workpad/status；不要为了只读探索预先创建或进入 sandbox。
4. 不把外部 repo clone、下载包、node_modules、venv、cache、认证文件、cookie、session 或内网长文写入 Git tracked 文件。
5. 如果 issue 明确要求改探索仓自身规则、脚本或技能，先在 workpad 说明原因和改动范围，再只改必要文件。
6. 除非 issue 明确需要历史上下文，不要主动读取个人 memory；结论以 Linear、repo-local 技能、命令输出和来源内容的实时证据为准。
7. 所有对外可见文字使用中文；命令、路径、包名、日志原文和错误原文可以保留英文。

## Linear 操作约定

1. Linear 读写按可靠程度选择通道，优先级从高到低如下；每次实际使用的通道和工具名都要写入 workpad：
   - **第一优先：Codex app Linear 工具**，例如 `mcp__codex_apps__linear._list_comments`、`mcp__codex_apps__linear._search`、`mcp__codex_apps__linear._save_comment`、`mcp__codex_apps__linear._save_issue`。这是默认通道，和 Codex app 注入、结构化参数、评论/状态写回最贴近；ZEE-200/ZEE-202/ZEE-204 已验证可完成 workpad 和状态流转。
   - **第二优先：专用 `linear` MCP server**，例如工具面出现的 `mcp__linear__*`。只在 Codex app Linear 工具缺失、某个必需操作不可用，或 issue 明确要求验证 MCP 时使用；使用前必须确认当前启动事件里 `linear` 为 `ready`，并且本机配置是 `bearer_token_env_var = "LINEAR_API_KEY"`，不要回退到已失效的 OAuth refresh token 路径。ZEE-204 已验证 bearer 模式的 `linear` startup 为 `ready`。
   - **第三优先：`linear_graphql` dynamic tool**。只在前两种通道不可用、需要精确字段级操作，或 issue 明确要求 GraphQL 时使用；使用前先读 `.agents/skills/linear/SKILL.md`，GraphQL 查询必须窄字段、一次一个 operation，先查 issue/team/status 的精确 ID，再 `commentCreate`/`commentUpdate`/`issueUpdate`，不要查询 `Issue.links` 等已知不稳定宽字段。
   - 不把 `linear` CLI 当作本 workflow 的常规 Linear 写回通道；除非 issue 明确要求 CLI 诊断，否则不要用它创建/更新 workpad 或状态。
   - 当前 smoke 观察到 `mcp__codex_apps__linear._research` 在子 agent 工具面可能出现但实际返回 `Tool research not found`，不要依赖它完成必需读写。
   - 如果三种通道都不可用，或可用通道无法完成 comment/status 的最小写回，把 issue 转为 `Human Review`，写清最小解除条件。
2. 开始时读取 issue 最新状态、描述和评论，查找标题为 `## Codex Workpad` 的未 resolved comment。
3. 如果没有 workpad，创建一个；如果已有，只更新同一个 comment。
4. 服务层可能已在 prompt 到达前把 `Todo` issue 移到 `In Progress`；如果读取时仍是 `Todo`，先移到 `In Progress`，否则记录观察到的当前状态，不要声称自己完成了未实际执行的状态变更。
5. workpad 必须包含：
   - 环境戳：`<host>:/Users/bytedance/zeefan-explore@<time-or-sha>`
   - 目标
   - 来源
   - 官方身份核对
   - 工具链选择
   - 实验步骤
   - 证据
   - 结论
   - 风险
   - 清理状态
6. 探索完成且证据充分时，把 issue 状态更新为 `Done`。
7. 如果缺少权限、登录态、内部文档访问、网络、API key 或 issue 信息不足，更新 workpad 后把状态更新为 `Human Review`，并写清最小解除条件。
8. 不要把不同 Linear 通道混为一谈：优先使用最高可用通道；如果 fallback，必须在 workpad 记录主通道失败的具体工具名、错误文本和切换原因。创建/更新 workpad 后必须复读 comments，确认只保留一个未 resolved 的 `## Codex Workpad`；状态变更后必须复读 issue 状态。



## 场景分流

### Twitter/X 工具探索

1. 用 OpenCLI 读取或检索来源，不直接依赖截图、转述或记忆：
   - 先读 `.agents/skills/opencli-usage/SKILL.md` 或 `.agents/skills/smart-search/SKILL.md`。
   - 通过 `opencli list -f yaml` 和 `opencli <site> -h` 确认当前 registry、站点名、子命令和参数。
   - 拉取原帖、作者、发布时间、链接、核心 claim；如果 OpenCLI 取不到，再记录失败命令和最小替代来源。
2. 对工具做官方身份核对：
   - Twitter/X 原帖是否指向官网、GitHub、npm、PyPI、Homebrew、Cargo、Go module 或 release 页。
   - registry 包名、GitHub owner、README install 命令是否一致。
   - 如果同名包存在歧义，停止安装，只写核对结果和推荐人工确认点。
3. 如需安装或试用外部工具，只在临时实验目录内进行：
   - clone 到 `.explore/issues/{{ issue.identifier }}/src/`
   - 下载物放 `.explore/issues/{{ issue.identifier }}/downloads/`
   - 可执行 wrapper 或临时 bin 放 `.explore/issues/{{ issue.identifier }}/bin/`
4. 最小验证只证明当前 claim：
   - CLI 工具：`--help`、版本号、一个无副作用 dry-run 或示例命令。
   - SDK/库：安装成功、import 成功、README 最小示例跑通。
   - 服务型工具：只跑本地端口或 dry-run，不默认接入真实账号和长期服务。

### ByteTech/内网探索

1. 用 BytedCLI 读取允许访问的信息：
   - 先读 `.agents/skills/bytedcli/SKILL.md`。
   - 不确定命令面时先跑 `bytedcli --help`、`bytedcli insearch --help` 或对应子命令 help。
   - ByteTech/ByTech/内部文档优先用 `bytedcli insearch` 或 repo-local bytedcli reference 中的最新入口。
   - 本机已安装用户级 `bytedcli` 全局命令；只读文章总结不要再用 issue-scoped `NPM_CONFIG_CACHE`，除非直接执行失败并需要记录最小绕过证据。
   - 默认直接使用本机 `HOME` 和已有 SSO/ByteCloud/Feishu 登录态；不要为只读文章总结创建 sandbox。
2. 只保存必要摘要：
   - workpad 记录链接、标题、作者或来源、发布时间、核心结论和必要短摘录。
   - 不把内网长文、截图、cookie、token、原始 HTML 或完整导出写入 Git tracked 文件。
3. 内部工具如果需要安装、clone、下载或本地 PoC，才使用 `.explore/issues/{{ issue.identifier }}/`：
   - BytedCLI 拉取的包、脚本、缓存、临时配置都放 `.explore/issues/{{ issue.identifier }}/`。
   - 不要为了隔离内部工具而覆盖 `HOME`；否则会丢失本机登录态并误报 `AUTH_REQUIRED`。
   - 如果工具必须写全局配置、登录态或系统服务，先写清原因和影响范围，除非 issue 明确要求，否则转 `Human Review`。
4. 验证标准必须是可复查证据：
   - 命令、关键输出、日志路径、版本号、最小 smoke 结果。
   - 对权限不足、登录态缺失或网络限制，写清最小解除条件。

### Repo-local 技能和插件

1. 本仓专属 agent skill、OpenCLI adapter、MCP/plugin 说明、脚本模板默认安装到 `.agents/skills/`、`scripts/` 或 `templates/`，不要安装到 `~/.codex/skills`、其他业务仓或全局插件目录。
2. 新增或更新 Skill 时保持高内聚：
   - `SKILL.md` 要能独立说明触发场景、输入、执行步骤、验证方式和安全边界。
   - 可以引用同目录 `references/`、`scripts/`、`examples/`，但不要依赖另一个仓库的隐含说明才能运行。
   - 中文描述为默认；命令、参数和错误原文可保留英文。
3. 外部 skillpack 或插件导入前先做来源核对：
   - 官方 repo、版本、安装命令、目标目录、会修改的 managed block。
   - 如果默认安装目标是全局目录，必须改成 repo-local 目标；无法改目标时停止安装并写 blocker。
4. 只有满足以下条件才追踪到 Git：
   - 是规则、Skill、脚本、模板、轻量示例或索引文件。
   - 不包含认证、缓存、大文件、外部 repo clone、下载包、内网原文。
   - 已通过 `git diff --check`，脚本还要通过语法检查。

### 通用 PoC

1. 先写清要验证的单一 claim，不扩大到无关能力评测。
2. 默认不创建 sandbox；能用本机只读命令确认的，就只把命令摘要和关键输出写入 workpad。
3. 只有外部工具试装、clone、下载或临时 PoC 需要落盘时，才手动创建 `.explore/issues/{{ issue.identifier }}/` 下的临时目录，并把证据放在 `.explore/issues/{{ issue.identifier }}/evidence/`。
4. 用户叫停安装或试用时，立即停止新的尝试；如果本次创建了临时实验目录，清理其中的安装产物/cache/tmp，并把清理状态写回 workpad。

## 验证与收口

1. 完成前至少验证：
   - 如果没有使用 sandbox，明确记录“未创建/未保留 sandbox 产物”。
   - 如果使用了临时实验目录，记录创建的目录、用途和清理状态。
   - 所有安装、clone、cache、下载物都留在 `.explore/issues/{{ issue.identifier }}/` 下，或已清理。
   - 需要保留的证据已经写入 workpad。
   - 不需要保留的试用产物已经清理。
2. 如果改了探索仓 tracked 文件，至少运行：
   ```bash
   git diff --check
   ```
   如果改了 shell 脚本，额外运行：
   ```bash
   zsh -n <script>
   ```
3. 不要用 `Review: PASS`、`Merge: PASS` 或 `Push: PASS` 收口；本 workflow 不走 AI Review/Merging/PR。

## 最终回复

最终回复必须使用以下结构：

```text
目标：
- ...

结论：
- ...

证据：
- ...

风险：
- ...

清理状态：
- ...

状态：
- Done 或 Human Review，并说明原因。
```
