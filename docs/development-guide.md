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
| 2 | game-catalog | 原 JSON 文件仓库，按 user-service 模式迁 SQLite/MySQL 双驱动 + 读缓存；目录量为数百行，过滤/排序/推荐语义保留在应用侧 | ✅ 本切片已落地 |
| 3 | community、content-service | 同为文件仓库，按 game-catalog 跑通的模式跟进 | content-service ✅、community ✅ 本切片均已落地 |

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
- **game-catalog 切片交付物（2026-10-02）**：
  - `services/game-catalog/model/gamemodel.go` —— JSON 文件仓迁 `GameModel`
    （games 表，数组列 JSON 文本编码；`GameStore` 接口五方法签名不变，
    logic 层零改动）；
  - `services/game-catalog/internal/cache/ttlcache.go` —— 上列泛型缓存的
    模块内同构副本（含单测）；
  - `svc.NewServiceContext` 驱动探测（`file:`/`.db` → SQLite）/`ensureSQLiteDir`/
    Ping 快速失败/建表 + `SeedIfEmpty`（空表写入内嵌 8 款种子目录，
    替代原 `data/games.json`，种子内嵌进二进制）；
  - 读路径语义保持：`List`/`Featured`/`Recommend` 每次全量读取后应用侧过滤排序
    （数百行量级 + 与原内存仓语义一致，SQL 不下推）；跨方言确定性以
    `ORDER BY id` 为基线，并列趋势分的排序回退到 id 序（与原插入序的次序
    漂移见台账）；List 的 `Limit=0` 返回 0 行等模型层语义不变（logic 层
    兜底 20 与原先一致）；
  - Dockerfile 改 CGO 构建 + debian:bookworm-slim 运行时（go-sqlite3 需 cgo），
    `DATASOURCE` 默认 `file:/app/data/games.db`。
- **content-service 切片交付物（2026-10-02）**：
  - `services/content-service/model/guidemodel.go` + `commentmodel.go` ——
    Guide/Comment 双 JSON 文件仓迁 `GuideModel`/`CommentModel`（guides/comments
    两表，tags 数组列 JSON 文本编码；`GuideStore` 七方法 / `CommentStore`
    五方法签名不变，logic 层零改动）；
  - `services/content-service/internal/cache/ttlcache.go` —— 泛型缓存的
    模块内同构副本（含单测）；
  - `svc.NewServiceContext` 驱动探测 / `ensureSQLiteDir` / Ping 快速失败 /
    建表 + `SeedIfEmpty` ×2（空表写入内嵌 2 攻略 + 3 评论种子，替代原
    `data/guides.json`、`data/comments.json`，种子内嵌进二进制）；
  - 两模型构造器 `db.SetMaxOpenConns(1)`：并发测试实写并发（SQLite 文件库
    多连接写触发 SQLITE_BUSY），且 `:memory:` 每连接各得独立空库；单实例 +
    低写速率假设备注于模型注释；
  - 读路径语义保持：`List` 全量读取后应用侧过滤 + 分页钳制（与原内存仓
    逐分支一致）；`ORDER BY id` 与原插入序严格一致（自增 id 恒单调，
    零次序漂移）；`Seed` 显式保留 id、`Create` 走 LastInsertId，
    「删除后 id 不复用」语义不变；
  - Dockerfile 改 CGO 构建 + debian:bookworm-slim（同 game-catalog），
    `DATASOURCE` 默认 `file:/app/data/content.db`。
- **community 切片交付物（2026-10-02）**：
  - `services/community/internal/model/{topic,post,follow}model.go` —— 三 JSON
    文件仓迁 `TopicModel`/`PostModel`/`FollowModel`（topics/posts/follows 三表；
    `TopicStore` 五方法 / `PostStore` 九方法 / `FollowStore` 五方法签名不变，
    logic 层零改动）；软删语义落库：posts 增 `status` 列（`Delete` 置
    `deleted`、行保留），与原内存仓「删除后不可见」逐分支一致；
    `hotScore` 时间衰减与 `Hot` 置顶优先排序保留在应用侧（不下推 SQL）；
  - `FollowModel` **不接缓存**（写多读少的关联表，唯一索引单查已足够；
    Follow/Unfollow 的 bool 返回由「影响行数」判定，双方言幂等写
    `INSERT OR IGNORE`/`INSERT IGNORE` + `UNIQUE(user_id,target_type,target_id)`）；
  - `services/community/internal/cache/ttlcache.go` —— 泛型缓存的模块内
    同构副本（含单测），接入 TopicModel/PostModel 读路径；
  - `svc.NewServiceContext` 驱动探测 / `ensureSQLiteDir` / Ping 快速失败 /
    建表 ×3 + `SeedIfEmpty` ×2（话题 2 条 + 帖子 2 条内嵌种子，替代原
    `data/{topics,posts,follows}.json`；follows 表刻意不设种子，空表起步）；
  - Dockerfile 改 CGO 构建 + debian:bookworm-slim（同前两切片），
    `DATASOURCE` 默认 `file:/app/data/community.db`。

