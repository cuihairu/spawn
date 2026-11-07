# 源码分层分析

本模型读取自 2026-11-07、提交 `bc9866d596d7c2e16fe65b08dda2a4af0a6e7596`。层表示职责，不是目录名。

## 清单

| 层 | 模块/服务 | 状态存储 | 配置输入 | 生命周期 |
|---|---|---|---|---|
| 1. 客户端体验层 | Expo 移动端 UI；React Web 页面 | SecureStore 会话；浏览器 localStorage 认证；实时客户端连接 | Expo 公共 API URL；Vite 服务地址 | AuthProvider 在渲染前恢复 SecureStore 会话；React 根组件管理轮询和 WebSocket 重连。证据：`apps/mobile-app/src/app/_layout.tsx:13-19`, `apps/mobile-app/src/auth/AuthContext.tsx:35-82`, `apps/web-client/src/App.tsx:23-119`, `apps/web-client/src/api/client.ts:116-128` |
| 2. 访问网关层 | `api-gateway` | 上传卷；本地代理缓冲；生成的路由状态 | `etc/gateway-api.yaml`、上游/JWT/上传环境变量 | 加载配置、创建服务上下文、注册 REST 处理器、追加 `/ws`、启动 go-zero。证据：`services/api-gateway/gateway.go:38-55`, `services/api-gateway/internal/config/config.go:8-34`, `services/api-gateway/internal/upload/upload.go:78-128` |
| 3. 领域服务层 | `user-service`、`game-catalog`、`community`、`content-service`、`data-panel` | 每服务 SQLite/MySQL；进程内 TTL+LRU 缓存；无服务自有文件 | 每服务 `internal/config/config.go` 加 `etc/*.yaml`；共享 JWT/上游/跨服务环境变量 | 统一执行 flag → 配置加载 → 服务上下文（Ping、建表、种子）→ 注册处理器 → 启动服务。证据：`services/user-service/user.go:32-58`, `services/game-catalog/game.go:30-52`, `services/community/community.go:30-51`, `services/content-service/content.go:30-52`, `services/data-panel/data-panel.go:30-51`; `services/user-service/internal/svc/servicecontext.go:43-81` |
| 4. 实时传输层 | `realtime-hub` | 内存客户端/在线注册表；可选 Redis 中继和 webhook 通知 | 自定义 YAML 加 JWT/内部令牌/Redis/webhook 环境变量 | Hub、中继和通知器装配后启动标准库 HTTP 服务；WebSocket 双向进出。证据：`services/realtime-hub/realtime-hub.go:34-67`, `services/realtime-hub/internal/config/config.go:16-37`, `services/realtime-hub/internal/hub/relay.go:44-119` |
| 5. 跨服务传输层 | 网关聚合客户端；用户→游戏客户端；内容→游戏韧性客户端 | 仅请求体和指标，无持久状态 | 网关/用户/内容的上游 URL 与超时设置 | 每请求执行；内容调用包重试、熔断和指标；网关组降级。证据：`services/api-gateway/internal/logic/homefeedlogic.go:20-61`, `services/user-service/internal/integration/gamecatalog_client.go:31-91`, `services/content-service/client/gamecatalog.go:18-68` |
| 6. 持久层 | 五个领域服务的 `model/` 包 | SQLite 或 MySQL 行；通用 TTL+LRU 模型缓存 | 每服务配置 schema 的 DSN 和驱动选择 | 服务上下文打开并 Ping 存储，在受理请求前执行建表/种子。证据：`services/game-catalog/internal/svc/servicecontext.go:34-58`, `services/game-catalog/model/gamemodel.go:72-119`, `services/community/internal/model/postmodel.go:24-69` |
| 7. 共享工具层 | `packages/config`、`packages/ui-kit`、`packages/data-models`、`packages/shared-utils`、`tools`、`tests` | 文件系统 Makefile/env 样例；部分包只有规划文档 | `packages/config/env/.env.example`、工作区脚本、公共 Makefile | 提供脚本/Make 目标和规划共享契约；不持有业务运行时状态。证据：`packages/config/make/common.mk:1-19`, `packages/config/env/.env.example:1-7`, `packages/README.md:1-9` |
| 8. 平台观测层 | `platform`、`deploy`、Docker Compose | 进程外的监控配置和日志/指标存储 | Compose 环境、Prometheus/Grafana/Loki/Promtail 配置 | Compose 接线服务生命周期、指标端口和采集代理。证据：`docker-compose.yaml:3-104`, `platform/README.md:1-9`, `deploy/prometheus/prometheus.yml:1-32` |

