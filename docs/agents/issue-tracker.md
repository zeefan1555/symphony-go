# Linear 任务跟踪

本仓的任务和 PRD 记录在 Linear。当前仓库工作流使用 `WORKFLOW.md` 中配置的 Linear 项目。

## 约定

- 当前 MCP 冒烟 workflow 中，派生会话的 Linear 读写优先使用 Linear MCP/app 工具。
- 不要在 MCP 冒烟里退回 `linear` CLI 或 `linear_graphql`；如果派生会话没有可用 MCP/app 工具，应记录 blocker。
- 对外可见文本默认使用中文，包括 Linear workpad 和交接记录。
- 工作流执行类任务使用一个标题为 `## Codex Workpad` 的持久 Linear 评论作为进度事实源。
- 除非用户或工作流明确要求其他状态，新建后续任务默认放到 `Backlog`。

## 当技能要求“发布到任务跟踪系统”

在本仓配置的 Linear 项目中创建或更新 Linear issue。内容应包含清晰标题、描述、验收标准、验证计划和相关链接。

## 当技能要求“获取相关 ticket”

通过 identifier 或 URL 获取 Linear issue，并包含描述、当前状态、标签、评论，以及已有的 `## Codex Workpad`（如果存在）。