### 5. 覆盖率台账刷新与 user-service-rpc 测试落地（2026-09-30）

全仓覆盖率快照（`services/cov_*.out`，本地不入库；生成命令：
`cd services/<模块> && go test ./... -covermode=atomic -coverpkg=./... -coverprofile=...`
后按块聚合去重，可信总数以 `go tool cover -func` 为准）：

| 模块 | 覆盖率 | 备注 |
|------|-------|------|
| api-gateway | 98.2% | 本轮收口（原 96.3%） |
| user-service | 97.7% | 本轮独立复核确认天花板（派发 97.2% → 现 97.7%） |
| game-catalog | 97.5% | DB 切片轮重测（原内存仓收口 97.6%；迁 DB 新增模型/缓存代码后同口径复测） |
| community | 98.3% | DB 切片轮重测（原内存仓收口 99.2%；迁 DB 新增模型/缓存代码后同口径复测，缺口全量登记） |
| content-service | 97.2% | DB 切片轮重测（原内存仓收口 98.2%；迁 DB 新增模型/缓存代码后同口径复测，缺口全量登记） |
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

**user-service 仓储接口化收口轮（次日第九轮，97.2% → 97.7%）**：上段登记的
update:85（更新后二次回读失败 → 500）与 update:97（昵称 NULL 回退 else）
复核后确认——不可达的根源是 `ServiceContext` 持有具体指针
`*model.UserModel`，对称 DB 故障（关库/只读库）在先行语句即返回，
单请求内无法注入「写成功、随后读失败」与「回读 NULL 昵称」，
而非分支本身死代码（生产瞬时故障与历史 NULL 行均可达）。
处置沿用 content-service 第六轮先例：model 包新增 `UserStore` 接口（含
`var _ UserStore = (*UserModel)(nil)` 编译期断言），`ServiceContext.UserModel`
字段改接口类型；生产装配不变（`NewServiceContext` 仍注入同一具体模型，
logic 调用签名不变；既有测试的复合字面量构造同步兼容）。
据此以内嵌真实仓储 + 仅覆写 `FindOne` 的故障实现补测 2 例
（`internal/logic/updateuser_refetch_test.go`）：二次回读报错 → 500
「更新成功，但获取信息失败」；二次回读昵称 NULL → 回退分支取 username。
剩余 11 块维持不可达登记：register:89（bcrypt ErrPasswordTooLong 需 >72
字节，校验上限 50）、login:57（HS256 签发恒成功）、utils:69（jwt/v5 在
Parse 阶段已完成 exp 与签名校验，err==nil 蕴含 token.Valid 的死代码，
全仓同款既有登记；原与 login:57 合并写作「HS256 签发恒成功」系误标，
签发与该 Parse 侧分支无关，已更正）、
usermodel:90（原 :77，mattn/go-sqlite3 LastInsertId 恒 (id, nil)；行号随接口
插入平移）+ main ×2、handler envelope ×5。


**user-service 覆盖率天花板验证轮（次日第十轮，97.7% → 97.7%）**：复测全模块覆盖率快照（台账口径），
仍为 97.7%。本轮新增 1 例单测：
- `model/usermodel_test.go` 追加 Create 分支（关库触发 Exec 错误 → 500 "创建用户失败"），
  覆盖 `usermodel.go:85` Exec 错误分支；
复核其余候选均为既定不可达集，维持台账登记：
1. `usermodel.go:90` LastInsertId —— mattn/go-sqlite3 恒返回 (id, nil)；
2. `registerlogic.go:89` HashPassword —— bcrypt ErrPasswordTooLong 需 >72 字节，校验上限 50；
3. `loginlogic.go:57` GenerateToken —— HS256 签发既定恒成功（签发侧；utils:69 为
   Parse 侧死代码，成因不同，见第 5 条与第九轮段落更正）；
