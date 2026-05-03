# 内部架构脚手架 IDL 与 Hertz 生成契约

本文档固化第一版内部架构脚手架 IDL 的目录、生成命令和边界检查。它只描述内部子系统能力边界，不把这些能力默认暴露为外部 HTTP 控制面。

## 目录契约

`idl/scaffold/orchestrator.thrift` 描述 issue run 调度、运行状态投影和控制入口的能力边界。它只表达 orchestrator 对外可被 adapter 调用的稳定能力，不描述 running map、goroutine 或 retry queue 的实现细节。

`idl/scaffold/workspace.thrift` 描述 workspace 路径解析、校验、准备和清理能力。路径安全、hook 执行和文件系统副作用仍由手写 workspace manager 拥有。

`idl/scaffold/codex_session.thrift` 描述 Codex session 和 turn 生命周期能力。它不暴露 Go callback、函数类型或 app-server wire protocol 细节。

`idl/scaffold/workflow.thrift` 描述 workflow 加载、reload、摘要和 prompt 渲染相关能力。workflow 文件语义、动态 reload 和模板渲染仍由现有手写包维护。

`internal/generated/hertz/scaffold/` 是内部脚手架 IDL 的 Hertz 模型生成目录。该目录可由生成命令重建，禁止承载手写业务逻辑。

## 生成命令

从仓库根目录运行：

```bash
make hertz-scaffold-generate
```

`make hertz-scaffold-generate` 调用 `scripts/hertz_scaffold_generate.sh`，脚本使用 `hz model` 读取 `idl/scaffold/orchestrator.thrift`、`idl/scaffold/workspace.thrift`、`idl/scaffold/codex_session.thrift` 和 `idl/scaffold/workflow.thrift`，并把生成结果写入 `internal/generated/hertz/scaffold/`。

生成结束后脚本会运行 `scripts/check_generated_hertz_boundary.sh`。该检查确认 `internal/generated/hertz/` 下的 Go 文件都带有生成代码头，并且不导入 `internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow` 或 `internal/linear` 等核心业务包。

## 边界说明

内部架构脚手架 IDL 是给维护者和 agent 看的实现边界地图。它按运行子系统分文件，帮助后续能力从 IDL 开始，再进入生成模型、手写 adapter/service 和现有核心包。

控制面 IDL 位于 `idl/control/`，只描述 operator-facing HTTP 控制面。控制面 IDL 可以带 Hertz route annotations；内部架构脚手架 IDL 不允许出现 `api.get`、`api.post` 或 `api.path` 这类 HTTP route annotations。

生成代码位于 `internal/generated/hertz/`，可被命令覆盖。手写 adapter/service 不放在生成目录内；后续 slice 应把 adapter 放在对应手写目录中，并通过小接口委托到现有核心包。

手写 adapter/service 负责类型转换、错误语义和调用现有业务能力。orchestrator、workspace、Codex runner、workflow loader、Linear tracker 和日志仍由现有核心包拥有，第一版不迁移这些包，也不新增泛化 `internal/services` 桶。
