# api-gateway

统一 API 入口：聚合路由反代各业务服务（缓冲转发）、本地盘统一上传、realtime-hub `/ws` 原始字节隧道。go-zero rest 服务，监听 `:8800`，Prometheus 指标 `:9095/metrics`。

## 路由面

| 类别 | 路径 | 去向 |
| --- | --- | --- |
| 聚合（本服务 handler） | `GET /games/featured`、`GET /users/:id/recommendations`、`GET /s/p/:id`（短链跳转）、`GET /home/feed` | 缓存 + 聚合多 upstream |
| 上传 | `POST /upload`（JWT 本地校验，≤10MB 图片落盘 `./uploads`）、`GET /uploads/:file` | 本服务静态回放 |
| 游戏反代 | `GET /games`、`GET /games/:id` | game-catalog `:8890` |
| 战绩反代 | `/api/v1/stats/*`（summary/games/records） | data-panel `:8896` |
| 内容反代 | `/api/v1/guides*`（CRUD/收藏/发布/点赞）、`/api/v1/comments*` | content-service `:8891` |
| 社区反代 | `/api/v1/posts|topics|users|notifications` 全组 | community `:8892` |
| WS 隧道 | `GET /ws?token=<jwt>` | realtime-hub `:8893` |

- 反代为缓冲式（读 body → 重建请求 → `http.Client` 往返，`internal/proxy/upstream.go`），透传 `Authorization`；upstream BaseURL 未配置则该组路由不注册（服务可选部署）。
- **WS 隧道不同**：Hijack 客户端连接后只重写请求行/Host，其余头原样透传（`Connection/Upgrade/Sec-WebSocket-*` 即握手本体，不可剥离），补 `X-Forwarded-For`/`X-Real-Ip`，双向字节 splice 不解析；hub 不可达时 Hijack 前回 502（`internal/realtime/tunnel.go`）。JWT 校验仍在 hub，网关只管透传。
- 聚合/上传细节见 `internal/handler/routes.go` 与 `internal/upload/upload.go`。

## 配置（`etc/gateway-api.yaml`，环境变量可覆盖）

| 键 | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| Listen | - | `0.0.0.0:8800` | 监听地址 |
| Auth.JWTSecret | `JWT_SECRET` | 必填 | 与 user-service 同值（上传入口本地校验 JWT） |
| Upload.Dir / MaxBytes | - | `./uploads` / 10485760 | 上传落盘目录与大小上限 |
| Upstreams.*.BaseURL | `USER_SERVICE_URL` / `GAME_SERVICE_URL` / `CONTENT_SERVICE_URL` / `COMMUNITY_SERVICE_URL` / `DATA_PANEL_URL` / `REALTIME_HUB_URL` | 见 yaml | 各 upstream 地址；除 UserService 必填外其余未配置即不注册对应路由（RealtimeHub 空 = 不注册 `/ws`） |
| Upstreams.*.Timeout | - | 5000ms | upstream 超时 |

## 运行

```bash
go run gateway.go -f etc/gateway-api.yaml
```

compose 部署见根目录 `docker-compose.yaml`（gateway 服务，注入各 upstream 环境变量与 JWT_SECRET）。

## 测试

```bash
go test ./... -race
```

覆盖：各反代组 stub upstream 透传（`internal/*/routes_integration_test.go`）、上传校验与落盘（`internal/upload`）、WS 隧道端到端（假 hub echo + go-zero 全量中间件链 + 裸 writer 两路、hub 不可达 502、上游拒绝透传、X-Forwarded-For 到达 hub，`internal/realtime`）。
