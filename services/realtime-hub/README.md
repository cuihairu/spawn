# realtime-hub

WebSocket 实时通道基座：JWT 鉴权长连接 + 内部发布接口，把 community 等服务的通知从 30s 轮询升级为即时推送。单实例内存注册表；网关 `/ws` 隧道已落地（M3-2），Redis pub/sub 跨实例中继已落地。设计与分期详见 [`docs/realtime-hub.zh.md`](../../docs/realtime-hub.zh.md)。

## 接口

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/health` | 无 | 探活 |
| GET | `/ws?token=<jwt>` | JWT（user-service 签发，HS256） | 客户端长连接；收帧 `{"type":"notification","data":{...}}` |
| POST | `/internal/publish` | `X-Internal-Token` | 服务间投递；body `{"user_ids":[..],"event":{"type":"..","payload":{..}}}`，返回 `{"delivered":n}` |
| GET | `/internal/presence?user_ids=1,2,3` | `X-Internal-Token` | 服务间在线查询；返回 `{"presence":[{user_id,online,connections,since}]}`（与入参同序，重复去重，最多 200 个） |

约束：发布事件 type 非空、user_ids 非空；每连接发送队列 16，队满即丢帧计数（不阻塞不慢速放大）；30s 服务端 ping / 60s 读超时，pong 续期；`/internal/publish` 未配置内部令牌时一律 403（fail closed）。

客户端上行控制消息（JSON，非这四类静默忽略，连接不断）：
`{"action":"watch","user_ids":[..]}` / `{"action":"unwatch",..}`——被 watch 用户上线（首连）/下线（末断）时收到 `{"type":"presence","data":{"user_id":..,"online":..,"at":..}}` 帧；`{"action":"query","user_ids":[..]}`——服务端向本连接回一帧 `{"type":"presence","data":{"items":[..]}}` 快照；`{"action":"signal","to":<用户id>,"room":"..","data":{..}}`——WebRTC 信令（offer/answer/ICE）原样转发给目标用户全部连接（发端连接除外），下行 `{"type":"signal","data":{"from":..,"room":..,"data":..}}`，目标不在线静默丢弃（重试归端上协商层），hub 不存房间状态。单客户端 watch 集合上限 200，超限截断；上行单条消息上限 4096 字节（即信令尺寸上限）。

## 配置（`etc/realtime-hub.yaml`，环境变量可覆盖）

| 键 | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| Listen | - | `:8893` | 监听地址 |
| JwtSecret | `JWT_SECRET` | 必填 | 与 user-service 同值 |
| InternalToken | `REALTIME_INTERNAL_TOKEN` | 必填 | 内部发布令牌，空则发布接口全 403 |
| WriteTimeoutMs / PingIntervalMs / SendQueueSize | - | 10000 / 30000 / 16 | 连接保稳参数 |
| RedisAddr / RedisPassword | `REALTIME_REDIS_ADDR` / `REALTIME_REDIS_PASSWORD` | 空 | Redis pub/sub 跨实例投递；空 = 单实例内存模式 |
| PushWebhook / PushToken | `REALTIME_PUSH_WEBHOOK` / `REALTIME_PUSH_TOKEN` | 空 | 离线推送 webhook（厂商推送桥接缝）：单实例模式下零投递用户 POST `{"user_ids":[..],"frame":{..}}`（`X-Push-Token`），多实例（Redis）模式下不推 |

## 运行

```bash
go run .                        # 本地：读 etc/realtime-hub.yaml
docker compose up -d realtime-hub
```

## 分期

- [x] M1 骨架：模块 + 配置/鉴权/hub/handler + 全量单测（-race）+ Dockerfile + compose
- [x] M2 接入：community 四触发点 fire-and-forget 发布；web WS 消费（断线退避重连，30s 轮询兜底保留）
- [x] M3-1 在线状态：REST 查询 + watch 推送 + query 快照（单实例内存，2026-10-11）
- [x] M3-2 网关 WS 隧道：`/ws` 原始字节透传（网关侧，2026-10-12，见 api-gateway）
- [x] M3-3 已读回执：mark-all-read 成功后广播 `notification.read`（community 侧，2026-10-12）
- [x] M3-4 Redis 扩展：`RedisAddr` 配置即启用 pub/sub 跨实例投递（尽力而为，回环剔除，2026-10-12）
- [x] M3-5 推送桥：`PushWebhook` 离线推送接缝（厂商协议由桥接服务接，2026-10-12）
- [x] M3-6 语音信令：`signal` 上行无状态转发（2026-10-12）

## 测试

```bash
go test ./... -race -count=1
```

覆盖：配置加载/环境变量覆盖、JWT 四类拒绝（错钥/过期/非 HMAC/垃圾）、hub 注册/出表/扇出/队满丢弃/并发竞态、Redis 跨实例投递（miniredis：跨实例送达/回环剔除/坏地址 fail fast）、离线推送（在线不推/离线去重推送/Relay 模式不推）、信令（用户间转发/多端自信号/非法目标忽略/跨实例兜底）、handler 端到端（WS 拒未授权升级、跨源放行、发布扇出多端多用户、信令端到端、守卫矩阵）。
