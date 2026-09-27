# Content Service 增强功能说明

## 概述

本次更新为 `content-service` 实现了两个核心功能:

1. **JWT 用户认证集成** - 从 JWT token 中获取用户信息
2. **跨服务调用** - 与 game-catalog 服务集成获取游戏详情

## 1. JWT 用户认证集成

### 新增文件

#### `services/content-service/utils/auth.go`
JWT 令牌解析工具，实现以下功能:
- `ParseToken()` - 解析和验证 JWT 令牌
- `ValidateToken()` - 验证令牌有效性
- `GetUserIdFromToken()` - 从令牌中提取用户ID

#### `services/content-service/middleware/auth.go`
HTTP 认证中间件，实现:
- Bearer Token 提取和验证
- 用户信息注入到请求上下文
- 公开路径跳过认证（GET 接口）

### 配置变更

**`internal/config/config.go`**
```go
Auth struct {
    JWTSecret string `json:",env=JWT_SECRET"`
}
```

**`etc/content-api.yaml`**
```yaml
Auth:
  JWTSecret: "your-secret-key-here-change-in-production"
```

### 业务逻辑更新

**`internal/logic/createguidelogic.go`**
- 从上下文获取 `user_id` 和 `username`
- 使用真实用户信息替代硬编码值
- 添加认证失败处理

**`internal/logic/createcommentlogic.go`**
- 同上，实现用户认证

### 使用示例

```bash
# 创建攻略（需要认证）
curl -X POST http://localhost:8891/api/v1/guides \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "game_id": "game-001",
    "title": "新手入门指南",
    "content": "这是攻略内容..."
  }'
```

## 2. 跨服务调用集成

### 新增文件

#### `services/content-service/client/gamecatalog.go`
游戏目录服务 HTTP 客户端，实现:
- `NewGameCatalogClient()` - 创建客户端实例
- `GetGameById()` - 根据游戏ID获取游戏详情
- 超时控制和错误处理

### 配置变更

**`internal/config/config.go`**
```go
Services struct {
    GameCatalog struct {
        BaseURL string `json:",default=http://localhost:8890,env=GAMECATALOG_BASE_URL"`
        Timeout int64  `json:",default=5000,env=GAMECATALOG_TIMEOUT"` // 毫秒
    }
}
```

**`etc/content-api.yaml`**
```yaml
Services:
  GameCatalog:
    BaseURL: "http://localhost:8890"
    Timeout: 5000
```

### 服务上下文更新

**`internal/svc/servicecontext.go`**
- 初始化 `GameCatalogClient`
- 注入到服务上下文供业务逻辑使用

### 业务逻辑更新

**`internal/logic/createguidelogic.go`**
```go
// 从 game-catalog 服务获取游戏名称
gameTitle := req.GameId // 默认使用 gameId
gameInfo, err := l.svcCtx.GameCatalogClient.GetGameById(l.ctx, req.GameId)
if err != nil {
    l.Logger.Errorw("获取游戏信息失败，使用游戏ID作为标题",
        logx.Field("gameId", req.GameId),
        logx.Field("error", err),
    )
} else {
    gameTitle = gameInfo.Title
}
```

- 调用 game-catalog 服务获取真实游戏标题
- 失败时降级使用 gameId 作为标题
- 不影响主流程（容错处理）

## 架构改进

### 认证流程

```
客户端请求
    ↓
[AuthMiddleware] 提取并验证 JWT
    ↓
将用户信息注入上下文 (user_id, username)
    ↓
[业务逻辑] 从上下文获取用户信息
    ↓
返回响应
```

### 服务调用流程

```
[CreateGuide] 创建攻略请求
    ↓
[GameCatalogClient] 调用 GET /games/:id
    ↓
获取游戏信息（标题、封面等）
    ↓
使用真实游戏标题创建攻略
```

## 安全性提升

1. **JWT 认证** - 所有写操作必须提供有效令牌
2. **用户追踪** - 记录真实的用户ID和用户名
3. **令牌验证** - 检查令牌签名、过期时间
4. **上下文隔离** - 每个请求的用户信息独立

## 可靠性提升

1. **超时控制** - HTTP 客户端设置超时（默认5秒）
2. **错误降级** - 获取游戏信息失败不影响主流程
3. **日志记录** - 详细记录跨服务调用失败原因
4. **类型安全** - 强类型的服务客户端

## 环境配置

### 开发环境

```yaml
# etc/content-api.yaml
Auth:
  JWTSecret: "dev-secret-key"

Services:
  GameCatalog:
    BaseURL: "http://localhost:8890"
    Timeout: 5000
```

### 生产环境（使用环境变量）

```bash
export JWT_SECRET="production-secret-key-very-long-and-secure"
export GAMECATALOG_BASE_URL="http://game-catalog-service:8890"
export GAMECATALOG_TIMEOUT=3000
```