4. `handler/*` envelope 空错分支 ×5 —— 对应 logic 恒返回 `ApiResponse, nil`（login 除外，其
   handler 分支已 100%）；
5. `utils/auth.go:69` ParseToken "无效的令牌" —— jwt/v5 Parse 阶段已完成 exp/签名校验，
   err==nil 蕴含 token.Valid；
6. `user.go:23` main stderr+exit —— 全仓既定不可达约定。
剩余 11 块即上述全量不可达集。手写面最低可推进项现为 user-service-rpc（91.7%，protoc
缺口维持不碰）。

**user-service 覆盖率独立复核轮（次日第十一轮，97.7% → 97.7%）**：任务派发读数 97.2%，
复测时模块已由任务间落地的两笔提交推进至 97.7%（`0be40ed` UserStore 接口化 + refetch
故障注入、`191eda0` 关库 Exec 分支 + 天花板登记），按铁律不重写、在其上叠加。本轮逐块
读源码对剩余 11 块 / 14 句（总 601 句）做独立复核——不照抄台账结论，全部 11 块证实
为真不可达而非「不可注入」误判（content-service 第六轮教训）：
1. `user.go:24/25` main stderr+exit ×2 —— 全仓既定约定；run() 错误路径已单独覆盖，
   user.go 其余块 100%；
2. `handler/*` envelope 空错分支 ×5 —— 逐行核对 getuserinfo / getuserrecommendations /
   register / updateuserinfo / user 五个 logic，全部仅 `return &types.ApiResponse{...}, nil`，
   结构上无非 nil error 出口（login 的 logic 真返错误，其 handler 分支已 100%）；
3. `loginlogic.go:57` GenerateToken 失败 —— 读 jwt/v5 **v5.3.0** `hmac.go` 源码：`Sign`
   仅当 key 非 `[]byte` 时返回 `ErrInvalidKeyType`，本包 `jwtSecret` 恒为 `[]byte`
   （空 secret 亦然），无空 key 拒绝分支；`SHA256.Available()` 由既有签发/登录绿测反证；
4. `registerlogic.go:89` HashPassword 失败 —— bcrypt `ErrPasswordTooLong` 需 >72 字节，
   `ValidatePassword` 上限 50 字节且先于加密执行（registerlogic.go:47 → :87 顺序核对）；
   错误路径本身由 `utils.TestHashPassword_TooLong` 覆盖；
5. `usermodel.go:90`（原 :77，随接口插入平移）LastInsertId 失败 —— **补强前序登记的
   单驱动论证**：本仓为双驱动装配（`servicecontext.go:37` sqlite3 / `:40` mysql），
   逐一核对 mattn/go-sqlite3 v1.14.32 `return r.id, nil`、go-sql-driver/mysql v1.9.3
   `result.go` `return res.insertIds[...], nil`，均字面恒 nil error；`database/sql`
   `driverResult.LastInsertId` 仅加锁转发；本仓无 sqlmock 依赖，不为其引入（不硬造）；
6. `utils/auth.go:69` ParseToken「无效的令牌」—— jwt/v5 `ParseWithClaims` err==nil 时
   claims 类型恒为 `*JWTClaims` 且 `token.Valid` 必真，断言后置分支不可入。
另更正前序记录：`191eda0` 提交信息「372 stmts」系与 game-catalog 总句数混淆，
user-service 实测 601 句（587/601 = 97.7%，未覆盖 14 句 / 11 块）。快照已刷新
（`services/cov_user-service.out`，本地不入库）。升至 98.0% 需再盖 2 句，而 14 句已全证死，
97.7% 为该模块硬顶；手写面最低可推进项维持 user-service-rpc（91.7%，protoc 缺口不碰）。

**content-service 收口轮（同日第四轮，88.8% → 94.2%）**：已补测试——
- logic 层：五个 401 分支（createcomment/createguide/deletecomment/updateguide/
  publishguide，ctx 缺 user_id）、createcomment 的目标攻略缺失 404 与昵称回退、
  createguide 的游戏拉取失败回退 gameId 标题与昵称回退、likecomment 的评论缺失
  404 与指向已删攻略的二级 404（种子孤儿评论触达）、listcomments/listguides
  分页归一化与草稿攻略评论 404、updateguide 的 cover_image/tags 增量字段、
  mapper 两个 nil 防御分支（`internal/logic/errorbranches_test.go`）；
