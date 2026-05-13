# Linear 任务跟踪

本仓的任务和 PRD 记录在 Linear。当前仓库工作流使用 `workflows/WORKFLOW-symphony-go.md` 中配置的 Linear 项目。

## 约定

- 当前无人值守 workflow 中，派生会话的 Linear 读写使用注入的 `linear_graphql` 工具，和 listener 的 Linear GraphQL client 保持同一路径。
- 不要在无人值守 child session 中使用 Linear MCP/app issue/comment 写入；MCP 写入可能触发审批，无法保证全自动化。
- 不要从派生会话退回 `linear` CLI；如果 `linear_graphql` 不可用，应记录 blocker，让外层 listener/orchestrator 接管状态和 workpad 写入。
- 对外可见文本默认使用中文，包括 Linear workpad 和交接记录。
- 工作流执行类任务使用一个标题为 `## Codex Workpad` 的持久 Linear 评论作为进度事实源。
- 除非用户或工作流明确要求其他状态，新建后续任务默认放到 `Backlog`。

## 当技能要求“发布到任务跟踪系统”

在本仓配置的 Linear 项目中创建或更新 Linear issue。内容应包含清晰标题、描述、验收标准、验证计划和相关链接。

## 当技能要求“获取相关 ticket”

通过 identifier 或 URL 获取 Linear issue，并包含描述、当前状态、标签、评论，以及已有的 `## Codex Workpad`（如果存在）。
