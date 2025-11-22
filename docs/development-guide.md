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

## 下一步

1. 实现用户认证和授权
2. 添加数据库模型和缓存
3. 实现服务间通信
4. 集成监控和日志收集
5. 配置 CI/CD 流水线

更多详细信息请参考：
- [Go-zero 官方文档](https://go-zero.dev/)
- [API 设计指南](./api-design.md)
- [架构文档](./architecture/overview.md)