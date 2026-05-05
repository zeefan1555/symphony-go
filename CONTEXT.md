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
_Avoid_: 稳定产品 API, 运行时 struct 镜像

**诊断控制面 API**:
通过 Hertz 暴露内部子系统能力的本地 HTTP 接口，用于 operator、TUI、agent、调试和 smoke harness。
_Avoid_: 稳定产品 API, 远程公网 API

**公共模型 IDL**:
可被 HTTP 和 RPC 控制面共同复用的稳定数据模型定义。
_Avoid_: HTTP 路由 IDL, Kitex 专用模型

**接口顶层契约**:
每个 IDL service method 独占拥有的顶层请求和响应结构，命名为 `MethodNameReq` 和 `MethodNameResp`。
_Avoid_: 复用公共 Empty, 直接返回公共模型, 多个接口共享同一个 Req 或 Resp

**主 IDL 入口**:
`idl/main.thrift` 中拥有唯一 Hertz service 和全部 HTTP route annotations 的生成入口。
_Avoid_: 只 include 子 IDL, 子 IDL 内定义 service route

**业务 POST 接口**:
所有由主 IDL 入口注册的业务 HTTP 接口统一使用 POST 和 body 参数。
_Avoid_: 业务 GET 接口, query 参数作为默认传参方式

**动作式路由**:
按领域和方法名 kebab-case 组织的 HTTP 路由，例如 `/api/v1/workspace/prepare-workspace`。
_Avoid_: REST 资源式路径, 在路径里重复 HTTP method 名

**HTTP 控制面**:
面向人、浏览器、curl 和 dashboard 的 HTTP 传输适配层。
_Avoid_: Symphony 核心服务

**Hertz 管理代码**:
以标准 Hertz 工程布局作为主应用 HTTP 外壳，根目录 `biz/` 由 IDL 和 hz 生成代码管理，核心业务逻辑仍由手写 internal 层实现。
_Avoid_: internal/generated/hertz 隐藏式控制面生成区、让 Hertz 生成 orchestrator、workspace 或 agent runner 的业务实现

**手写服务实现**:
HTTP handler 绑定请求后委托的业务实现层，负责控制面语义、类型转换、副作用和对核心包的调用。
_Avoid_: 生成 handler 内业务逻辑, 生成目录内状态机

**业务服务层**:
位于 `internal/service/...` 的手写核心业务命名空间，按领域承载 orchestrator、workspace、codex、workflow、control 等业务逻辑，并供 Hertz handler 调用。
_Avoid_: 泛化 helper 包、HTTP handler、IDL 生成模型、仅作为控制面 facade 的薄转发层

**运行支撑层**:
长期位于 `internal/runtime/...` 的本地 daemon 支撑命名空间，承载应用配置、日志事件、观测快照和进程运行偏好。
_Avoid_: Symphony 业务状态机、HTTP handler、第三方系统接入

**外部集成层**:
长期位于 `internal/integration/...` 的第三方系统接入命名空间，Linear issue tracker 是其中一个具体集成。
_Avoid_: workflow 状态机、Issue Run 调度、Hertz handler

**传输层**:
长期位于 `internal/transport/...` 的入站协议适配命名空间，Hertz HTTP hook/server 属于这里。
_Avoid_: workflow 解析、workspace 操作、Codex 启动、Linear 读取

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

**Issue Tracker 集成**:
外部集成层中的 issue 管理系统接入能力，供业务服务读取和推进 issue 状态。
_Avoid_: 业务服务层、workflow 状态机、Hertz handler

**应用配置**:
运行支撑层中的 Viper 应用内配置读取与解析边界，默认读取 `conf/config.yaml`，承载本机/项目级 key-value、阈值、名单和默认行为。
_Avoid_: pkg/config 公共库、handler 或 service 中写死规则数据、workflow 执行定义

**Workflow 配置**:
描述 issue 执行流程、tracker 状态、prompt 和运行策略的工作流配置。
_Avoid_: 本机个性化 key-value、可独立于 workflow 切换的默认偏好、合入目标分支

**合入目标分支**:
Symphony 在 Merging 阶段用于更新、合入和同步的目标 Git 分支，属于应用级操作偏好。
_Avoid_: workflow merge.target 长期来源、硬编码 main

## Relationships

