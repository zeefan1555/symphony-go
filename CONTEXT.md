# Symphony Go Context

Symphony Go 是一个以 Linear issue 驱动编码代理执行的本地自动化服务。本上下文记录领域语言，避免把控制面契约、运行时实现和传输机制混在一起。

## Language

**控制面 API**:
面向操作者或外部系统的粗粒度服务接口，用来启动、停止、查询和调试 Symphony 运行。
_Avoid_: 内部状态机接口、运行时 struct 镜像

**控制面 IDL**:
用 IDL 表达的控制面 API 契约，只描述未来可能被外部调用的接口和稳定消息。
_Avoid_: 把所有 Go struct 原样翻译成 thrift

**公共模型 IDL**:
可被 HTTP 和 RPC 控制面共同复用的稳定数据模型定义。
_Avoid_: HTTP 路由 IDL, Kitex 专用模型

**HTTP 控制面**:
面向人、浏览器、curl 和 dashboard 的 HTTP 传输适配层。
_Avoid_: Symphony 核心服务

**RPC 控制面**:
面向后端服务或 agent 平台调用的 RPC 传输适配层。
_Avoid_: 默认本地 dashboard 接口

**监听服务**:
启动后按 workflow 配置持续轮询 issue tracker 并调度 eligible issues 的 Symphony 主运行形态。
_Avoid_: 单 issue 手动执行器

**运行时状态**:
orchestrator 在单次服务运行中维护的内部调度、重试、agent session 和观测数据。
_Avoid_: 控制面契约

**Issue Run**:
Symphony 针对一个 issue 创建或复用 workspace、执行 agent、记录进度并推动 workflow 状态的完整尝试。
_Avoid_: task, job

## Relationships

- 一个 **控制面 API** 可以读取或触发一个或多个 **Issue Run**。
- **控制面 IDL** 描述 **控制面 API** 的稳定边界，不直接拥有 **运行时状态**。
- **公共模型 IDL** 可以被 **HTTP 控制面** 和 **RPC 控制面** 共同使用。
- **运行时状态** 可以被投影成控制面响应，但不是 IDL 的来源真相。
- **监听服务** 是默认运行形态；单 issue 过滤或单轮 poll 只属于诊断/测试辅助，不进入第一版 **控制面 IDL**。

## Example dialogue

> **Dev:** “要不要把 orchestrator 的 running map 放进 thrift?”
> **Domain expert:** “不要。IDL 只定义控制面 API；running map 是运行时状态，只能通过状态查询响应被投影出来。”

## Flagged ambiguities

- “抽 IDL”已解析为抽取 **控制面 IDL**，不是把现有 Go struct 批量翻译成 thrift。
- `run --once --issue` 已解析为诊断/测试辅助，不是 Symphony 的主领域能力。
- 第一版外部控制面已解析为优先实现 **HTTP 控制面**；**RPC 控制面** 保留为后续适配层。