- middleware：非 Bearer 头 401、空 Bearer token 401、/ping 与 /health 放行、
  GET 非 guides/comments 前缀的兜底 return；
- client：构造参数归一化（负值/nil 注入项）、退避期间 ctx 取消、调用前 ctx
  已取消、截断响应体（hijack 写半截 Content-Length）、gameId 含控制字符的
  构造请求失败、statusForInternalMetrics/resultLabel 的 unknown/canceled 映射、
  breaker SetOnChange 回调与 half-open 双探测（白盒置态）、非法状态名的
  unknown 兜底、退避封顶的循环内与循环后两个分支（`client/extras_test.go`）。

content-service 剩余 43 块全部按台账口径登记（不硬造用例）：
- 仓储为纯内存 map（构造时一次性加载，写入不落盘），`Get`/写方法各自只有
  单一哨兵错误——「get failed → 500」分支（getguide:38、createcomment:50、
  likecomment:38/55、likeguide:38、deletecomment:46、updateguide:46、
  publishguide:46）与「Get 成功后写操作再 404/500」分支（likecomment:71/77、
  likeguide:53/59、deletecomment:61/67、publishguide:61/67、updateguide:79/85、
  createcomment:81、createguide:70）均不可达；
- handler `httpx.ErrorCtx` 空错分支 ×10：十个 logic 恒返回 envelope+nil error
  （与 user-service 同款 envelope 模式，逐一核对）；
- `content.go` main stderr+exit（全仓既定）；
- `utils/auth.go:46/51`：jwt/v5 在 Parse 阶段已完成 exp 与签名校验，
  err==nil 蕴含 token.Valid，两分支为死代码；
- `client/gamecatalog.go:157`（重试循环编译完整性兜底，源码注释自述）、
  `gamecatalog.go:176` 与 `metrics.go:90`（错误类别 switch 穷举后的 default）。

**content-service 仓储接口化收口轮（次日第六轮，94.2% → 98.2%）**：原登记为
不可达的「仓储 Get/写失败 → 500」与「Get 成功后写再 404/500」两类分支（约
41 语句）复核后确认——不可达的根源是 `ServiceContext` 持有具体指针类型
`*model.GuideRepository` / `*model.CommentRepository`，故障无法注入，而非分支
本身死代码。处置：model 包新增 `GuideStore` / `CommentStore` 接口（含
`var _ XxxStore = (*XxxRepository)(nil)` 编译期断言），`ServiceContext` 两个字段
改为接口类型；生产装配不变（`NewServiceContext` 仍注入同一具体仓储，行为
零变化），logic 层方法调用签名不变。据此以「内嵌真实仓储 + 仅覆写指定方法
注入错误」的故障仓储补测 19 例（`internal/logic/repoerror_test.go`）：getguide/
createguide/updateguide/publishguide/likeguide 的 Get 500 与写 404/写 500，
createcomment 的目标攻略 Get 500 与评论落库 500，deletecomment/likecomment
的评论 Get 500、二级攻略 Get 500、写 404 与写 500，以及 updateguide 此前
漏测的 Summary 增量字段。剩余 18 块维持不可达登记：
- handler `httpx.ErrorCtx` 空错分支 ×10（十个 logic 恒 envelope+nil error）；
- `content.go` main stderr+exit ×2；
- `utils/auth.go:46/51`（jwt/v5 Parse 阶段已完成 exp 与签名校验的死代码）；
- `client/gamecatalog.go:157`、`gamecatalog.go:176`、`metrics.go:90`
  （编译完整性兜底与穷举 switch default，重试次数归一化/错误类别枚举为
  构造器不变量，生产不可达）。

**game-catalog 收口轮（次日第七轮，95.7% → 97.6%）**：按台账口径重跑快照
（372 语句，原 16 未覆盖语句 / 12 块），逐块分诊后 4 块补测、8 块登记。
沿用 content-service 第六轮的仓储接口化先例——`ServiceContext` 原持具体指针
`*model.GameRepository`，故障无法注入：model 包新增 `GameStore` 接口（含
`var _ GameStore = (*GameRepository)(nil)` 编译期断言），字段改接口类型，
生产装配与 logic 调用签名不变。已补测试——
- svc 层：相对路径数据源走 `filepath.Clean` 归一化分支，文件不存在时回落
  内置种子（`internal/svc/servicecontext_test.go`）；
