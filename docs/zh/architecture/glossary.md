# 源码分层术语表

本表收录跨实现边界复用的抽象概念。每条引用定义该行为的源码，而不是目录名。

| 术语 | 英文 | 定义 | file:line |
|---|---|---|---|
| 网关路由注册器 | Gateway Route Registry | 服务先注册生成的 REST 路由，再把实时 WebSocket 隧道作为独立路由族挂载。 | `services/api-gateway/gateway.go:48`, `services/api-gateway/gateway.go:54` |
| 上游目录 | Upstream Catalog | 网关配置在分发前映射五个 HTTP 服务族和可选实时中心的 URL。 | `services/api-gateway/internal/config/config.go:11-34` |
| 缓冲代理 | Buffered Proxy | 网关在 1 MiB 上限内缓冲请求体后转发，使上游请求具有确定长度。 | `services/api-gateway/internal/proxy/upstream.go:45-63` |
| 逐组降级 | Per-Group Feed Degrade | 首页聚合的各组独立失败：单上游错误只清空该组并标记降级，不拖垮整个响应。 | `services/api-gateway/internal/logic/homefeedlogic.go:38-61` |
| 上传存储 | Upload Store | 网关本地磁盘保存魔数校验后的上传，并用白名单安全解析公开文件名。 | `services/api-gateway/internal/upload/upload.go:97-128` |
| JWT 声明契约 | JWT Claims Contract | 用户服务签发身份声明；其他 Go 服务校验共享声明结构，不回调用户服务。 | `services/user-service/internal/svc/servicecontext.go:31-38`, `services/game-catalog/internal/svc/servicecontext.go:64-75` |
| 双 DSN 开关 | Dual DSN Switch | 持久层按 `file:` 数据源打开 SQLite，否则打开 MySQL，使本地与部署服务共用模型层。 | `services/user-service/internal/svc/servicecontext.go:43-58`, `services/game-catalog/internal/svc/servicecontext.go:34-47` |
| TTL + LRU 缓存 | TTL + LRU Cache | 模型级缓存同时执行过期淘汰和最久未用淘汰，保护存储免受重复读取冲击。 | `services/user-service/internal/cache/ttlcache.go:15-104` |
| 跨服务韧性客户端 | Client Resilience Client | 内容服务的游戏目录客户端围绕有界 REST 调用加入重试、熔断和 Prometheus 指标。 | `services/content-service/client/gamecatalog.go:18-22`, `services/content-service/client/breaker.go:22-68` |
| 实时帧注册表 | Realtime Frame Registry | 实时中心持有内存中的 WebSocket 客户端、房间在线状态，并分发类型化 JSON 帧。 | `services/realtime-hub/internal/hub/hub.go:16-41` |
| 实时中继 | Realtime Relay | 可选 Redis 扇出把中心事件送到其他中心进程，同时保留进程内通道契约。 | `services/realtime-hub/internal/hub/relay.go:44-119` |
| 内部事件通知器 | Internal Event Notifier | 社区发布器经共享内部令牌向实时中心发送服务源事件，浏览器不接触业务状态。 | `services/community/internal/realtime/publisher.go:14-74`, `services/community/internal/svc/service_context.go:154` |
| 共享运行时配置 | Shared Runtime Config | 打包的 Makefile 加环境模板为客户端和服务任务提供统一本地开发入口。 | `packages/config/make/common.mk:1-19`, `packages/config/env/.env.example:1-7` |
| 客户端会话钩子 | Client Session Hook | 移动端认证保留 SecureStore 会话，并通过 React Context 暴露登录、登出和刷新状态。 | `apps/mobile-app/src/auth/AuthContext.tsx:13-82` |
| 直连扇出 | Direct Service Fan-Out | Web 客户端保留显式的分域服务地址，浏览器调用不必被迫全部经过网关。 | `apps/web-client/src/api/client.ts:116-128` |