- 一个 **控制面 API** 可以读取或触发一个或多个 **Issue Run**。
- **控制面 IDL** 描述 **控制面 API** 的稳定边界，不直接拥有 **运行时状态**。
- **内部架构脚手架 IDL** 描述 orchestrator、workspace、agent runner 等内部子系统的能力边界，并可通过 **诊断控制面 API** 暴露为本地 HTTP 路由。
- **内部架构脚手架 IDL** 按运行子系统分文件，而不是按 workflow 状态或一次性需求分文件。
- **主 IDL 入口** include 平铺的领域 IDL 文件；子 IDL 只定义 **接口顶层契约** 和可嵌套模型，不定义 service。
- **主 IDL 入口** 中的 service method 默认都是 **业务 POST 接口**；健康检查等非业务路由不属于该规则。
- **业务 POST 接口** 使用 **动作式路由**，路径中的动作名与 service method 名保持可映射。
- **Hertz 管理代码** 要求新增 HTTP 接口先改 IDL、再生成路由注册和 handler 骨架、最后委托到 **手写服务实现**。
- **Hertz 管理代码** 使用根目录 `biz/handler`、`biz/model`、`biz/router` 作为 hz 管理的标准外壳，手写业务不得进入 `biz/model` 或 `biz/router`。
- **Hertz 管理代码** 可以成为主应用外壳的一部分，但 `cmd/symphony-go/main.go` 和根目录 `build.sh` 仍是本仓手写权威入口，不由生成命令直接覆盖。
- Hertz 控制面生成代码迁移到根目录 `biz/...`，核心业务逻辑保留在手写 internal 层。
- `internal/service/...` 是核心业务命名空间；`internal/orchestrator`、`internal/workspace`、`internal/codex`、`internal/workflow`、`internal/control` 等现有顶层业务包只允许作为迁移期兼容 shim 或待迁移遗留包。
- **业务服务层** 可以调用基础设施型内部包，但不得导入 Hertz `app.RequestContext`。
- **Issue Tracker 集成** 不迁入 `internal/service/...`；Linear 具体实现的长期归属是 `internal/integration/linear`，现有 `internal/issuetracker/linear` 只允许作为迁移期遗留边界。
- **应用配置** 的归属是 `internal/runtime/config`，不迁移到 `pkg/config`；旧顶层 `internal/config` 已删除。
- 日志和观测能力的归属是 `internal/runtime/logging` 与 `internal/runtime/observability`；旧顶层 `internal/logging` 与 `internal/observability` 已删除。
- Hertz hook/server 的长期归属是 `internal/transport/hertz...`，现有 `internal/control/hertz*` 只允许作为迁移期遗留边界。
- **应用配置** 与 **Workflow 配置** 分工明确：应用级个性化默认值不应为了读取方便重复塞进 workflow。
- **合入目标分支** 的长期优先级为 CLI `--merge-target` > `conf/config.yaml` > 默认 `main`；旧 `workflow merge.target` 仅作为兼容期来源。
- **合入目标分支** 在 `conf/config.yaml` 中的 canonical key 是 `git.merge_target`。
- Makefile 的 `MERGE_TARGET` 保留为一次性 CLI 覆盖入口；设置后等价于传入 `--merge-target`，优先级高于 `conf/config.yaml`。
- Viper 应用配置不支持环境变量覆盖，避免配置来源继续扩散。
- 兼容期内如果 `conf/config.yaml` 和旧 `workflow merge.target` 同时存在，`conf/config.yaml` 生效，并记录 deprecation warning 提示 workflow 字段已被忽略。
- **公共模型 IDL** 可以被 **HTTP 控制面** 和 **RPC 控制面** 共同使用。
- 所有 IDL service method 必须通过专属 **接口顶层契约** 接收请求和返回响应；**公共模型 IDL** 只能作为字段嵌套，不能直接充当 service method 的顶层 Req 或 Resp。
- **运行时状态** 可以被投影成控制面响应，但不是 IDL 的来源真相。
- **监听服务** 是默认运行形态；单 issue 过滤或单轮 poll 只属于诊断/测试辅助，不进入第一版 **控制面 IDL**。

## Internal Directory Contract

长期 internal 顶层目录按人类可读职责收敛为少数语义根：