- logic 层故障注入 ×2：getgamedetail 仓储 Get 非哨兵错误 → 500 查询失败、
  creategame 校验通过后落库失败 → 500 创建失败
  （`internal/logic/repoerror_test.go`，同包访问 `*apiError` 断言状态码）；
- handler 层：请求上下文缺路由变量 id 时 `httpx.Parse` 失败走
  `httpx.ErrorCtx` 分支 → 400（`internal/handler/getgamedetail_parse_test.go`，
  go-zero 对缺失 path 必填字段报错；真服务路由恒有 pathvar，故仅直调注入）。

game-catalog 剩余 8 块（372 语句中 9 句）登记不可达（不硬造用例）：
- `game.go` main stderr+exit ×2（全仓既定；run() 错误路径与其他服务一致
  已有单测）；
- handler `httpx.ErrorCtx` 空错分支 ×3：listgames/getfeaturedgames/
  getrecommendations 三个 logic 恒 `return &types.XxxResponse{...}, nil`
  无非 nil error（逐一核对；getgamedetail 的 Parse 分支本轮已补）；
- `model/gamerepository.go:290`：内嵌种子 JSON 编译期字面量，
  `defaultSeedGames` 的 unmarshal panic 生产不可达；
- `utils/auth.go:43/47`：jwt/v5 Parse 阶段已完成 exp 与签名校验，
  err==nil 蕴含 token.Valid，两分支为死代码（全仓同款既有登记）。

**api-gateway 收口轮（次日第八轮，96.3% → 98.2%）**：按台账口径重跑快照
（原 10 未覆盖语句 / 8 块），逐块分诊后 4 块补测、4 块登记。已补测试——
- integration 层：两个 client 构造器不校验 baseURL，配置值携带控制字符时
  `http.NewRequestWithContext` 内部 `url.Parse` 失败，GetFeatured / Login /
  GetRecommendations 的构造请求错误分支 ×3 由此触达（不经网络，
  `internal/integration/client_malformed_url_test.go`）；
- proxy 层：`serveHTTP` 的请求构造错误分支 → 502 bad gateway——真 http
  服务器在协议解析层即拒绝非法方法 token，handler 永远收不到，故白盒直调
  注入敌意方法（与既有 `TestCopyHeaderDeletesHopByHop` 同款直测口径，
  `internal/proxy/upstream_badrequest_test.go`）。

api-gateway 剩余 4 块（5 句）登记不可达（不硬造用例）：
- `gateway.go` main stderr+exit ×2（全仓既定）；
- `internal/integration/user_client.go:52`：`json.Marshal(LoginPayload)`
  仅两字符串字段，Marshal 对任意字符串（含非法 UTF-8 会替换而非报错）
  恒成功，分支死代码；
- `internal/proxy/upstream.go:117`：`Ping` 的构造请求错误分支——baseURL
  在 `NewUpstream` 已通过 Parse 与 scheme/host 校验，`Parse(String()+"/health")`
  恒成功，构造器不变量。

风险登记（仅登记不动手）：2026-10-01 push 响应提示 GitHub 对默认分支报
164 个 dependabot 告警（13 critical / 72 high / 47 moderate / 32 low），
2026-10-02 push 复测为 168 个（13 critical / 74 high / 49 moderate / 32 low，
波动来自依赖图刷新，非本仓改动引入——两个 DB 切片新增 mysql/sqlite3 驱动
各带少量间接依赖），
详见仓库 security/dependabot 页；按任务约束本轮不处置。

**community 收口轮（同日第五轮，91.5% → 99.2%）**：按台账口径重跑快照确认
community 为手写面最大缺口（77 个未覆盖块），逐块分诊后 70 块补测、7 块登记。
已补测试——
- model 层（`internal/model/errorbranches_test.go`）：persist 五个故障分支
  （无目录前缀直返 nil、空文件、父路径为普通文件的 MkdirAll 失败、chan 不可
  序列化的 marshal 失败、临时路径被目录占用的 WriteFile 失败）；follow 仓储
  种子 JSON 装载的两组填充循环、重复关注/无关系集/目标缺失的六个 false 返回、
  保存与列举时 ≥2 元素集合的排序比较器；topic 仓储空数组种子回落默认话题、
  null 元素在建索引与列表跳过、Create 落盘失败、List 的 offset/limit/越界
  三处钳制、不存在话题的两种计数自增哨兵错误；post 仓储空数组种子、null
  元素与状态/类型过滤的三个 continue、列表三处钳制、Hot 的 null 跳过与
  limit<=0 回落、Update 的 Images 增量字段及 Update/Create/Like/Share 的
  落盘失败四分支、hotScore 的 CreatedAt 解析失败与 decay=4 / decay=8 分桶；
