# Symphony Go Context

Symphony Go 是一个以 Linear issue 驱动编码代理执行的本地自动化服务。本上下文记录领域语言，避免把控制面契约、运行时实现和传输机制混在一起。

## Language

**控制面 API**:
面向操作者或外部系统的粗粒度服务接口，用来启动、停止、查询和调试 Symphony 运行。
_Avoid_: 内部状态机接口、运行时 struct 镜像

**控制面 IDL**:
用 IDL 表达的控制面 API 契约，只描述未来可能被外部调用的接口和稳定消息。
_Avoid_: 把所有 Go struct 原样翻译成 thrift

**内部架构脚手架 IDL**:
用 IDL 按子系统能力描述内部实现边界，帮助维护者和 agent 理解职责并生成框架脚手架。
_Avoid_: 外部 HTTP API, 运行时 struct 镜像

**公共模型 IDL**:
可被 HTTP 和 RPC 控制面共同复用的稳定数据模型定义。
_Avoid_: HTTP 路由 IDL, Kitex 专用模型

**HTTP 控制面**:
面向人、浏览器、curl 和 dashboard 的 HTTP 传输适配层。
_Avoid_: Symphony 核心服务

**Hertz 管理代码**:
以 IDL 和 Hertz 生成的骨架函数作为接口入口和目录约束，核心业务逻辑仍由手写 adapter/service 实现。
_Avoid_: 让 Hertz 生成 orchestrator、workspace 或 agent runner 的业务实现

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
- **内部架构脚手架 IDL** 描述 orchestrator、workspace、agent runner 等内部子系统的能力边界，但默认不暴露成外部可调用 API。
- **内部架构脚手架 IDL** 按运行子系统分文件，而不是按 workflow 状态或一次性需求分文件。
- **Hertz 管理代码** 要求新增能力先改 IDL、再生成骨架函数、最后把核心实现接到手写 adapter/service。
- **Hertz 管理代码** 可以成为主应用外壳的一部分，但根目录 `build.sh` 仍是本仓手写权威入口，不由生成命令直接覆盖。
- Hertz 生成代码统一放在 `internal/generated/hertz/...`，手写应用装配放在 `internal/app/...`，核心业务逻辑保留在手写 service/adapter 层。
- 第一版 Hertz-owned shell 不迁移现有核心业务包；`internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/linear` 等继续保留原边界。
- **公共模型 IDL** 可以被 **HTTP 控制面** 和 **RPC 控制面** 共同使用。
- **运行时状态** 可以被投影成控制面响应，但不是 IDL 的来源真相。
- **监听服务** 是默认运行形态；单 issue 过滤或单轮 poll 只属于诊断/测试辅助，不进入第一版 **控制面 IDL**。

## Example dialogue

> **Dev:** “要不要把 orchestrator 的 running map 放进 thrift?”
> **Domain expert:** “不要。IDL 只定义控制面 API；running map 是运行时状态，只能通过状态查询响应被投影出来。”
>
> **Dev:** “那能不能用 IDL 描述 workspace 和 agent runner 的能力边界，方便生成脚手架?”
> **Domain expert:** “可以，但这是内部架构脚手架 IDL；除非明确进入操作面，否则不要自动变成 HTTP 控制面。”
>
> **Dev:** “Hertz 管理代码是不是业务逻辑也都生成?”
> **Domain expert:** “不是。Hertz 管理 IDL、骨架函数和启动入口，核心逻辑仍然写在手写实现层。”

## Flagged ambiguities

- “抽 IDL”已解析为抽取 **控制面 IDL**，不是把现有 Go struct 批量翻译成 thrift。
- “核心接口抽 IDL”已解析为抽取 **内部架构脚手架 IDL**，按能力分类描述内部边界并生成脚手架，不默认暴露外部 HTTP API。
- “按类别分文件”已解析为按运行子系统分文件，例如 orchestrator、workspace、agent runner、workflow、tracker、observability；不按 workflow 状态阶段分文件。
- “Hertz 管理代码”已解析为 IDL-first 生成骨架函数和应用外壳，不是生成业务状态机或覆盖手写核心实现。
- “改造 Hertz 生成到根目录的 build.sh”已解析为保留根目录 `build.sh` 作为手写权威入口，并让它接入 Hertz-owned shell；不允许 `hz --force` 直接覆盖根目录构建脚本。
- “生成代码目录”已解析为 `internal/generated/hertz/...`；该目录禁止手写业务逻辑。
- “手写业务目录”已解析为保留现有核心包边界，只新增 `internal/app` 做应用装配，不新建泛化 `internal/services`。
- `run --once --issue` 已解析为诊断/测试辅助，不是 Symphony 的主领域能力。
- 第一版外部控制面已解析为优先实现 **HTTP 控制面**；**RPC 控制面** 保留为后续适配层。
