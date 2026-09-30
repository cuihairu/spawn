# Tappi 开发指南

## 项目概述

Tappi 是一个基于 Go-zero 框架的游戏社交平台 monorepo 项目，采用微服务架构，支持多端应用开发。

## 开发环境准备

### 1. 工具安装

```bash
# 安装 Go (建议 1.25+)
brew install go

# 安装 goctl 工具
go install github.com/zeromicro/go-zero/tools/goctl@latest

# 安装 protobuf 工具
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 将 GOPATH/bin 添加到 PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

### 2. 项目初始化

```bash
# 克隆项目
git clone git@github.com:cuihairu/tappi.git
cd tappi

# 验证 Go workspace
go work sync

# 安装依赖
go work download
```

## 项目结构

```
tappi/
├── apps/                 # 面向最终用户的应用
├── services/             # 微服务
│   ├── user-service      # 用户服务 API
│   └── user-service-rpc  # 用户服务 RPC
├── packages/             # 共享包
├── platform/             # 基础设施
├── tools/                # 开发工具
└── docs/                 # 文档
```

## 开发工作流

### 1. 创建新服务

```bash
# 创建 API 服务
cd services
goctl api new my-service

# 创建 RPC 服务
goctl rpc new my-service-rpc

# 更新 go.work
cd ..
go work use ./services/my-service ./services/my-service-rpc
```

### 2. 开发 API

1. **修改 API 定义** (`my-service.api`)
```api
syntax = "v1"

type UserRequest {
    UserId int64 `path:"userId"`
}

type UserResponse {
    UserId   int64  `json:"userId"`
    Username string `json:"username"`
    Email    string `json:"email"`
}

service my-service {
    @handler UserHandler
    get /users/:userId (UserRequest) returns (UserResponse)

    @handler CreateUserHandler
    post /users (CreateUserRequest) returns (UserResponse)
}
```

2. **生成代码**
```bash
cd my-service
goctl api go -api my-service.api -dir .
```

3. **实现业务逻辑** (`internal/logic/userlogic.go`)
```go
func (l *UserLogic) User(req *types.UserRequest) (resp *types.UserResponse, err error) {
    // 从数据库或其他服务获取用户信息
    user, err := l.svcCtx.UserModel.FindOne(l.ctx, req.UserId)
    if err != nil {
        return nil, err
    }

    return &types.UserResponse{
        UserId:   user.Id,
        Username: user.Username,
        Email:    user.Email,
    }, nil
}
```

### 3. 开发 RPC 服务

1. **定义 Proto 文件** (`my-service.proto`)
```protobuf
syntax = "proto3";

package myservice;

option go_package = "./myservice";

message UserRequest {
    int64 user_id = 1;
}

message UserResponse {
    int64 user_id = 1;
    string username = 2;
    string email = 3;
}

service MyService {
    rpc GetUser(UserRequest) returns (UserResponse);
}
```

2. **生成代码**
```bash
cd my-service-rpc
goctl rpc protoc my-service.proto --go_out=. --go-grpc_out=. --zrpc_out=.
```

3. **实现服务逻辑** (`internal/logic/userlogic.go`)
```go
func (l *GetUserLogic) GetUser(in *myservice.UserRequest) (*myservice.UserResponse, error) {
    // 实现获取用户逻辑
    user, err := l.svcCtx.UserModel.FindOne(l.ctx, in.UserId)
    if err != nil {
        return nil, err
    }

    return &myservice.UserResponse{
        UserId:   user.Id,
        Username: user.Username,
        Email:    user.Email,
    }, nil
}
```

## 服务通信

### 1. 在 API 服务中调用 RPC 服务

1. **配置 RPC 客户端** (`etc/my-service.yaml`)
```yaml
Name: my-service
Host: 0.0.0.0
Port: 8888
MyServiceRpc:
  Etcd:
    Hosts:
      - etcd:2379
    Key: my-service-rpc