- logic 层：topic 包分页钳制 ×3、GetTopic 空请求 400、GetFollowingTopics 的
  401 与已删话题 continue、follow/unfollow topic 的 401+400、CreateTopic 的
  401/空请求/空名称/落盘失败（`internal/logic/topic/errorbranches_test.go`）；
  follow 包 FollowUser/UnfollowUser 的 401、user_id 缺失 400、自取关 400
  （`internal/logic/follow/errorbranches_test.go`）；post 包 CreatePost 的
  落盘失败（`internal/logic/post/create_save_error_test.go`）；
- handler 层：逻辑可报错的三个 handler（create_topic/unfollow_user/
  get_following_topics）在请求上下文缺 user_id 时 401 走 `httpx.ErrorCtx`
  分支（`internal/handler/errorbranch_test.go`）。

community 剩余 7 块全部按台账口径登记（不硬造用例）：
- `community.go:24-27`（main 的 run 错误 stderr+exit 两块，全仓既定；run()
  自身的错误返回已由 `community_test.go` 的 LoadConfigError 覆盖）；
- handler `httpx.ErrorCtx` 空错分支 ×3：get_topics/get_posts/get_hot_posts
  三个 logic 无任何非 nil error 返回（纯查询 + 分页钳制，逐一核对）；
- `utils/auth.go:40/44`：jwt/v5 在 Parse 阶段已完成 exp 与签名校验，
  err==nil 蕴含 token.Valid，两分支为死代码（与 user-service/content-service
  同款既有登记）。

**game-catalog 数据库切片轮（2026-10-02，97.6% → 97.5%，功能切片）**：「下一步」
第 2 项第二切片落地——JSON 文件仓迁 SQLite/MySQL 双驱动 `GameModel` + 进程内读
缓存（设计决策见「数据库集成 → 4」），非覆盖率专项轮；覆盖率按同口径重测并登记。
- 语义保持：`GameStore` 五方法签名不变，logic 层与第七轮的 `repoerror_test.go`
  故障注入测试零改动通过；缓存值副本隔离、无负缓存、Create 统一失效点，
  与 user-service 切片同款；
- 新增测试：模型层（SeedIfEmpty 填充与空表跳过、缓存关库命中、值副本隔离、
  文件库持久化重开、过滤/排序/分页钳制、推荐题材池与轮转确定性、关库错误
  传播（列表读静默零值、单条读写显式报错）、Seed 主键冲突传播、数组列损坏
  JSON ×3 列与空串列与数值列脏数据、同分 tie-break、offset>0 且 limit<0 的
  窗口倒挂防御钳制）；svc 层（SQLite 文件库建表种子、内存库带参 DSN 跳过
  目录创建、Open 急切解析 panic、Ping 不可达 panic——实测
  `user:pw@tcp(127.0.0.1:3306)x` 会在 Open 期 ParseDSN 即失败（缺 "/dbname"
  分隔），并不触达 Ping，前序 user-service 巡检第三轮对该 DSN 的描述据此
  修正理解：Open-panic 与 Ping-panic 是两个分支，各自需不同 DSN 形态）；
  handler 层（列表类 query `limit=abc` 类型不匹配 → `httpx.Parse` 失败 400
  ×3——字段全 optional/default 的正常路径 Parse 恒成功，类型不匹配是唯一
  可达 Parse 失败入口，与 getgamedetail 缺 pathvar 同口径）；
- 删除：`model/gamerepository*.go`、`data/games.json`。第七轮台账所引
  `model/gamerepository.go:290`（defaultSeedGames panic）现平移至
  `model/gamemodel.go:458`，登记不变；
