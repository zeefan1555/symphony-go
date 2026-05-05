# Hertz 控制面 IDL 与生成工作流

本文档固化第一版 Hertz-first 控制面的维护流程。它面向改 IDL、重新生成脚手架和 review 控制面改动的人。

## 目录边界

`idl/main.thrift` 是统一 Hertz 生成入口。这个文件拥有唯一 service 和所有业务 HTTP route annotations；维护者新增控制面业务接口时，必须先把 method 加到这个 service 中，再运行统一生成命令。

`idl/common.thrift` 放公共模型，例如 runtime state、issue detail 和 error envelope。这个文件只描述 transport-neutral 数据模型，不承载 HTTP 路由语义，也不能作为 service method 的顶层请求或响应。

`idl/control.thrift`、`idl/orchestrator.thrift`、`idl/workspace.thrift`、`idl/workflow.thrift` 和 `idl/codex_session.thrift` 是平铺领域 IDL。它们只放对应 method 的专属 `XxxReq` / `XxxResp` contract 和必要嵌套模型；可以引用公共模型作为字段，但不定义 service，也不放 route annotations。

`idl/workflow.thrift` 中的 `WorkflowIssueFlow` 是维护者可读的 issue 状态流转投影，用于解释 active/terminal states、review policy、phase routes、stage flows、transitions 和 dispatch rules。它不是新的状态机真相；实际执行仍由 workflow 配置、orchestrator 阶段路由和 agent 更新 issue 状态共同决定。默认执行边界是单 issue agent session，阶段 IDL 只描述流程，不表示每个阶段都要新建 agent。

`gen/hertz/...` 是 Hertz 生成代码外壳。`gen/hertz/handler`、`gen/hertz/model` 和 `gen/hertz/router` 是 Hertz 生成代码目录。这里的 model、router 和默认 handler skeleton 由 `hz` 生成，review 时不要把生成噪音当作主要讨论对象。

`internal/transport/hertzbinding/` 是长期手写 Hertz 绑定层。它保存当前 control service、定义 HTTP error envelope helper，并让生成 handler 不直接拥有业务逻辑。旧 `internal/transport/hertzhook/` 和 server-local `controlAdapter` 只属于迁移前形态。

`internal/transport/hertzserver/` 是手写 HTTP 传输层。它把 Hertz handler 边界接到仓库自己的 control service，并负责 HTTP 状态码、error envelope 和测试。

`internal/service/control/` 是手写 ControlService 业务服务边界。控制面业务语义先进入这里，再通过 snapshot provider 或 refresh trigger 等小接口连接 orchestrator/listener 能力。

## 新增接口流程

新增诊断控制面 API 时按固定顺序做：

1. 在对应平铺领域 IDL 中新增专属 `XxxReq` / `XxxResp`。即使请求或响应暂时为空，也必须定义独立结构体；不要复用其他接口的 Req/Resp，也不要让公共模型直接作为 service method 的顶层类型。
2. 在 `idl/main.thrift` 的唯一 `SymphonyAPI` service 中注册 method，并使用 `api.post` 动作式路由。业务 POST 接口统一从 request body 取输入，字段应使用 `api.body` 标明来源。
3. 运行 `make hertz-generate`，让 Hertz 刷新 `gen/hertz/handler`、`gen/hertz/model` 和 `gen/hertz/router`。
4. 生成 handler 只做 request bind、response 写出和对 `internal/transport/hertzbinding/` 的调用；不要在 handler 中解析 workflow、操作 workspace、启动 Codex 或读取 issue tracker。
5. 在 `internal/transport/hertzbinding/` 做 HTTP error envelope 和生成模型委托，在 `internal/service/control/` 做手写服务实现，并通过小接口调用现有 orchestrator、workspace、workflow、Codex runner 等核心能力。
6. 补测试：IDL/route contract、HTTP route smoke 或 fake delegate 测试、service 单元测试，以及对应 transport 测试。最后运行 `make hertz-layout-smoke` 和 `./test.sh ./...`。

## 生成命令

重新生成 Hertz 脚手架时，从仓库根目录运行：

```bash
make hertz-generate
```

`make hertz-generate` 会调用 `scripts/hertz_generate.sh`，脚本内部执行 `hz new`，以 `idl/main.thrift` 为入口，把生成结果写入 `gen/hertz/handler`、`gen/hertz/model` 和 `gen/hertz/router`。脚本只同步这三个目录，仓库现有程序入口继续由 `cmd/` 和手写服务入口负责。

需要重新生成的场景包括：新增、删除或重命名 HTTP route；修改 request/response IDL 类型；修改 Hertz route annotations；升级 Hertz/thriftgo 后需要刷新 handler skeleton。

## Review 重点

控制面变更应优先 review IDL 契约和手写传输层，并同步检查业务服务边界：先确认 `idl/main.thrift`、`idl/common.thrift`、`idl/control.thrift`、`idl/orchestrator.thrift`、`idl/workspace.thrift`、`idl/workflow.thrift` 与 `idl/codex_session.thrift` 的输入、输出、错误模型和 route annotations 是否符合诊断控制面 API 语义，再确认 `internal/transport/hertzbinding/` 与 `internal/transport/hertzserver/` 是否只承担 binding/HTTP 职责，最后确认 `internal/service/control/` 保持手写服务实现边界。

`gen/hertz/handler`、`gen/hertz/model` 和 `gen/hertz/router` 的大体积 diff 通常来自 `hz` 生成。review 时只需要确认生成命令来自 `make hertz-generate`，且生成结果和 IDL 变更一致；不要把生成代码噪音当成主要讨论对象。

## Transport 边界

公共模型 IDL 不能依赖 Hertz route annotations。`idl/common.thrift` 不应出现 `api.get`、`api.post`、`api.path` 等 Hertz annotation；这些只属于 `idl/main.thrift` 的主 service。

主 IDL 注册的业务 HTTP 接口统一使用 POST 和动作式路由，例如 `/api/v1/control/get-state`。即使请求或响应暂时为空，也必须定义专属 `XxxReq` / `XxxResp`；公共模型只能作为字段嵌套，不能直接充当 service method 的顶层 Req 或 Resp。

未来 Kitex 只能新增专用 RPC 传输层和 IDL，并复用公共模型和控制面语义。不要为了未来 RPC 消费者把 Hertz annotations 下沉到公共模型，也不要让 Kitex 设计污染当前 Hertz-first 结构。

第一版不实现 Kitex runtime，也不把 `run --once --issue` 变成产品 API。单次运行命令仍是本地调试入口；产品控制面只暴露当前 IDL 明确声明的 HTTP 能力。