## 源码模块

- 移动端 UI：`apps/mobile-app/src/app/_layout.tsx:13`，路由组在 `apps/mobile-app/src/app`，API 契约 `apps/mobile-app/src/api/client.ts:9`。
- Web UI：`apps/web-client/src/App.tsx:23`，领域页面在 `src/pages`，API 契约在 `apps/web-client/src/api/client.ts:116`。
- 网关：`services/api-gateway/gateway.go:44`，生成处理器在 `internal/handler`，代理在 `internal/proxy`，聚合在 `internal/logic`，上传/实时适配器在 `internal/upload` 和 `internal/realtime`。
- 领域服务：五个入口加服务上下文、逻辑和模型文件分别在 `user-service`、`game-catalog`、`community`、`content-service`、`data-panel`。
- 实时：`services/realtime-hub/realtime-hub.go:34`，帧/在线注册表在 `internal/hub`，中继在 `internal/relay`，通知器在 `internal/push`。
- 跨服务客户端：网关集成客户端在 `internal/integration`；用户/游戏和内容/游戏客户端在各自集成或客户端包。
- 持久层：模型包在各领域服务；通用缓存在 `services/user-service/internal/cache/ttlcache.go:15`。
- 共享工具和平台：`packages/config/make/common.mk:1`、`packages/README.md:1`、`platform/README.md:1`、`deploy/`。

## 调用面

- 移动端 → 网关 REST 和 `/ws`：一个网关基础地址复用到所有 HTTP 服务常量（`apps/mobile-app/src/api/client.ts:9-13`）。
- Web → 指定服务直连：浏览器客户端从 Vite 运行时值解析用户/游戏/内容/社区地址（`apps/web-client/src/api/client.ts:116-128`）。
- 网关 → 五上游和实时隧道：compose URL 映射和网关注册展示精确通道（`docker-compose.yaml:8-18`, `services/api-gateway/gateway.go:48-54`）。
- 用户 → 游戏目录：推荐逻辑经配置化 REST 客户端拉取（`services/user-service/internal/integration/gamecatalog_client.go:31-91`）。
- 内容 → 游戏目录：REST 调用包重试、熔断和指标（`services/content-service/client/gamecatalog.go:18-68`, `services/content-service/client/breaker.go:22-68`）。
- 社区 → 实时中心：内部令牌 REST 发布器发送领域事件；浏览器只消费 WebSocket 流（`services/community/internal/realtime/publisher.go:14-74`, `services/community/internal/svc/service_context.go:154`）。

## 协议面

- REST 契约：各 Go 服务和网关的 `.api` 文件，如 `services/api-gateway/gateway.api:1`, `services/community/community.api:1`, `services/game-catalog/game.api:1`。
- RPC 文件：`services/user-service-rpc/user.proto:14` 只有 go-zero Ping 契约。
- 实时 JSON 帧：类型化 `Frame` 及房间/在线结构在 `services/realtime-hub/internal/hub/hub.go:16-41`；信号在 `services/realtime-hub/internal/hub/signal.go:5-36`。
- 上传协议：multipart 加魔数嗅探和网关确定性文件名（`services/api-gateway/internal/upload/upload.go:78-110`）。

## 实现、配置、协议、文案位置