- 剩余缺口登记（DB 切片后重测，全量 15 块 / 16 句，均不可达或外部依赖口径）：
  1. `game.go` main stderr+exit ×2（全仓既定）；
  2. handler `httpx.ErrorCtx` 逻辑错误分支 ×3：listgames/getfeaturedgames/
     getrecommendations 三个 logic 恒 envelope+nil error（逐一核对，与第七轮
     登记同款；getgamedetail/creategame 的对应分支既有覆盖）；
  3. `svc/servicecontext.go` 建表 panic 与种子 panic ×2 —— SQLite 路径 Ping
     通过后建表恒成，MySQL 在线但建表/种子失败需真实 MySQL 故障注入，
     CI（SQLite-only）口径不可达；
  4. `model/gamemodel.go` all 的 rows.Err —— 需驱动级遍历中断注入；
  5. `model/gamemodel.go:458` defaultSeedGames panic —— 编译期内嵌常量；
  6. `utils/auth.go:43/47` jwt/v5 死分支（全仓同款既有登记）。

**content-service 数据库切片轮（2026-10-02，98.2% → 97.2%，功能切片）**：
「下一步」第 2 项第三切片落地——Guide/Comment 双 JSON 文件仓迁 SQLite/MySQL
双驱动 `GuideModel`/`CommentModel` + 进程内读缓存（设计决策见「数据库集成 → 4」），
非覆盖率专项轮；覆盖率按同口径重测并登记。
- 语义保持：`GuideStore` 七方法 / `CommentStore` 五方法签名不变，第六轮
  `repoerror_test.go` 故障注入测试改嵌 `GuideStore`/`CommentStore` 接口后
  零行为改动通过；缓存值副本隔离（Guide 含 Tags 切片深拷）、无负缓存、
  写路径统一失效点，与前两切片同款；
- id 语义：表主键 AUTOINCREMENT/AUTO_INCREMENT，`Seed` 显式保留 id
  （自增从最大 id 续），`Create` 走 LastInsertId——「删除后 id 不复用」
  原断言保持；`ORDER BY id` 与原插入序严格一致（自增 id 恒单调，零漂移）；
- 新增测试：模型层（SeedIfEmpty 填充/跳过、显式 id 保留与非正 id 跳过、
  持久化重开幂等建表、过滤/分页钳制全量用例（含 page=0 回归）、缓存关库
  命中、值副本隔离、关库错误传播（列表读静默零值、单条读写显式报错）、
  Seed 主键冲突传播、tags 列脏数据三态（空串/坏 JSON/数值元素）、数值列
  文本脏行、并发读写、UPDATE 触发器 RAISE(ABORT) 写失败传播）；svc 层
  （SQLite 建库建表种子、`?mode=ro` 只读库四条启动 panic——预建库逐项
  裁剪表与数据可精确分色到建表×2/种子×2，Open/Ping panic 两分支沿用
  game-catalog 口径、内存库带参 DSN）；handler/logic 层装载改临时 SQLite
  （JSON 种子常量反序列化后 Seed，原行为断言不变）；
- 删除：`model/guiderepository*.go`、`model/commentrepository*.go` 与
  `data/guides.json`、`data/comments.json` 装载路径。第六轮台账所引旧文件
  删除，对应结论平移（handler 信封死分支 ×10 与 jwt/v5 死分支登记不变）；
- 部署：Dockerfile 改 CGO 构建 + debian:bookworm-slim（同 game-catalog），
  `DATASOURCE` 默认 `file:/app/data/content.db`；
- 剩余缺口登记（DB 切片后重测，全量 30 块，均不可达/外部依赖/既有登记口径）：
  1. `content.go` main stderr+exit ×2（全仓既定）；
  2. handler `httpx.ErrorCtx` 逻辑错误分支 ×10（第六轮登记：十个 logic 恒
     envelope+nil error，本轮逐一核对不变）；
  3. `client` GetGameById 尾块 / statusForInternalMetrics / resultLabel ×3
     （第六轮登记，client 包测试未动，与切片前一致）；
  4. `utils/auth.go` jwt/v5 Parse 侧死分支 ×2（全仓同款既有登记）；
  5. `model` 驱动不可达 ×11：LastInsertId 失败 ×2（成功 INSERT 后驱动恒成功）、
     RowsAffected 失败 ×5（Publish/Like/IncrementViews/Delete 与 Like），
     Like 回读点赞数失败 ×2（UPDATE 成功后同连接 SELECT 恒成功），
     all 的 rows.Err ×2（需驱动级遍历中断注入，同 game-catalog 口径）；
  6. `model` 防御死分支 ×2：List 循环内 nil-guide/nil-comment continue
     （all() 恒产非 nil 元素）。