- `internal/service/...`：Symphony 核心业务能力，包括 issue run 调度、workspace lifecycle、Codex session、workflow 语义和 control semantics。
- `internal/runtime/...`：本地 daemon 运行支撑，包括配置解析、日志事件、观测快照和进程级偏好。
- `internal/integration/...`：第三方系统接入，包括 Linear issue tracker client、fake、状态推进和 blocker normalization。
- `internal/transport/...`：入站协议层，包括 Hertz HTTP hook/server、HTTP error envelope 和协议模型转换。
- `biz/...`：标准 Hertz 生成外壳，是控制面生成模型、handler skeleton 和 router 的权威来源。

迁移顺序为：先统一文档和边界检查；再退役旧 `internal/generated` scaffold 生成链；再拆分顶层 `internal/types`；再迁移 runtime、integration 和 transport；最后收口 service-rooted 业务迁移与 smoke。当前旧 `internal/generated` 与 `internal/types` 已删除；迁移期间仍允许其他旧顶层 shim，但 shim 只能转发到新归属，不得承载新增业务逻辑。

不再把 `adapter` 或 `platform` 作为长期目录名：前者不能清楚表达第三方系统接入，后者不能清楚表达本地运行支撑。若某个能力无法归入上述语义根，应先更新 PRD 或创建 follow-up，再新增目录。

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
- “Hertz 管理代码”已解析为 IDL-first 生成标准 Hertz 根目录外壳，不是生成业务状态机或覆盖手写核心实现。
- “改造 Hertz 生成到根目录的 build.sh”已解析为保留根目录 `build.sh` 作为手写权威入口，并让它接入 Hertz-owned shell；不允许 `hz --force` 直接覆盖根目录构建脚本。
- “生成代码目录”最初解析为 `internal/generated/hertz/...`，现已改为标准 Hertz 根目录 `biz/...`；旧内部 scaffold 生成链只作为 ZEE-78 之前的待退役遗留，不是长期权威边界。
- “Hertz 标准根目录布局”已解析为迁移 `biz/` 生成外壳，不迁移程序入口；主入口仍是 `cmd/symphony-go/main.go`。
- “手写业务目录”已解析为新增统一 `internal/service/...`，并将 orchestrator、workspace、codex、workflow、control 等核心业务包逐步迁入该层，而不是只作为 handler facade。
- `run --once --issue` 已解析为诊断/测试辅助，不是 Symphony 的主领域能力。
- 第一版外部控制面已解析为优先实现 **HTTP 控制面**；**RPC 控制面** 保留为后续适配层。
- “每个接口必须独立定义 Req/Resp”已解析为所有 IDL service method 的 **接口顶层契约** 规则，不只约束外部 **控制面 IDL**。
- “内部 scaffold 变成外部 HTTP 路由”已解析为暴露 **诊断控制面 API**，不是把这些接口承诺为稳定产品 API。
- “所有核心函数都被 Hz 管理”已解析为 Hz 管理 HTTP 契约、路由注册、handler 骨架和生成模型；业务逻辑仍由 **手写服务实现** 拥有。
- “统一 main.thrift 生成”已解析为 **主 IDL 入口** 拥有唯一 service 和所有 route annotations；只 include 子 IDL 不足以让 Hertz 注册子 service 路由。
- “默认 POST”已解析为主 IDL 注册的业务 HTTP 接口全部使用 **业务 POST 接口**，以便统一本地调试和 agent/TUI 调用。
- “路由命名”已解析为使用 **动作式路由**，按领域和 method kebab-case 映射，不采用 REST 资源式路径。
- “issue 管理器”已解析为 **Issue Tracker 集成**；Linear 是具体实现，长期目录为 `internal/integration/linear`，不属于 **业务服务层**。
- “pkg/config”已解析为不采用；配置模块长期归入 `internal/runtime/config`，但所有业务阈值、名单和 KV 数据必须通过该边界读取。
- “conf/config.yaml”已解析为采用；它由 Viper 读取，用于应用级个性化 key-value，不替代 **Workflow 配置**。
- “merge.target”已解析为从 **Workflow 配置** 迁出；长期来源为 CLI 覆盖或 `conf/config.yaml`，旧 workflow 字段进入兼容期。
- “conf/config.yaml 与 workflow merge.target 冲突”已解析为 `conf/config.yaml` 赢，并对旧 workflow 字段记录 deprecation warning。
- “合入目标分支配置 key”已解析为 `git.merge_target`。
- “Makefile MERGE_TARGET”已解析为保留；它是 CLI 覆盖入口，不是新的配置真相来源。
- “Viper 环境变量覆盖”已解析为不支持；配置来源保持为 CLI、`conf/config.yaml` 和代码默认值。