```

2. **更新 ServiceContext** (`internal/svc/servicecontext.go`)
```go
type ServiceContext struct {
    Config         config.Config
    MyServiceRpc   myserclient.MyService
}

func NewServiceContext(c config.Config) *ServiceContext {
    return &ServiceContext{
        Config:       c,
        MyServiceRpc: myserclient.NewMyService(zrpc.MustNewClient(c.MyServiceRpc)),
    }
}
```

3. **在 Logic 中调用 RPC**
```go
func (l *UserLogic) User(req *types.UserRequest) (resp *types.UserResponse, err error) {
    rpcResp, err := l.svcCtx.MyServiceRpc.GetUser(l.ctx, &myservice.UserRequest{
        UserId: req.UserId,
    })
    if err != nil {
        return nil, err
    }

    return &types.UserResponse{
        UserId:   rpcResp.UserId,
        Username: rpcResp.Username,
        Email:    rpcResp.Email,
    }, nil
}
```

## 本地开发

### 1. 启动依赖服务

```bash
# 启动 etcd (用于服务发现)
docker run -d --name etcd \
  -e ALLOW_NONE_AUTHENTICATION=yes \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.0

# 启动 Redis (用于缓存)
docker run -d --name redis -p 6379:6379 redis:7-alpine

# 启动 MySQL (用于持久化)
docker run -d --name mysql \
  -e MYSQL_ROOT_PASSWORD=root123 \
  -p 3306:3306 \
  mysql:8.0
```

### 2. 启动服务

```bash
# 启动 RPC 服务
cd services/user-service-rpc
go run user.go -f etc/user.yaml

# 启动 API 服务
cd ../user-service
go run user.go -f etc/user-api.yaml
```

### 3. 测试 API

```bash
# 测试用户服务
curl http://localhost:8888/from/you
# 返回: {"message": "Hello, you!"}

curl http://localhost:8888/from/me
# 返回: {"message": "Hello, me!"}
```

## 配置管理

### 1. 环境变量

服务配置支持环境变量替换：

```yaml
Name: user-api
Host: ${HOST:0.0.0.0}
Port: ${PORT:8888}
```

### 2. 配置文件位置

- API 服务: `etc/{service-name}.yaml`
- RPC 服务: `etc/{service-name}.yaml`
- 配置结构: `internal/config/config.go`

## 数据库集成

### 1. 添加 MySQL 依赖

```bash
go get github.com/go-sql-driver/mysql
```

### 2. 配置数据库连接

```yaml
# etc/user-api.yaml
DataSource: ${MYSQL_USER:root}:${MYSQL_PASSWORD:root123}@tcp(${MYSQL_HOST:localhost}:3306)/tappi?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai
```

### 3. 使用 Model Cache

```go
// 自动生成缓存模型
goctl model mysql datasource -url="user:pass@tcp(localhost:3306)/database" -table="user" -dir="./model"