**community 数据库切片轮（2026-10-02，99.2% → 98.3%，功能切片）**：
「下一步」第 2 项末切片落地——Post/Topic/Follow 三 JSON 文件仓迁 SQLite/MySQL
双驱动 `PostModel`/`TopicModel`/`FollowModel` + 进程内读缓存（设计决策见
「数据库集成 → 4」），非覆盖率专项轮；覆盖率按同口径重测并登记。
- 语义保持：三 Store 接口签名不变，logic 层零改动；旧 `.tmp` 目录占位式
  落盘失败注入（topic/create 两处）改 `failingTopicStore`/`failingPostStore`
  接口故障注入（ServiceContext 字段已改接口类型，测试直接换实现），
  断言语义不变；`FollowRepo.FollowTopic(7, 99999)` 直写测试照旧可用；
- id 与软删语义：`Seed` 显式保留 id、`Create` 走 LastInsertId；posts 增
  `status` 列表达软删（`Get`/`allVisible`/`Like`/`Share`/`Update`/`Delete`
  对已删行统一返回 ErrPostNotFound，`List`/`Hot` 不再可见）；
  `ORDER BY id` 与原插入序严格一致（自增 id 恒单调，零漂移）；
- 新增测试：模型层（SeedIfEmpty 填充/跳过、nil 与非正 id 种子跳过、
  关键词大小写不敏感与官方过滤、分页钳制、计数增减下限钳 0、缺失话题
  增减返回哨兵、缓存关库命中与写后失效、值副本隔离、并发点赞恰好一次
  生效、持久化重开、关注幂等 bool 语义（重复 false/缺失 false/影响行数）、
  两类目标 id 互不串扰、图像/标签数组列三态（空串/好图坏签/坏 JSON）、
  数值列文本脏行 ×2、Hot 排序（置顶优先 + draft 不上榜 + limit 钳制 +
  IsHot 仅作用返回副本）、hotScore 四分桶与非法时间戳退化、UPDATE
  触发器 RAISE(ABORT) 写失败传播 ×2、AFTER 触发器删行致二次读取失败 ×2）；
  svc 层（SQLite 建库建表种子、`?mode=ro` 只读库五条启动 panic——预建库
  逐项裁剪表与数据精确分色到建表×3/种子×2、Open/Ping panic 两分支、
  内存库两种 DSN 形态、带参 DSN 目录创建）；handler/logic 层装载改临时
  SQLite（原行为断言不变）；
- 删除：`model/{post,topic,follow}_repository*.go`、`model/persist.go`、
  `model/errorbranches_test.go` 与 `internal/svc/data/` 占位文件。原
  errorbranches 台账所引文件级分支已平移（List 权限/可见性断言在
  postmodel_test 重生）；
- 部署：Dockerfile 改 CGO 构建 + debian:bookworm-slim（同前两切片），
  `DATASOURCE` 默认 `file:/app/data/community.db`；etc/community-api.yaml
  改 `MySQL.DataSource` 块；
- 剩余缺口登记（DB 切片后重测，全量 15 块，均不可达/既有登记口径）：
  1. `community.go` main stderr+exit ×2（全仓既定）；
  2. handler `httpx.ErrorCtx` 逻辑错误分支 ×3：getposts/gethotposts/
     gettopics 三个 logic 恒 envelope+nil error（List 系仓储方法无错误
     返回，逐一核对；其余端点的对应分支既有覆盖）；
  3. `utils/auth.go` jwt/v5 Parse 侧死分支 ×2（全仓同款既有登记：
     ParseWithClaims 对过期/无效令牌先行报错，其后的手动校验恒不可达）；
  4. `model` 驱动不可达 ×8：LastInsertId 失败 ×2（topic/post Create，
     成功 INSERT 后驱动恒成功）、RowsAffected 失败 ×3（topic 计数更新、
     follow 幂等写/删），rows.Err ×3（topic List / post allVisible /
     follow List 的驱动级遍历中断，同前两切片口径）。

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
2. ~~添加数据库模型和缓存~~ ✅ 已完成：设计决策已落档（见上文「数据库集成 →
   设计决策：数据库模型与缓存」——落地顺序 user-service → game-catalog →
   community/content、DSN 双驱动部署形态、进程内 TTL+LRU 缓存选型）；
   user-service、game-catalog、content-service、community 四切片全部落地
   （模型层读缓存 + 单测/集成测试，全模块 build + `-race` 门禁通过；
   game-catalog、content-service、community 的 JSON 文件仓均已迁
   SQLite/MySQL 双驱动，交付物见「数据库集成 → 4」）。
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