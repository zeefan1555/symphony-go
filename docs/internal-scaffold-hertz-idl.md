# 内部架构脚手架 IDL 与 Hertz 生成契约

本文档记录旧内部架构脚手架 IDL 的迁移期契约。它描述内部子系统能力边界；当某个能力被明确纳入诊断控制面 API 时，外部 HTTP 路由仍必须通过 `idl/main.proto` 和平铺领域 IDL 注册，而不是直接暴露 scaffold service。

当前长期方向以 ZEE-83 / ADR-0003 为准：`gen/hertz/...` 是控制面生成模型、handler skeleton 和 router 的权威生成边界；旧 `internal/generated` scaffold 生成链是 ZEE-78 中已退役遗留，不再保留目录、生成脚本或 Makefile 入口。本文档不得被理解为允许新增业务逻辑继续落到旧顶层 internal 包，也不得被理解为允许继续新增旧 scaffold Module。

## 目录契约

`idl/orchestrator.proto` 描述 issue run 调度、运行状态投影和控制入口的能力边界。它只表达 orchestrator 对外可被调用的稳定能力，不描述 running map、goroutine 或 retry queue 的实现细节。

`idl/workspace.proto` 描述 workspace 路径解析、校验、准备和清理能力。路径安全、hook 执行和文件系统副作用仍由手写 workspace manager 拥有。

`idl/codex_session.proto` 描述 Codex session 和 turn 生命周期能力。它不暴露 Go callback、函数类型或 app-server wire protocol 细节。

`idl/workflow.proto` 描述 workflow 加载、reload、摘要和 prompt 渲染相关能力。workflow 文件语义、动态 reload 和模板渲染仍由现有手写包维护。

旧内部脚手架 IDL 目录和旧生成输出已经退役。控制面类型以 `gen/hertz/model/...` 为模型来源，手写 `internal/transport/hertzbinding` 只依赖这些 Hertz 生成模型包并委托到 `internal/service/control`。

## 生成命令

从仓库根目录运行标准 Hertz 生成入口：

```bash
make hertz-generate
```

该入口先运行 `buf lint`，再以 `idl/main.proto` 为唯一 service 来源，并通过根目录平铺领域 IDL 生成 `gen/hertz/model`、`gen/hertz/handler` 和 `gen/hertz/router`。旧 scaffold 生成命令已经删除；新增或修改诊断控制面接口时，先改根目录 IDL，再更新手写 service / transport 绑定。

生成结束后应运行 `scripts/check_generated_hertz_boundary.sh`。该检查确认 `gen/hertz/...` 生成外壳下的 Go 文件都带有生成代码头；其中生成 model、router 和 handler 外壳不能直接导入 `internal/service/orchestrator`、`internal/service/workspace`、`internal/service/codex`、`internal/service/workflow`、`internal/service` 或现有 issue tracker 接入包。脚本同时检查 `internal/service/` 不导入 Hertz `app.RequestContext`，也不直接依赖 issue tracker 集成。

## 边界说明

内部架构脚手架 IDL 的角色已经收束为迁移期契约记录。实现边界地图现在由根目录领域 IDL、`CONTEXT.md` 和 ADR 共同描述，帮助后续能力从 IDL 开始，再进入生成模型、手写传输层、业务服务层和现有核心包。

控制面 IDL 现在由根目录 `idl/` 的 `main.proto` 与平铺领域 IDL 组成，用来承载诊断控制面 API。`idl/main.proto` 是唯一 service 和所有业务 POST 接口 route annotations 的来源；平铺领域 IDL 只定义专属 Req/Resp 和嵌套模型。内部架构脚手架 IDL 不允许出现 `(api.get)`、`(api.post)` 或 `(api.path)` 这类 HTTP route annotations。

旧 scaffold 生成代码是已退役遗留，不能再被命令重建。手写传输层和业务服务层不放在生成目录内；HTTP 入站协议逻辑放到 transport 语义边界，并通过 Hertz 绑定层委托到 `internal/service/control`。

手写传输层和业务服务层负责类型转换、错误语义和调用现有业务能力。orchestrator、workspace、Codex runner、workflow loader、Linear tracker 和日志能力必须落到 ZEE-76 定义的 service、runtime、integration 或 transport 语义边界。
