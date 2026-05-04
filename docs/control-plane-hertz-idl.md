# Hertz 控制面 IDL 与生成工作流

本文档固化第一版 Hertz-first 控制面的维护流程。它面向改 IDL、重新生成脚手架和 review 控制面改动的人。

## 目录边界

`idl/control/common.thrift` 放公共模型，例如 runtime state、issue detail、refresh result 和 error envelope。这个文件只描述 transport-neutral 数据模型，不承载 HTTP 路由语义。

`idl/control/http.thrift` 放 HTTP 控制面 contract。它可以引用公共模型，并通过 Hertz `api.get`、`api.post`、`api.path` annotations 描述 `/api/v1/*` 路由。

`biz/handler`、`biz/model` 和 `biz/router` 是 Hertz 生成代码目录。这里的 model、router 和默认 handler scaffold 由 `hz` 生成，review 时不要把生成噪音当作主要讨论对象。

`internal/control/hertzhook/` 是手写 Hertz hook 层。它保存当前 control service、定义 HTTP error envelope helper，并让生成 handler 不直接拥有业务逻辑。

`internal/control/hertzserver/` 是手写 HTTP adapter。它把 Hertz handler 边界接到仓库自己的 control service，并负责 HTTP 状态码、error envelope 和测试。

`internal/service/control/` 是手写 ControlService 业务服务边界。控制面业务语义先进入这里，再通过 snapshot provider 或 refresh trigger 等小接口连接 orchestrator/listener 能力。

## 生成命令

重新生成 Hertz 脚手架时，从仓库根目录运行：

```bash
make hertz-generate
```

`make hertz-generate` 会调用 `scripts/hertz_generate.sh`，脚本内部执行 `hz new`，以 `idl/control/http.thrift` 为入口，把生成结果写入 `biz/handler`、`biz/model` 和 `biz/router`。脚本只同步这三个目录，仓库现有程序入口继续由 `cmd/` 和手写服务入口负责。

需要重新生成的场景包括：新增、删除或重命名 HTTP route；修改 request/response IDL 类型；修改 Hertz route/path annotations；升级 Hertz/thriftgo 后需要刷新 scaffold。

## Review 重点

控制面变更应优先 review IDL 契约和手写 adapter，并同步检查业务服务边界：先确认 `idl/control/common.thrift` 与 `idl/control/http.thrift` 的输入、输出、错误模型和 route annotations 是否符合产品语义，再确认 `internal/control/hertzhook/`、`internal/control/hertzserver/` 是否只承担 hook/adapter 职责，最后确认 `internal/service/control/` 保持协议无关的业务服务边界。

`biz/handler`、`biz/model` 和 `biz/router` 的大体积 diff 通常来自 `hz` 生成。review 时只需要确认生成命令来自 `make hertz-generate`，且生成结果和 IDL 变更一致；不要把生成代码噪音当成主要讨论对象。

## Transport 边界

公共模型 IDL 不能依赖 Hertz route annotations。`idl/control/common.thrift` 不应出现 `api.get`、`api.post`、`api.path` 等 Hertz annotation；这些只属于 HTTP 控制面 IDL。

未来 Kitex 只能新增专用 adapter/IDL 层，并复用公共模型和控制面语义。不要为了未来 RPC 消费者把 Hertz annotations 下沉到公共模型，也不要让 Kitex 设计污染当前 Hertz-first 结构。

第一版不实现 Kitex runtime，也不把 `run --once --issue` 变成产品 API。单次运行命令仍是本地调试入口；产品控制面只暴露当前 IDL 明确声明的 HTTP 能力。
