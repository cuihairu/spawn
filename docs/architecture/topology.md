# 现状拓扑与服务边界（2026-10-04）

> 本文记录当前实际落地的服务边界、端口、调用拓扑与数据流；
> 愿景层面的架构方向见同目录 `overview.md`。两者冲突时以本文为准。

## 1. 服务清单与边界

| 服务 | 语言/框架 | 业务端口 | /metrics | 职责边界 | 存储现状 |
|------|----------|---------|----------|---------|---------|
| user-service | Go / go-zero | 8888 | 9091 | 账号（注册/登录/JWT 签发）、用户资料、成长体系；经 GameCatalogClient 跨服务拉游戏推荐 | MySQL（默认 DSN 指向本机 3306） |
| game-catalog | Go / go-zero | 8890 | 9092 | 游戏库、列表筛选、详情、精选/推荐榜单 | SQLite（默认 `file:data/games.db`）或 MySQL |
| content-service | Go / go-zero | 8891 | 9093 | 攻略 CRUD/发布/点赞、评论（含嵌套回复） | SQLite（默认 `file:data/content.db`）或 MySQL |
| community | Go / go-zero | 8892 | 9094 | 帖子流/热榜、话题圈子、关注关系（话题/用户）、点赞/分享计数、关注流、我的点赞 | SQLite/MySQL（post_likes 关系表等启动建表） |
| api-gateway | Go / go-zero | 8800 | 9095 | BFF：自有聚合 handler + 四路反代（见 §3） | 无状态 |
| user-service-rpc | Go / gRPC | 8080 (gRPC) | 9096 | 内网 gRPC 通道（手机/Web 均不直连；protoc 生成代码缺口保留原样） | — |

未落地（README 规划占位）：data-panel、matchmaking、realtime-hub、crawler-jobs、admin-console、mini-program。

## 2. 客户端接入拓扑

两条接入路径并存，互不影响：

```
web-client (React/Vite, :5173)
  └─ 直连各服务（src/api/client.ts 硬编码 localhost:88xx）
       ├─ user-service :8888        （登录/注册/资料/推荐）
       ├─ game-catalog  :8890       （游戏列表/详情/精选）
       ├─ content-service :8891     （攻略/评论）
       └─ community :8892           （帖子/话题/关注/点赞）

mobile-app (Expo/RN)
  └─ 统一经 api-gateway :8800（BFF 第一阶段，2026-10-04 切换）
       EXPO_PUBLIC_API_URL 覆盖网关域名；
       开发默认 host:8800（Android 模拟器 10.0.2.2）
```

web-client 直连拓扑保持不变，网关路由扩容不要求 web 迁移。

## 3. api-gateway 路由面

### 3.1 自有聚合 handler（internal/handler + internal/logic）

| 路由 | 说明 | 上游 |
|------|------|------|
| POST /auth/login | 登录聚合（token + user_info） | user-service |
| GET /games/featured | 精选透传 | game-catalog |
| GET /users/:id/recommendations | 跨服务推荐（Bearer 透传） | user-service → game-catalog |
| GET /s/p/:id | 分享卡跳板页（html/template og meta + `spawn://` 深链按钮） | community |
| GET /home/feed | BFF 第二阶段列表聚合：精选游戏/热帖/话题/攻略四路并发，逐组字段裁剪（正文/图片等大字段不下发，帖子正文截 60 字摘要）；单上游故障降级为空组并记 `degraded`，不做整体 5xx | game-catalog + community + content-service |

### 3.2 反代路由（internal/{users,games,community,content}，proxy.NewUpstream passthrough）

| 前缀 | 路由 | 上游 |
|------|------|------|
| users | POST /auth/register、GET /users/:id | user-service |
| games | GET /games、GET /games/:id | game-catalog |
| community | /api/v1/topics**、/api/v1/posts**、/api/v1/users/{following,likes} 等 M1–M3 全量调用面 | community |
| content | /api/v1/guides**、/api/v1/comments** | content-service |

反代语义：path+query 透传、hop-by-hop 头剥离、请求体缓冲定长转发（1 MiB 上限，超限 413。缓冲是为绕开 go-zero 上游不解析 chunked 请求体的缺陷）、状态码/响应体原样回传（匿名 401 等语义保持）。

## 4. 认证与跨服务数据流

```
注册/登录:  client → 网关 → user-service（签发 JWT，HS256 共享密钥
            "tappi-user-service-jwt-secret-key-2024"，各服务 env=JWT_SECRET 覆盖）
带鉴权请求:  client → 网关（反代原样透传 Bearer / 自有 handler 透传）
            → community 等 AuthMiddleware 用 utils.Auth.ParseToken 严格验签
            （JWTClaims 结构跨服务一致，无需二次调用 user-service）
跨服务拉取:  user-service → game-catalog（推荐）；网关 → community/content/game-catalog（聚合/分享卡）
```

## 5. 可观测性与部署

- go-zero 内置指标经各服务 Prometheus agent 暴露 `/metrics`（端口见 §1）；
  api-gateway 日志 JSON 输出 stdout，由 Promtail 收集进 Loki（deploy/docker-compose.monitoring.yaml）。
- `docker compose up --build` 一键拉起全部服务；各服务目录有独立 Dockerfile。
- 配置：`etc/*.yaml`，上游地址支持环境变量覆盖（`USER_SERVICE_URL` 等）。

## 6. 演进路线（与 README「下一步建议」对齐）

1. BFF 第三阶段（按需）：更多聚合口径（列表聚合已落地 `/home/feed`，字段裁剪模式可复制）；必要时给聚合端点加缓存。
2. 真机回归：Android release 包已可本地 gradle 出包（`pnpm --dir apps/mobile-app build:android:release`），本地推送/深链分享卡等待真机验证。
3. data-panel 落地后：mobile 小黑盒式战绩面板（M4 另立项）；api-gateway 反代/聚合模式照搬。
4. realtime-hub / matchmaking：实时通道独立于网关，客户端直连 WebSocket 网关（规划）。
5. 生产化：网关前置 TLS 终结（当前 compose 无证书）、明文 HTTP 收敛、CORS 收紧。