// 在 Logic 中使用
func (l *UserLogic) GetUser(req *types.UserRequest) (*types.UserResponse, error) {
    user, err := l.svcCtx.UserModel.FindOne(l.ctx, req.UserId)
    if err != nil {
        return nil, err
    }

    return &types.UserResponse{
        UserId:   user.Id,
        Username: user.Name,
    }, nil
}
```

> 注：上例 goctl model cache / Redis 流程为**未来可选方案**，当前未采用
> （原因见下文设计决策第 3 条）。

### 4. 设计决策：数据库模型与缓存（「下一步」第 2 项）

> 2026-09-30 拍板并落档。本项为多轮工作，以下决策覆盖全部后续切片；
> 首个切片（user-service 模型层缓存）已落地，交付物见第 3 条。

#### 1）落地服务与顺序

| 顺序 | 服务 | 理由 | 状态 |
|------|------|------|------|
| 1 | user-service | 模型层已有 MySQL/SQLite 双驱动（本节先例），只缺缓存；登录/注册是全仓最热 DB 路径，缓存收益直接 | ✅ 本切片已落地 |
| 2 | game-catalog | 现为 JSON 文件仓库，按 user-service 模式迁 MySQL；其读路径已在内存（无 DB I/O），缓存收益低于 user-service，故排第二 | 待做 |
| 3 | community、content-service | 同为文件仓库，按 game-catalog 跑通的模式跟进 | 待做 |

- user-service-rpc 为 goctl 生成层，不手改、不纳入切片。

#### 2）MySQL DSN 与部署形态（沿用双驱动先例）

- **DSN 注入**：`MySQL.DataSource`（env `DATASOURCE`），标准形态见上文
  「配置数据库连接」：`user:pass@tcp(host:3306)/tappi?charset=utf8mb4&parseTime=true&...`
- **驱动选择**（`svc.NewServiceContext` 判定）：DSN 含 `file:` 或 `.db` → SQLite；
  否则 → MySQL。启动时 `Ping` 失败即 panic（快速失败，启动期暴露配置问题）。
- **部署形态**（假设注明：仓库当前无 K8s 集群/镜像仓库凭据，与上文
  「CI 流水线」一节结论一致）：
  - 开发：本页「启动依赖服务」的 docker mysql（8.0）；
  - CI / 单测：SQLite（临时文件或 `:memory:`），门禁零外部依赖；
  - 生产：环境变量注入的外置 MySQL 8 实例（单实例假定；
    多实例化前需先落跨进程缓存失效方案，见第 3 条）。

#### 3）缓存选型：进程内 TTL+LRU（本切片），Redis 留作演进

- **选型**：进程内泛型缓存 `services/user-service/internal/cache`（TTL + LRU，
  可注入时钟，`ttl<=0` 即禁用），接入 `model.UserModel`：
  - 读：`FindOne` / `FindByUsername` / `FindByEmail` 命中返回值副本（无别名共享）；
  - 写：`Create` / `Update` 走统一失效点；`Update` 先读行内旧 username/email
    （调用方常只传 Id+Email 部分结构，旧键须从行内容推导）；
  - 不做负缓存（未找到/错误不写入）；`CheckXxxExists` 的 COUNT 恒直查数据库；
  - 兜底：任何失效遗漏由 60s TTL 硬界收敛（陈旧 ≤60s）。
- **不选 go-zero sqlc / goctl model cache + Redis 的原因**：
  1. 本仓模型为手写 `database/sql`，全仓无 sqlc 代码生成，引入即等于重写全部模型；
  2. CI 无 Redis 服务，新增外部依赖会破坏「门禁零外部依赖」的现状；
  3. 当前单实例部署，进程内缓存语义正确（无跨进程失效问题）。
- **演进触发条件**：实例数 >1、或出现跨进程失效/共享缓存需求时，将
  `internal/cache` 实现替换为 go-zero `cache.Cache`（Redis，接入点不变、
  模型层代码零改动）；届时启用本页「启动依赖服务」的 Redis 容器，
  上文 goctl model cache 流程按需采用。
- **首个切片交付物**：
  - `services/user-service/internal/cache/ttlcache.go` —— 泛型 TTL+LRU 缓存；
  - `services/user-service/model/usermodel.go` —— 读缓存与失效点接入；
  - 单测 `internal/cache/ttlcache_test.go`（命中/过期/LRU 淘汰/禁用/并发）；
  - 集成测试 `model/usercache_test.go`（关库命中、更新失效、无负缓存、值副本隔离）。

### 5. 覆盖率台账刷新与 user-service-rpc 测试落地（2026-09-30）

全仓覆盖率快照（`services/cov_*.out`，本地不入库；生成命令：
`cd services/<模块> && go test ./... -covermode=atomic -coverpkg=./... -coverprofile=...`
后按块聚合去重，可信总数以 `go tool cover -func` 为准）：

| 模块 | 覆盖率 | 备注 |
|------|-------|------|
| api-gateway | 96.3% | |
| user-service | 97.2% | 重测确认（旧快照 15.8% 系滞后） |
| game-catalog | 95.7% | |
| community | 91.5% | |
| content-service | 88.8% | |
| user-service-rpc | 79.2%（含生成代码）/ **91.7%（手写面）** | 见下 |

**user-service-rpc 首轮测试落地**：`user.go` main→run 重构（与五个兄弟服务一致，
配置加载失败返回错误而非 panic）；新增 logic/server/svc 单测 + main 集成测试
（`run()` 起真 zRPC 服务，`userclient` 经 gRPC 端到端 Ping，`Mode: test` 覆盖
reflection 注册分支）。手写代码除 `main()`（stderr+exit，全仓既定不可达约定）外
100% 覆盖。模块口径 79.2% 的缺口全部来自 `user/` 包 protoc 生成管道
（getter/Descriptor/GZIP/Unimplemented 守卫，`DO NOT EDIT`）——按「不硬造不可达
路径用例」的既定约定不做覆盖，同其早前被排除出覆盖率战役的理由一致。

**user-service logic 层 DB 错误分支收口（同日第二轮）**：
`registerlogic.go:73`（邮箱存在性检查报错）已补——用缺 `email` 列的 users 表触达
（CheckUsernameExists 按 username 查询正常，CheckEmailExists 报
no such column: email；闭库会让先执行的用户名检查先失败，够不到此分支），
见 `internal/logic/register_dberror_test.go`，96.5% → 96.8%。
其余候选经逐行论证确属不可达，按台账口径登记：
- `registerlogic.go:89`（HashPassword）：bcrypt 唯一错误 ErrPasswordTooLong
  （>72 字节），而 ValidatePassword 以 len() 字节计上限 50 < 72，不可达；
- `loginlogic.go:57`（GenerateToken）：HS256 生成恒成功（既定 ledger）；
- `updateuserinfologic.go:85`（更新后二次 FindOne 失败）：关库/只读库故障会先在
  Update 处返回（同一故障对称命中写、读两步），单请求内无法注入
  「写成功、随后读失败」；
- `updateuserinfologic.go:97`（昵称回退 else）：logic 的 if/else 两臂均构造
  Valid=true 的 Nickname，更新后的行 nickname 不可能为 NULL；
- `model/usermodel.go:77`（LastInsertId）：mattn/go-sqlite3 的
  SQLiteResult.LastInsertId 恒返回 (id, nil)，无错误路径。
**巡检第三轮（同日）**：原沿袭登记的 svc 两个 panic 分支复核后实际可达，已补测
（`internal/svc/servicecontext_test.go`）——
`servicecontext.go:44`：畸形 MySQL DSN 使 sql.Open 阶段 ParseDSN 即失败
（如 `user:pw@tcp(127.0.0.1:3306)x`，tcp 地址后缺 "/dbname" 分隔）；
`servicecontext.go:63`：只读库（`file:...?mode=ro`）且 users 表不存在时
Ping 成功、CreateUsersTable 的 SQLite/MySQL 双格式建表均失败。
96.8% → 97.2%。handler `httpx.ErrorCtx` 空错分支 ×5 经逐一核对：对应 logic
（register/update/getuserinfo/getuserrecommendations/user）全部零非 nil error
返回（envelope 恒 `}, nil`；login 的 logic 会返回真实错误且其 handler 分支已覆盖），
维持 ledger。
剩余未覆盖 13 块 = 已论证 5 块（register:89、login:57、update:85/97、usermodel:77）
+ main（2）、handler ×5、utils HS256 GenerateToken 错误（1）。

## 部署

### 1. Docker 构建

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go work download
RUN CGO_ENABLED=0 GOOS=linux go build -o service services/user-service/user.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/service .
COPY --from=builder /app/services/user-service/etc ./etc

EXPOSE 8888
CMD ["./service", "-f", "etc/user-api.yaml"]
```