| 层 | 实现位置 | 配置 schema | 专属协议文件 | 用户文案 |
|---|---|---|---|---|
| 客户端体验层 | `apps/*/src` 与 `apps/*/app` 下的 Expo 路由和 React 页面 | 应用 API 客户端中的构建期/运行时变量；包脚本 | 各 API 客户端内嵌类型接口 | 路由/页面组件中的界面文案和兜底提示 |
| 访问网关层 | `services/api-gateway/gateway.go:44`, `internal/proxy`, `internal/logic`, `internal/upload`, `internal/realtime` | `services/api-gateway/internal/config/config.go:8`, `etc/gateway-api.yaml` | `services/api-gateway/gateway.api` | 网关持有的分享卡 HTML 模板 |
| 领域服务层 | 各服务的入口/服务上下文/逻辑/模型文件 | 每服务 `internal/config/config.go`，`etc/*.yaml` | 每服务 `.api` 文件 | 逻辑/模型代码中的错误、状态和审核文案 |
| 实时传输层 | `services/realtime-hub/realtime-hub.go:34`, `internal/hub` | `services/realtime-hub/internal/config/config.go:16`, `etc/realtime-hub.yaml` | 代码内 JSON 帧和信号结构 | Hub 处理器返回的连接/错误 JSON |
| 跨服务传输层 | `internal/integration`、用户游戏客户端、内容韧性客户端 | 承载上游地址/超时的服务配置 schema | 无专属跨服务文件；生成 `.api` 加代码内 REST 客户端 | 集成服务映射后的上游消息 |
| 持久层 | 五个领域服务的模型包 | 服务配置 schema 中的 DSN 字段 | 建表/迁移辅助代码内嵌 SQL | 数据库约束错误转为 API 信封 |
| 共享工具层 | `packages/*`, `tools`, `tests` | `packages/config/env/.env.example`、Makefile、工作区脚本 | 无业务专属协议文件 | 包 README |
| 平台观测层 | Dockerfile、compose、deploy 配置、规划中的 platform 包 | `docker-compose.yaml`, `deploy/**` | Prometheus/Loki/Grafana 配置格式 | 面板和监控标签 |

## 为什么拆八层

1. **客户端体验层单独拆，因为两种传输拓扑都已存在。** 移动端把每个 HTTP 常量收敛到网关 URL（`apps/mobile-app/src/api/client.ts:9-13`），Web 解析四个直连领域服务地址（`apps/web-client/src/api/client.ts:116-128`）；合并会掩盖刻意保留的拓扑差异。
2. **网关层独立，因为它只拥有传输态。** 它在固定上限内缓冲请求体（`services/api-gateway/internal/proxy/upstream.go:45-63`），把上传留在网关卷（`services/api-gateway/internal/upload/upload.go:97-128`），并隧道 WebSocket 升级（`services/api-gateway/internal/realtime/tunnel.go:54-94`），不拥有业务表。
3. **领域服务保持一层，因为它们共享具体生命周期和鉴权契约。** 五个入口执行相同的 flag/加载/上下文/注册/启动序列（`services/user-service/user.go:32-58`, `services/game-catalog/game.go:30-52`, `services/community/community.go:30-51`, `services/content-service/content.go:30-52`, `services/data-panel/data-panel.go:30-51`），非签发服务校验同形声明（`services/game-catalog/internal/svc/servicecontext.go:64-75`）。
4. **持久层独立，因为一个服务上下文模式扇出到不同模型状态。** 上下文按 DSN 决定 SQLite/MySQL（`services/game-catalog/internal/svc/servicecontext.go:34-47`），再初始化模型建表/种子（`services/game-catalog/internal/svc/servicecontext.go:53-58`）；通用接线不同于领域规则。
5. **实时层独立，因为它的状态和失败语义不同于 REST。** 中心拥有内存在线和类型化 JSON 帧（`services/realtime-hub/internal/hub/hub.go:16-41`），Redis 中继是可选能力而非领域表依赖（`services/realtime-hub/internal/hub/relay.go:44-119`）。
6. **跨服务传输层独立，因为韧性策略不均衡。** 内容调用包重试、熔断和指标（`services/content-service/client/gamecatalog.go:18-68`, `services/content-service/client/breaker.go:22-68`），网关首页组独立降级（`services/api-gateway/internal/logic/homefeedlogic.go:38-61`）；这些策略不等于业务规则。
7. **共享工具层是构建/运行胶水层，不是产品状态拥有者。** 公共 Make 目标驱动工作区任务（`packages/config/make/common.mk:1-19`），环境模板集中开发输入（`packages/config/env/.env.example:1-7`）。
8. **平台观测层独立，因为部署与采集的生命周期超出进程态。** Compose 映射所有服务地址、端口和存储（`docker-compose.yaml:3-104`），Prometheus/Loki/Grafana/Promtail 在 `deploy/` 自持采集配置（`deploy/prometheus/prometheus.yml:1-32`）。

## 读取范围限制

- `packages/ui-kit`、`packages/data-models`、`packages/shared-utils` 目前由规划/README 材料代表，没有运行时源码。
- `services/user-service-rpc` 只有 Ping protobuf 契约，没有接入当前调用图的业务 RPC。
