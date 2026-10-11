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

### M1 骨架（当前）

`services/realtime-hub` 独立 Go 模块（module path
`github.com/tappi/tappi/services/realtime-hub`，Go 1.26，websocket 库
`github.com/gorilla/websocket`）：config / main / hub registry / ws
handler / publish handler；`go.work` 纳入（CI 按 go.work 动态发现，自动
覆盖 -race）；compose 加服务与 `JWT_SECRET`/`REALTIME_INTERNAL_TOKEN`
注入；服务 README。

### M2 接入

- community：通知创建（like/comment/reply/follow 四触发点）后
  fire-and-forget publish，写失败仅记日志（与通知写同款契约）。
- web：通知中心接 WS 实时 +1；断线自动重连（指数退避），降级回 30s
  轮询（保留现有轮询代码作为兜底，hub 不可达不损功能）。

### M3 远期（逐项再拍板）

网关 WS 隧道、在线状态与已读回执广播、Redis pub/sub 水平扩展、移动端
远程推送接桥、语音/组队信令（对应 overview.md 的 realtime-hub 愿景）。

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
