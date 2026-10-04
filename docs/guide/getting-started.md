# 快速上手

从零把 spawn 跑起来：一键容器化，或分别启动后端服务与前端应用。

## 环境要求

| 依赖 | 版本 | 说明 |
| --- | --- | --- |
| Go | ≥ 1.25 | 各服务 `go.mod` 声明 `go 1.25.4`，根目录 `go.work` 聚合多模块 |
| Node.js | 24 | 与 CI 一致 |
| pnpm | 10.22 | 根 `package.json` 以 `packageManager` 钉定 |
| Docker | 可选 | 一键拉起全部服务与观测栈 |

后端工具链（按需）：`goctl`（go-zero 代码生成）、`protoc` 与 `protoc-gen-go`、`protoc-gen-go-grpc`，安装方式见开发指南「Go-zero 开发约定」。

## 获取代码

```bash
git clone https://github.com/cuihairu/spawn.git
cd spawn
pnpm install
```

## 一键拉起全部服务

```bash
docker compose up --build
```

各服务为独立镜像（CGO 构建 + debian:bookworm-slim 运行时），数据源由 `DATASOURCE` 环境变量注入，默认本地 SQLite 文件库并自动建表、写入内嵌种子数据。

| 服务 | 业务端口 | /metrics 端口 |
| --- | --- | --- |
| user-service | 8888 | 9091 |
| game-catalog | 8890 | 9092 |
| content-service | 8891 | 9093 |
| community | 8892 | 9094 |
| api-gateway | 8800 | 9095 |
| user-service-rpc | 8080 (gRPC) | 9096 |

## 分别启动后端服务

不使用容器时，各服务独立进程运行，配置文件在各自 `etc/` 目录：

```bash
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/content-service/content.go -f services/content-service/etc/content-api.yaml
go run services/community/community.go -f services/community/etc/community-api.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml
```

`api-gateway` 是对外统一入口：JWT 签发与校验、跨服务路由反代、`/home/feed` 聚合都从这里走。生产环境把 `DATASOURCE` 换成 MySQL 连接串即可切换存储，建表与种子迁移在启动时自动完成。

## 启动 Web 端

```bash
pnpm --dir apps/web-client run dev
```

web-client 直连各业务服务开发调试，构建用 `pnpm --dir apps/web-client run build`（先 `tsc -b` 类型检查再 vite 打包）。

## 启动移动端

```bash
pnpm --dir apps/mobile-app start
```

mobile-app 基于 Expo，`expo start` 起开发服务后按提示在模拟器或 Expo Go 中打开；Android release 出包流程见「各端说明」。

## 延伸阅读

- [架构总览](/architecture/overview)——平台愿景与服务边界。
- [现状拓扑](/architecture/topology)——服务边界、端口、网关路由面与认证数据流的现状记录。
- [开发指南](/development-guide)——环境、配置、数据库、认证、监控、CI 与部署的完整手册。