### 2. Kubernetes 部署

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: user-service
  template:
    metadata:
      labels:
        app: user-service
    spec:
      containers:
      - name: user-service
        image: tappi/user-service:latest
        ports:
        - containerPort: 8888
        env:
        - name: ETCD_ENDPOINTS
          value: "etcd:2379"
---
apiVersion: v1
kind: Service
metadata:
  name: user-service
spec:
  selector:
    app: user-service
  ports:
  - port: 8888
    targetPort: 8888
  type: ClusterIP
```

## 最佳实践

### 1. 错误处理

```go
// 使用 go-zero 的错误处理
if err != nil {
    logx.Errorf("处理用户请求失败: %v", err)
    return nil, httpx.NewApiError(http.StatusBadRequest, "参数错误")
}
```

### 2. 日志记录

```go
// 结构化日志
logx.Infow("处理用户请求",
    logx.Field("userId", req.UserId),
    logx.Field("action", "getUser"),
)
```

### 3. 限流和熔断

```yaml
# 配置文件中添加
Name: user-api
Host: 0.0.0.0
Port: 8888
RateLimit: 1000  # 每秒 1000 次请求
Prometheus:
  Host: 0.0.0.0
  Port: 9091
```

### 4. 单元测试

```go
func TestUserLogic_User(t *testing.T) {
    tests := []struct {
        name    string
        req     *types.UserRequest
        want    *types.UserResponse
        wantErr bool
    }{
        {
            name: "获取用户信息成功",
            req:  &types.UserRequest{UserId: 1},
            want: &types.UserResponse{
                UserId:   1,
                Username: "test",
            },
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            l := &UserLogic{}
            got, err := l.User(tt.req)
            if (err != nil) != tt.wantErr {
                t.Errorf("User() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("User() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## 监控与指标（Prometheus）

所有 go-zero 服务通过配置文件中的 `Prometheus` 段暴露 `/metrics` 端点（go-zero agent，
独立于业务端口监听），输出 Prometheus 文本格式：

| 服务 | 业务端口 | /metrics 端口 |
| --- | --- | --- |
| user-service | 8888 | 9091 |
| game-catalog | 8890 | 9092 |
| content-service | 8891 | 9093 |
| community | 8892 | 9094 |
| api-gateway | 8800 | 9095 |
| user-service-rpc | 8080 (gRPC) | 9096 |

```bash
# 验证指标端点
curl http://localhost:9093/metrics
```

### 内置指标（go-zero 自动采集）

rest 服务（`rest.MustNewServer` 内部调用 `ServiceConf.SetUp()` 启动 agent，Prometheus 中间件默认开启）：

- `http_server_requests_duration_ms_bucket{path,method,code}` — 请求耗时直方图
- `http_server_requests_code_total{path,method,code}` — 按状态码统计的请求量

zrpc 服务（经 `UnaryPrometheusInterceptor`）：

- `rpc_server_requests_duration_ms_bucket{method,code}` 等

### 自定义指标（content-service 跨服务调用）

`services/content-service/client` 对 game-catalog 的跨服务调用（含熔断与重试）导出：

- `content_service_gamecatalog_client_requests_total{result}` — 调用次数，
  `result` ∈ success / breaker_open / transient_error / bad_request / canceled / contract_error
- `content_service_gamecatalog_client_retries_total` — 瞬时错误触发的重试次数
- `content_service_gamecatalog_client_request_duration_seconds` — 调用总耗时（含重试与退避）
- `content_service_gamecatalog_client_breaker_state` — 熔断器状态：0=closed 1=half-open 2=open

错误率示例（PromQL）：

```promql
sum(rate(content_service_gamecatalog_client_requests_total{result!="success"}[5m]))
  / sum(rate(content_service_gamecatalog_client_requests_total[5m]))
```

### 新服务接入监控

在 `etc/*.yaml` 中增加配置即可（无需改代码）：

```yaml
Prometheus:
  Host: 0.0.0.0
  Port: <未占用端口>
```

## 认证与授权（JWT）

### 现状总览（6 个服务）

| 服务 | 鉴权状态 | 实现位置 |
|------|---------|---------|
| user-service | ✅ 签发 + 全局校验 | 签发：`utils/auth.go` `GenerateToken`（HS256，默认 7 天）；校验：`middleware/auth.go` 全局挂载（`server.Use`），白名单 `/auth/register`、`/auth/login`、`/ping`、`/health`、`/from/` 前缀 |
| content-service | ✅ 可选校验 | `utils/auth.go` + `middleware/auth.go`（GET 公开、写接口需令牌），已有单测 |
| community | ✅ 按路由校验 | `internal/middleware/auth_middleware.go`（`rest.Middleware`），写路由经 `rest.WithMiddlewares` 挂载，已有集成测试 |
| api-gateway | ✅ 透传 | 转发 `Authorization` 头到上游（`upstream_test.go` 覆盖） |
| user-service-rpc | ➖ 无需鉴权 | 仅集群内部 gRPC 通信，不直接暴露公网 |
| game-catalog | ✅ 按路由校验（本次新增） | `utils/auth.go` + `middleware/auth.go`，`POST /games` 受保护，4 个 GET 路由保持匿名公开 |

### 令牌机制

- **算法**：HS256 对称签名；user-service 是唯一签发方，其余服务只做校验。
- **载荷**：`user_id`（int64）、`username`（string），校验通过后注入请求上下文 key `user_id` / `username`。
- **过期**：签发方 `TokenExpire` 默认 7 天；校验方在 `ParseToken` 中强制检查 `exp`。
- **密钥**：各服务 `etc/*.yaml` 的 `Auth.JWTSecret`，支持 `JWT_SECRET` 环境变量覆盖；
  开发环境共享默认值，生产环境必须通过环境变量注入独立密钥。

### 受保护路由示例（game-catalog）

```bash
# 登录获取令牌（user-service 签发）
TOKEN=$(curl -s -X POST http://localhost:8888/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","password":"***"}' | jq -r '.data.token')

# 匿名读：公开
curl http://localhost:8890/games

# 匿名写：401
curl -X POST http://localhost:8890/games -d '{}'   # {"code":401,"message":"缺少认证令牌"}

# 携带令牌写：通过
curl -X POST http://localhost:8890/games \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Halo","description":"FPS","genres":["FPS"],"platforms":["Xbox"],"release_date":"2001-11-15","developer":"Bungie","publisher":"Microsoft","tags":[],"score":9.5,"cover_image":""}'
```

### 新服务接入认证

1. 复制任一服务的 `utils/auth.go`（校验工具）到本服务；
2. `config.Config` 增加 `Auth.JWTSecret`（带 `env=JWT_SECRET`），yaml 补 `Auth` 段；
3. 按 go-zero 惯例二选一挂载：
   - 全局：`server.Use(middleware.AuthMiddleware(serverCtx))`（user-service 模式，配白名单）；
   - 按路由：`rest.WithMiddleware(...)` / `rest.WithMiddlewares(...)` 包裹受保护路由（community / game-catalog 模式）。

## 故障排查

### 1. 常见问题

- **服务启动失败**: 检查配置文件格式和端口占用
- **RPC 调用失败**: 检查 etcd 连接和服务注册
- **数据库连接失败**: 检查连接字符串和权限

### 2. 调试技巧

```bash
# 查看服务日志
tail -f logs/user-service.log

# 查看服务注册
etcdctl get --prefix "" | grep user-service

# 测试服务连通性
curl -v http://localhost:8888/health
```

## CI 流水线

`.github/workflows/ci.yml`（GitHub Actions）在 push 到 main 与所有 PR 时执行三道门禁：

1. **gofmt 检查**：`gofmt -l .` 非空即失败；
2. **全模块构建**：遍历 `go work edit -json` 声明的模块执行 `go build ./...`（新增服务自动纳入，无需改 workflow）；
3. **全模块测试**：同列表执行 `go test ./... -race -count=1`。

Go 版本跟随 `go.work`（`actions/setup-go@v5` 的 `go-version-file`），模块间共享构建缓存。
等价的本地验证：

```bash
gofmt -l .                                        # 应无输出
for m in $(go work edit -json | jq -r '.Use[].DiskPath'); do
  (cd "$m" && go build ./... && go test ./... -race -count=1) || echo "FAIL: $m"
done
```

CD（部署流水线）暂未配置：当前无生产部署目标（K8s 集群/镜像仓库凭据），待部署方案确定后补充。

## 下一步

1. ~~实现用户认证和授权~~ ✅ 已完成（详见上文「认证与授权（JWT）」）：
   核查 6 个服务均有 JWT 体系——user-service 签发+全局校验、content-service 可选校验、
   community 按路由校验、api-gateway 透传、user-service-rpc 内网免鉴权（设计如此）；
   唯一缺口 game-catalog（有公开写端点 POST /games）已补齐：`utils/auth.go` 校验工具 +
   `middleware/auth.go` + 路由挂载（`POST /games` 受保护、GET 匿名公开）。
   同时补齐 user-service 鉴权中间件与 game-catalog 认证工具/中间件/路由集成测试。
2. 添加数据库模型和缓存 ⚙️ 部分完成：设计决策已落档（见上文「数据库集成 →
   设计决策：数据库模型与缓存」——落地顺序 user-service → game-catalog →
   community/content、DSN 双驱动部署形态、进程内 TTL+LRU 缓存选型）；
   首个切片 user-service 已落地（模型层读缓存 + 单测/集成测试，
   全模块 build + `-race` 门禁通过）；game-catalog 起的后续切片待做
3. ~~实现服务间通信~~ ✅ 已完成（通路审计 + 补齐客户端契约测试）：

   | 通路 | 实现位置 | 契约测试 |
   |------|---------|---------|
   | api-gateway → user-service | `services/api-gateway/internal/integration/user_client.go`（Login、GetRecommendations，Bearer 透传） | `user_client_test.go`（httptest 断言路径/查询/头/响应解析） |
   | api-gateway → game-catalog | `services/api-gateway/internal/integration/game_client.go`（GetFeatured） | `game_client_test.go` |
   | api-gateway → community / content（反向代理） | `internal/proxy` + `internal/{community,content}/routes.go` | `internal/proxy` 上游转发测试（既有） |
   | content-service → game-catalog | `client/gamecatalog.go`（熔断/重试/指标加持） | `client/*_test.go`（既有，89.7%） |
   | user-service → game-catalog | `internal/integration/gamecatalog_client.go`（GetRecommendations） | `gamecatalog_client_test.go`（新增，97.2%） |
   | 其余服务 → user-service-rpc | `user-service-rpc`（zrpc + etcd） | `user_test.go` 集成测试（run() 起真 zRPC + userclient 端到端 Ping，见「覆盖率台账刷新」一节） |
   | community | 无出向调用（独立域） | — |

   全部 HTTP 通路的端点/参数/响应契约均已对照服务端真实路由核实
   （曾修复 content-service 客户端路径不一致问题，见 ENHANCEMENT.md）。
4. 集成监控和日志收集
5. ~~配置 CI/CD 流水线~~ ✅ CI 部分已完成（`.github/workflows/ci.yml`，见上文「CI 流水线」）：
   push/PR 触发 gofmt + 全模块 build + `go test -race` 门禁，模块列表动态读取 go.work；
   CD 部分（部署流水线）因无部署目标暂缓。

更多详细信息请参考：
- [Go-zero 官方文档](https://go-zero.dev/)
- [API 设计指南](./api-design.md)
- [架构文档](./architecture/overview.md)