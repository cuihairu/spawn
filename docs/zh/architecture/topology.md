# 现状拓扑与服务边界（2026-10-10 更新）


> 本文记录当前实际落地的服务边界、端口、调用拓扑与数据流；
> 愿景层面的架构方向见同目录 `overview.md`。两者冲突时以本文为准。

## 1. 服务清单与边界

| 服务 | 语言/框架 | 业务端口 | /metrics | 职责边界 | 存储现状 |
|------|----------|---------|----------|---------|---------|
| user-service | Go / go-zero | 8888 | 9091 | 账号（注册/登录/JWT 签发）、用户资料；经 GameCatalogClient 跨服务拉游戏推荐 | MySQL（默认 DSN 指向本机 3306） |
| game-catalog | Go / go-zero | 8890 | 9092 | 游戏库、列表筛选、详情、精选/推荐榜单 | SQLite（默认 `file:data/games.db`）或 MySQL |
| content-service | Go / go-zero | 8891 | 9093 | 攻略 CRUD/发布/点赞、评论（含嵌套回复） | SQLite（默认 `file:data/content.db`）或 MySQL |
| community | Go / go-zero | 8892 | 9094 | 帖子流/热榜/关键词搜索、话题圈子、关注关系（话题/用户）、点赞/分享计数、关注流、我的点赞、站内通知（点赞/评论/回复/关注四类，唯一键去重）、私信、举报审核 | SQLite/MySQL（post_likes、notifications 关系表等启动建表） |
| data-panel | Go / go-zero | 8896 | 9097 | 战绩摄入（JWT 保护，单调增量 upsert）与玩家战绩查询（公开，win_rate/kd 读侧 4 位小数） | SQLite/MySQL + TTL/LRU 缓存 |
| courier-adapter | Go / go-zero | 8897 | 9098 | GP-201 平台账号 ↔ Courier 适配：courier_game_mapping（GP-002 映射/能力）、platform_courier_binding（GP-001 一对一绑定，代登录随机密码 AES-GCM 密文落库）、游戏启动票据（一次性/短期/不落盘）、注销同步；管理面角色经 game-catalog /me 权威判定 | SQLite（默认 `file:data/adapter.db`）或 MySQL；绑定/映射/审计三表 + adapter_audit_logs |
| payments | Go / go-zero | 8898 | 9099 | GP-202 支付接入：商品配置（服务端定价 + 商户归属）、玩家下单/订单查询（Courier /v1/payments/* 只读转发 + 金额/币种核验）、渠道 S2S 回调验签接缝（沙箱 HMAC 仅开发，真实渠道原生验签待 ChannelAdapter + 渠道密钥）；订单事实源在 Courier teller，不复制状态机。GP-203 发货接缝：订单→区服/角色/商品关联（下单时校验区服归属/开放）+ teller Deliver 钩子的签名 webhook 落地面（`POST /internal/deliveries`，验签先于解析，未配置密钥 fail-closed 403）+ 游戏服发货接口配置（每游戏 endpoint+secret，管理面）+ 按订单幂等投递账本 + 有权限补单/目标回填 + 定时恢复（退避重投，重启续扫）；teller Deliver 生产接线归 Courier 提案/GP-005，本服务不复制订单状态机。GP-204 订单运营面：玩家我的订单列表（Courier 只读转发）+ 运营订单检索/详情经只读投影（order_projection 事实副本，状态= Courier 上报值 CREATED/PAID/DELIVERED）+ 操作审计（order_audit_logs）+ 退款/冲正管理适配（refund_requests 登记 PENDING 意图 + 状态映射，执行待 Courier 退款端点提案，不伪造成功）。GP-205 厂商对账单：从只读投影按商户维度聚合实付/优惠承担（暂 0，优惠券未接 GP-305）/退款冲减（只计 EXECUTED），算合同分成基数（split_base=实付-优惠-退款）与差异（base-结算单人工录入）；先人工确认不自动打款（F23）+ CSV 导出（明细+汇总）；无全局结算 HTTP API（边界 6），对账单为快照不旁路契约 | SQLite（默认 `file:data/payments.db`）或 MySQL；payment_products + delivery_orders + game_delivery_endpoints + order_projection + order_audit_logs + refund_requests + vendor_statements |
| promotions | Go / go-zero | 8902 | 9103 | GP-301 活动基础：活动状态机 draft→published（发布生成不可变版本快照 activity_versions.snapshot，运行时按 current_version 引用）→disabled→revise（回草稿再发布出新版本）；起止时间/时区、游戏/区服/渠道/人群、触发条件、限次/预算、叠加组、奖励配置（服务端配置域，不信任客户端可信金额）；管理面角色经 game-catalog /me 权威判定；操作审计（activity_audit_logs）；奖励/优惠的运行时计算归 GP-302/303/305，限次/预算运行时扣减归后续 | SQLite（默认 `file:data/promotions.db`）或 MySQL；activities + activity_versions + activity_audit_logs |
| api-gateway | Go / go-zero | 8800 | 9095 | BFF：自有聚合 handler + 六路反代（见 §3）+ 统一图片上传入口（见 §3.1） | uploads 卷（图片本地盘存储，选型方案 A） |
| user-service-rpc | Go / gRPC | 8080 (gRPC) | 9096 | 内网 gRPC 通道（手机/Web 均不直连；protoc 生成代码缺口保留原样） | — |

未落地（README 规划占位）：matchmaking、crawler-jobs、admin-console、mini-program（realtime-hub 已于 2026-10-11 立项落地）。

## 2. 客户端接入拓扑

两条接入路径并存，互不影响：

```
web-client (React/Vite, :5173)
  └─ 直连各服务（src/api/client.ts 硬编码 localhost:88xx）
       ├─ user-service :8888        （登录/注册/资料/推荐）
       ├─ game-catalog  :8890       （游戏列表/详情/精选）
       ├─ content-service :8891     （攻略/评论）
       └─ community :8892           （帖子/话题/关注/点赞/通知）

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
| GET /games/featured | 精选透传 | game-catalog |
| GET /users/:id/recommendations | 跨服务推荐（Bearer 透传） | user-service → game-catalog |
| GET /s/p/:id | 分享卡跳板页（html/template og meta + `spawn://` 深链按钮） | community |
| GET /home/feed | BFF 第二阶段列表聚合：精选游戏/热帖/话题/攻略四路并发，逐组字段裁剪（正文/图片等大字段不下发，帖子正文截 60 字摘要）；单上游故障降级为空组并记 `degraded`，不做整体 5xx | game-catalog + community + content-service |
| POST /upload | 统一图片上传入口（2026-10-08 方案 A 落地）：Bearer JWT 本地校验（HS256 与 user-service 同密钥）、512 字节魔数嗅探白名单 png/jpeg/webp/gif、10MB 上限、随机文件名 `O_EXCL` 落 uploads 卷，返回 `/uploads/<hash>.<ext>`；业务服务只存 URL 不碰文件 | 本地盘（uploads 卷） |
| GET /uploads/:file | 已上传图片静态托管：文件名正则白名单（`^[0-9a-f]{32}\.(png\|jpg\|webp\|gif)$`）防路径遍历后 `ServeContent` 回读 | 本地盘（uploads 卷） |

### 3.2 反代路由（internal/{users,games,community,content,datapanel,adapter,payments,promotions,realtime}；users/games/community/content/data-panel 为 proxy.NewUpstream passthrough，realtime 为 /ws 隧道）

| 前缀 | 路由 | 上游 |
|------|------|------|
| users | POST /auth/register、POST /auth/login、GET /users/:id（2026-10-11 起 login 亦走反代，信封原样透传） | user-service |
| games | GET /games、GET /games/:id（公开）、PATCH /games/:id/tags、GET /me、`admin/**`（角色/合作方/审核/归属/上下架/版本/审计）、`partner/**`（games/submit/versions） | game-catalog |
| community | `/api/v1/topics**`、`/api/v1/posts**`（含 /posts/search 与 :id/comments）、`/api/v1/users/{following,likes}`、`/api/v1/users/:user_id/follow`、`/api/v1/notifications**`、`/api/v1/messages**`、`/api/v1/moderation**` 等 M1–M3 全量调用面 | community |
| content | `/api/v1/guides**`、`/api/v1/comments**` | content-service |
| data-panel | `/api/v1/stats/users/:user_id/summary`、`/api/v1/stats/users/:user_id/games`、`/api/v1/stats/users/:user_id/games/:game_id`、POST `/api/v1/stats/records` | data-panel |
| adapter | POST /games/:id/session、POST /session/redeem、POST /games/:id/account/delete(+ /cancel)、GET/PUT /admin/games/:id/mapping、GET /admin/audit（GP-201；CourierAdapter.BaseURL 为空时不注册） | courier-adapter |
| payments | POST /games/:gameId/orders、GET /games/:gameId/orders/:orderId、GET /games/:gameId/orders、GET /games/:gameId/skus、POST /admin/products/:id、PATCH /admin/products/:id/enabled、GET /admin/games/:gameId/products、PUT/GET /admin/games/:gameId/delivery、GET /admin/deliveries、POST /admin/deliveries/:orderId/redeliver、PATCH /admin/deliveries/:orderId/target、GET /admin/orders、GET /admin/orders/:orderId、POST /admin/orders/:orderId/refund、GET /admin/refunds、GET /admin/audit、POST/GET /admin/statements、GET /admin/statements/:id、PUT /admin/statements/:id/settlement、POST /admin/statements/:id/confirm、GET /admin/statements/:id/export、POST /callback/:channel（GP-202/203/204/205；Payments.BaseURL 为空时不注册；`/internal/deliveries` 为 S2S 专用不进网关） | payments |
| promotions | POST /admin/activities、PUT /admin/activities/:id、POST /admin/activities/:id/{publish,disable,revise}、GET /admin/activities、GET /admin/activities/:id、GET /admin/activities/:id/versions、GET /admin/activity-audit（GP-301；Promotions.BaseURL 为空时不注册） | promotions |
| realtime | GET /ws（WebSocket 隧道：Hijack + 双向 splice，Hijack 前 502；RealtimeHub.BaseURL 为空时不注册） | realtime-hub |

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
- `docker compose up --build` 一键拉起全部服务；各业务服务目录有独立 Dockerfile
  （user-service-rpc 无镜像，按设计独立内网部署；api-gateway/realtime-hub 为
  CGO_ENABLED=0 构建——api-gateway 用 distroless、realtime-hub 用
  debian:bookworm-slim——其余 CGO + debian:bookworm-slim）。
- 配置：`etc/*.yaml`，上游地址支持环境变量覆盖（`USER_SERVICE_URL` 等）。

## 6. 演进路线（与 README「下一步建议」对齐）

1. BFF 第三阶段（按需）：更多聚合口径（列表聚合已落地 `/home/feed`，字段裁剪模式可复制）；必要时给聚合端点加缓存。
2. 真机回归：Android release 包已可本地 gradle 出包（`pnpm --dir apps/mobile-app build:android:release`），本地推送/深链分享卡等待真机验证。
3. data-panel 已落地（2026-10-09，M4 第一里程碑）：mobile 战绩 Tab + 单游戏详情（经网关 `/api/v1/stats/*` 四路反代）、web `/stats` 面板、compose/nightly/Prometheus 接线均同步落地；真机验证顺延。
4. realtime-hub：实时通道已经网关 `/ws` 隧道透传（客户端默认走网关，直连变量保留）；matchmaking 仍为规划占位。
5. 生产化：网关前置 TLS 终结（当前 compose 无证书）、明文 HTTP 收敛、CORS 收紧。

## 7. 源码分层分析

2026-11-07 的层、模块、状态存储、调用面、配置输入和生命周期清单见 [layer-analysis.md](layer-analysis.md)；术语定义见 [glossary.md](glossary.md)。
