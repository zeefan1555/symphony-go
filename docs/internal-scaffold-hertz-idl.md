# 内部架构脚手架 IDL 与 Hertz 生成契约

本文档记录旧内部架构脚手架 IDL 的迁移期契约。它描述内部子系统能力边界；当某个能力被明确纳入诊断控制面 API 时，外部 HTTP 路由仍必须通过 `idl/main.thrift` 和平铺领域 IDL 注册，而不是直接暴露 scaffold service。

当前长期方向以 ZEE-76 为准：标准 Hertz 根目录 `biz/...` 是控制面生成模型、handler skeleton 和 router 的权威生成边界；旧 `internal/generated` scaffold 生成链只作为 ZEE-78 之前的待退役遗留存在。本文档不得被理解为允许新增业务逻辑继续落到旧生成链或旧顶层 internal 包。

## 目录契约

`idl/scaffold/orchestrator.thrift` 描述 issue run 调度、运行状态投影和控制入口的能力边界。它只表达 orchestrator 对外可被调用的稳定能力，不描述 running map、goroutine 或 retry queue 的实现细节。

`idl/scaffold/workspace.thrift` 描述 workspace 路径解析、校验、准备和清理能力。路径安全、hook 执行和文件系统副作用仍由手写 workspace manager 拥有。

`idl/scaffold/codex_session.thrift` 描述 Codex session 和 turn 生命周期能力。它不暴露 Go callback、函数类型或 app-server wire protocol 细节。

`idl/scaffold/workflow.thrift` 描述 workflow 加载、reload、摘要和 prompt 渲染相关能力。workflow 文件语义、动态 reload 和模板渲染仍由现有手写包维护。

`internal/generated/hertz/scaffold/` 是旧内部脚手架 IDL 的 Hertz 模型生成目录。该目录可由生成命令重建，禁止承载手写业务逻辑，并将在 ZEE-78 中退役或迁出旧顶层 generated 边界。

## 生成命令

从仓库根目录运行：

```bash
make hertz-scaffold-generate
```

`make hertz-scaffold-generate` 是迁移期命令。它调用 `scripts/hertz_scaffold_generate.sh`，脚本使用 `hz model` 读取 `idl/scaffold/orchestrator.thrift`、`idl/scaffold/workspace.thrift`、`idl/scaffold/codex_session.thrift` 和 `idl/scaffold/workflow.thrift`，并把生成结果写入旧 `internal/generated/hertz/scaffold/` 边界。

生成结束后脚本会运行 `scripts/check_generated_hertz_boundary.sh`。该检查确认旧 generated 树与标准 `biz` 生成外壳下的 Go 文件都带有生成代码头；其中生成 model、router 和 handler 外壳不能直接导入 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow`、`internal/service` 或现有 issue tracker 接入包。脚本同时检查 `internal/service/` 不导入 Hertz `app.RequestContext`，也不直接依赖 issue tracker 集成。

## 边界说明

内部架构脚手架 IDL 是给维护者和 agent 看的实现边界地图。它按运行子系统分文件，帮助后续能力从 IDL 开始，再进入生成模型、手写传输层、业务服务层和现有核心包。

控制面 IDL 现在由根目录 `idl/` 的 `main.thrift` 与平铺领域 IDL 组成，用来承载诊断控制面 API。`idl/main.thrift` 是唯一 service 和所有业务 POST 接口 route annotations 的来源；平铺领域 IDL 只定义专属 Req/Resp 和嵌套模型。内部架构脚手架 IDL 不允许出现 `api.get`、`api.post` 或 `api.path` 这类 HTTP route annotations。

旧 scaffold 生成代码位于 `internal/generated/hertz/`，可被命令覆盖。手写传输层和业务服务层不放在生成目录内；后续 slice 应把 HTTP 入站协议逻辑放到 transport 语义边界，并通过小接口委托到业务服务或迁移期核心包。

手写传输层和业务服务层负责类型转换、错误语义和调用现有业务能力。orchestrator、workspace、Codex runner、workflow loader、Linear tracker 和日志在迁移期间可能仍由现有顶层包提供，但这些顶层包只允许作为兼容 shim 或待迁移遗留；新增业务逻辑必须落到 ZEE-76 定义的 service、runtime、integration 或 transport 语义边界。
