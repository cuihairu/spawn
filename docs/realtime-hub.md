# realtime-hub 简档（2026-10-11 立项拍板）

> 状态：M1 开工。拍板记录见 `docs/backlog.md` 待办 #1；架构定位见 `docs/architecture/overview.md`。

## 目标

统一实时推送通道：站内通知即时送达（替代 web 端 30s 轮询的延迟与空转）、
后续承载在线状态 / 已读回执 / 私信等实时语义。移动端本地推送（expo）
不受影响；远程推送接桥列为远期项。

## 现状与约束

- **网关代理带不动 WS**：api-gateway 各 upstream 转发是自研缓冲式实现
  （读 body → 重建请求 → `http.Client` 往返，`internal/proxy/upstream.go`），
  无 Hijack，无法透传 WS upgrade。故 M1/M2 采用**客户端直连 hub**；
  网关 WS 隧道（ServerConn Hijack 转发）列为 M3 可选项。
- 端口：8893（hub WS/HTTP）；9091-9097 已被各服务 /metrics 占用，hub
  指标面 M1 不开。
- 鉴权：与各服务同款 HS256 JWT（共享 `JWT_SECRET`，claims 结构同
  `utils.JWTClaims`）。浏览器 WS 无法带自定义 header，握手 token 走
  query（本地部署口径，访问日志注意脱敏）。
- 单实例内存 registry 起步；水平扩展（Redis pub/sub）M3。

## 接口设计

| 端点 | 方法 | 说明 |
|------|------|------|
| `/health` | GET | 探活，200 |
| `/ws?token=<jwt>` | GET(Upgrade) | 握手校验 JWT，失败即断开；成功后进 registry（userId → 多连接） |
| `/internal/publish` | POST | 服务间投递。体 `{"user_ids":[..],"event":{"type":..,"payload":..}}`；头 `X-Internal-Token` 必须匹配配置（未配置=403 拒绝），投递失败不影响调用方主链路 |

- 帧格式：JSON 文本帧 `{"type":"notification","data":{...}}`；服务端
  ping/pong 保活（30s ping / 读超时 60s，pong 刷新）。
- 背压：每连接有界发送队列（16），溢出丢弃并计数记日志，发布方永不阻塞。
- 事件类型 M1 只定义 `notification`；后续类型增量注册，向后兼容。

## 分期

### M1 骨架（已完成，commit c8f978e）

`services/realtime-hub` 独立 Go 模块（module path
`github.com/tappi/tappi/services/realtime-hub`，Go 1.26，websocket 库
`github.com/gorilla/websocket`）：config / main / hub registry / ws
handler / publish handler；`go.work` 纳入（CI 按 go.work 动态发现，自动
覆盖 -race）；compose 加服务与 `JWT_SECRET`/`REALTIME_INTERNAL_TOKEN`
注入；服务 README。

### M2 接入（已完成，2026-10-11）

- community：通知创建（like/comment/reply/follow 四触发点）后
  fire-and-forget publish，写失败仅记日志（与通知写同款契约）。
- web：通知中心接 WS 实时 +1；断线自动重连（指数退避），降级回 30s
  轮询（保留现有轮询代码作为兜底，hub 不可达不损功能）。

#### M2 落地记录

- community：`internal/realtime` 发布器（Url 空 = no-op，3s 超时 goroutine，
  失败仅 logx），`Realtime` 配置段（`REALTIME_HUB_URL`/`REALTIME_INTERNAL_TOKEN`
  环境变量）；四触发点通知落库成功后推帧，落库失败不推（防幽灵红点）。
- web：`src/lib/realtime.ts`（指数退避重连 1s→30s 封顶，静默降级），App 铃铛
  帧到达即重拉未读数；VITE_REALTIME_HUB_URL 进 compose build args。
- 端到端冒烟（compose 实测）：注册 u1/u2 → u1 发帖 → u1 挂 WS → u2 点赞 →
  u1 收到 `{"type":"notification","data":{...like_post...}}` 帧；停 hub 后
  点赞仍 200，publish 失败仅 error 日志。

### M3 远期（逐项再拍板）

网关 WS 隧道、已读回执广播、Redis pub/sub 水平扩展、移动端远程推送
接桥、语音/组队信令（对应 overview.md 的 realtime-hub 愿景）。

### M3-1 在线状态（已完成，2026-10-11 拍板开工）

单实例内存档，不依赖网关隧道与 Redis。

设计口径：

- 在线定义：userId 活连接 ≥1；事件只在转换沿触发（首连/末断），
  多端上下线不重复刷帧。
- 三个面：REST 服务间查询 `GET /internal/presence?user_ids=1,2,3`
  （X-Internal-Token，fail closed 同 publish，ids 缺失/非法 400）；
  WS watch 推送（上行 `{"action":"watch|unwatch","user_ids":[..]}`，
  转换沿 → watcher 收 `{"type":"presence","data":{user_id,online,at}}`）；
  WS query 快照（上行 `{"action":"query","user_ids":[..]}`，只回请求
  连接——终端拿不到内部令牌，拉取走 WS 既有 JWT 闸）。
- 内存有界：单客户端 watch 上限 200（超限静默截断）、单次 ids 上限
  200；畸形上行静默忽略不断连；hub 不存社会关系（watch 谁由客户端
  声明，隐私过滤在产品层）。
- 无新配置、无新依赖；顺手修掉 SendToUsers 在 RLock 下 drops++
  的潜在竞态（改 atomic）。

落地记录：`hub` 增 since/watchers 表与 Watch/Unwatch/Presence/
replyPresence（转换沿事件锁外投递）；handler 增 presence 端点与
路由；新增 hub 6 测 + handler 2 测（含 A watch 7 → 7 连收 online、
REST 快照一致、畸形上行存活、7 断收 offline 端到端）；顺手修掉
既有 fanout 测试的入表时序竞态（Dial 返回 ≠ 服务端 Join 已执行，
publish 前轮询连接数）。

## 验收口径

- **M1**：`go test ./... -race` 绿；集成——两客户端同连、publish 后各收
  一帧、无效 JWT 拒断、publish 缺/错 token 403；`docker compose build
  realtime-hub && up` 后 `/health` 200；CI 全绿。
- **M2**：web 铃铛收到实时 +1（端到端手动口径 + 集成测试）；停 hub 后
  web 自动降级轮询不断功能；web 三连门禁绿。
- **M3**：逐项立项时再定。

## 铁律沿用

非交互；禁 tag/release、禁 force push；push 前 fetch+rebase origin/main；
每增量测试全绿才提交推送；commit 英文短句；只 add 自己改的文件；Go
module path `github.com/tappi/tappi` 永不改；只推 main。
