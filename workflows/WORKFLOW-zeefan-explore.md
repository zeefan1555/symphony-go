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
  command: codex --config shell_environment_policy.inherit=all --config 'model="gpt-5.5"' --config model_reasoning_effort=medium app-server
  read_timeout_ms: 60000
  approval_policy: never
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    readOnlyAccess:
      type: fullAccess
    networkAccess: true
    excludeTmpdirEnvVar: false
    excludeSlashTmp: false
---

你正在处理 Linear ticket `{{ issue.identifier }}`。

这是 `/Users/bytedance/zeefan-explore` 的探索 workflow：目标是围绕 Twitter、ByteTech、公司内网文章、外部工具、CLI、MCP、技能包和临时 PoC 做隔离分析或试用，并把最终结论沉淀回 Linear issue 的单一 `## Codex Workpad`。本 workflow 默认不做业务代码实现、不创建分支、不提交、不发 PR。

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
3. 默认只允许写入 Linear workpad/status，以及 `/Users/bytedance/zeefan-explore/.explore/issues/{{ issue.identifier }}/` 下的 issue sandbox。
4. 不把外部 repo clone、下载包、node_modules、venv、cache、认证文件、cookie、session 或内网长文写入 Git tracked 文件。
5. 如果 issue 明确要求改探索仓自身规则、脚本或技能，先在 workpad 说明原因和改动范围，再只改必要文件。
6. 所有对外可见文字使用中文；命令、路径、包名、日志原文和错误原文可以保留英文。

## Linear 操作约定

1. 只使用注入的 `linear_graphql` 读写 Linear；不要使用 Linear MCP/app 工具或 `linear` CLI 作为兜底。
2. 开始时读取 issue 最新状态、描述和评论，查找标题为 `## Codex Workpad` 的未 resolved comment。
3. 如果没有 workpad，创建一个；如果已有，只更新同一个 comment。
4. 对 `Todo` issue，先移动到 `In Progress`，再创建或更新 workpad。
5. workpad 必须包含：
   - 环境戳：`<host>:/Users/bytedance/zeefan-explore@<time-or-sha>`
   - 目标
   - 来源
   - 官方身份核对
   - 实验步骤
   - 证据
   - 结论
   - 风险
   - 清理状态
6. 探索完成且证据充分时，把 issue 状态更新为 `Done`。
7. 如果缺少权限、登录态、内部文档访问、网络、API key 或 issue 信息不足，更新 workpad 后把状态更新为 `Human Review`，并写清最小解除条件。

## 探索方法

1. 读取并遵守 `/Users/bytedance/zeefan-explore/AGENTS.md`。
2. 根据任务类型读取 repo-local skill：
   - 通用 issue 执行：`.codex/skills/explore-issue/SKILL.md`
   - 外部工具安装或试用：`.codex/skills/tool-probe/SKILL.md`
   - Twitter、ByteTech 或内网资料分析：`.codex/skills/source-analysis/SKILL.md`
3. 对不同来源先读取对应 Agent Skill，再调用工具；不要凭记忆猜命令：
   - Twitter/X 内容：先读 repo-local `.codex/skills/source-analysis/SKILL.md`，再读本机 OpenCLI 相关 Agent Skill，例如 `/Users/bytedance/.skills-manager/skills/smart-search/SKILL.md` 或 `/Users/bytedance/.skills-manager/skills/opencli-usage/SKILL.md`。通过 `opencli list -f yaml` 和 `opencli <site> -h` 确认当前 registry、站点名、子命令与参数后，再用 OpenCLI 拉取或检索原始内容。
   - ByteTech/ByTech、公司内网文章或内部知识：先读 repo-local `.codex/skills/source-analysis/SKILL.md`，再读 `/Users/bytedance/.skills-manager/skills/bytedcli/SKILL.md`。优先用 BytedCLI 的 `insearch` 域拉取或检索，例如 `bytedcli insearch get --target "<ByteTech URL>"`；不确定命令时先看 `bytedcli insearch --help`。
   - 如果对应目录下存在更专门的 Agent Skill 或 reference，优先按该 Skill 的最新说明执行，并把实际读取的 Skill 路径写入 evidence 或 workpad。
4. 进入强隔离 sandbox：
   ```bash
   source scripts/explore-issue.sh {{ issue.identifier }}
   ```
5. 外部工具安装前必须先核对官方身份：
   - 官方 repo 或官网
   - registry 包名和发布者
   - README 推荐入口
   - CLI bin 名称
   - 会写入的路径
   - 许可、网络、登录态或内网权限要求
6. Twitter 或公开内容分析要区分原始事实、作者观点、二手转述和未验证推断。
7. ByteTech 或公司内网内容只保存链接、标题、要点和必要短摘录；不要复制长篇原文到本地 tracked 文件或 workpad。
8. 证据统一放在 `.explore/issues/{{ issue.identifier }}/evidence/`，workpad 只写相对路径、命令摘要和关键输出。
9. 用户叫停安装或试用时，立即停止新的尝试，清理当前 issue sandbox 中的安装产物/cache/tmp，并把清理状态写回 workpad。

## 验证与收口

1. 完成前至少验证：
   - issue sandbox 已创建且隔离环境变量生效。
   - 所有安装、clone、cache、下载物都留在 `.explore/issues/{{ issue.identifier }}/` 下。
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