## 测试建议

### 1. JWT 认证测试

```bash
# 先登录获取 token
TOKEN=$(curl -X POST http://localhost:8888/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"password"}' \
  | jq -r '.token')

# 使用 token 创建攻略
curl -X POST http://localhost:8891/api/v1/guides \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "game_id": "the-last-of-us-2",
    "title": "生存技巧分享",
    "content": "攻略内容..."
  }'
```

### 2. 跨服务调用测试

```bash
# 确保 game-catalog 服务运行
curl http://localhost:8890/games/the-last-of-us-2

# 创建攻略时会自动获取游戏名称
# 检查返回的 game_title 字段
```

## 依赖项

新增 Go 依赖:
- `github.com/golang-jwt/jwt/v5` - JWT 令牌处理

已通过 `go mod tidy` 自动安装。

## 兼容性

- ✅ 向后兼容：不影响现有 API 接口
- ✅ 可选降级：跨服务调用失败时使用 gameId
- ✅ 配置驱动：通过配置文件/环境变量控制行为

## 下一步建议

1. ~~添加单元测试覆盖认证和跨服务调用逻辑~~ ✅ 已完成：
   - `utils/auth_test.go` 覆盖 `ParseToken`/`ValidateToken`/`GetUserIdFromToken`（有效令牌、过期、密钥错误、非 HMAC 签名方法、畸形令牌、密钥隔离）。
   - `client/gamecatalog_test.go` 覆盖 `GetGameById`（裸 `{"game":...}` 响应、旧版 `{code,data}` 包装响应、非 200 状态码、业务错误码、非法 JSON、无法识别的响应体、服务不可达、上下文取消、baseURL 尾斜杠归一化）。
   - 同时修复客户端与 game-catalog 实际契约不一致的问题：请求路径由 `/api/v1/games/:id` 改为 game-catalog 实际路由 `/games/:id`，并兼容裸 `{"game": {...}}` 响应格式（此前跨服务调用始终失败并降级为 gameId）。
2. ~~实现服务熔断和重试机制~~ ✅ 已完成：
   - `client/breaker.go`：基于连续失败次数的熔断器（closed → open → half-open → closed/open），`Allow/Success/Failure` 状态机，可注入时钟。
   - `client/retry.go`：错误分类（`kindTransient` 网络错误/5xx/429/业务错误码 → 重试+计入熔断；`kindBadRequest` 4xx 与 `kindCanceled` 上下文取消 → 不重试不计入；`kindContract` 解析失败/无法识别响应 → 不重试但计入熔断）与确定性指数退避 `base×2^(n-1)` 封顶 `maxDelay`（无抖动，便于测试）。
   - `GetGameById`：瞬时错误按指数退避重试（默认 3 次尝试、100ms→1s），熔断开启时快速失败（`ErrBreakerOpen`，默认连续失败 3 次、冷却 5s）；失败降级（调用方用 gameId 作标题）语义不变。
   - 测试：`breaker_test.go` 覆盖 open/半开探测成功恢复/探测失败重开/单探测占用/连续失败复位/非法配置钳制；`gamecatalog_test.go` 覆盖重试退避序列与封顶、重试耗尽、4xx/取消/契约异常不重试、熔断打开后快速失败与半开恢复（成功/失败/4xx 探测）。
3. ~~添加 Prometheus 指标监控跨服务调用~~ ✅ 已完成：
   - `client/metrics.go`：`content_service_gamecatalog_client_requests_total{result}`（success / breaker_open / transient_error / bad_request / canceled / contract_error）、`retries_total`、`request_duration_seconds`（含重试与退避）、`breaker_state` gauge（0=closed 1=half-open 2=open，经熔断器 `onChange` 回调上报）。
   - 指标注册到默认 registry，随 go-zero Prometheus agent 的 `/metrics` 暴露（`etc/content-api.yaml` 中 `Prometheus.Port: 9093`）；同时为全部 6 个服务补充 `Prometheus` 配置段（端口见 `docs/development-guide.md` 监控章节），go-zero 内置 `http_server_requests_*` / `rpc_server_requests_*` 指标自动生效。
   - 测试（`client/metrics_test.go`，独立 registry 隔离并行用例）：默认 registry 注册断言、按 result 分类的调用计数 delta、熔断打开快速失败计数、重试次数与直方图 SampleCount 采集、熔断状态 gauge 迁移序列（closed→open→half-open→closed）、promhttp 文本格式渲染断言。
4. 考虑实现 gRPC 调用替代 HTTP 以提升性能
5. ~~实现用户权限控制（只能修改/删除自己的攻略）~~ ✅ 已完成（见 `internal/logic/permissions_test.go`）
