# User Service 实现文档

## 概述

User Service 是 spawn 平台的账号中心：注册、登录（JWT 签发）、用户资料与个性化游戏推荐。其余服务不复制口令与凭证数据，一律以平台 JWT 共享密钥自行校验（`Auth.JWTSecret`），或按需 HTTP 回查本服务。

## 运行

- 端口：`8888`（REST，配置 `etc/user-api.yaml`）；gRPC `9091`（`user-service-rpc` 同仓拆出）
- 本地启动：`go run user.go`；测试：`go test ./... -race`
- 存储：SQLite（`file:` DSN，零配置）或 MySQL（`MySQL.DataSource`），启动自动建表

## API 端点

- `GET /from/:name` — 连通性测试接口
- `POST /auth/register` — 注册（用户名唯一，返回 `user_id`）
- `POST /auth/login` — 登录，签发 JWT（`token` + `user_info`）
- `GET /users/:id` — 公开资料（免鉴权；邮箱等私有字段不下发）
- `PUT /users/:id` — 更新资料（本人 JWT）
- `GET /users/:id/recommendations` — 个性化游戏推荐（BFF 聚合用）

## 结构

- `internal/logic` — 业务逻辑（go-zero goctl 生成 + 手写实现）
- `internal/model` — 用户表仓储（双方言）
- `middleware` — JWT 认证中间件
- `utils` — JWT 签发/校验（`JWTClaims`：user_id、username、exp）
- `test.sh` / `test-data.json` — 冒烟脚本与样例数据

## 与其他服务的关系

- `api-gateway` 反代 `/auth/*`、`/users/*`（信封原样透传）
- `game-catalog` /me 以本服务签发的 JWT 判角色（`role`、`partner_id` 字段由 game-catalog 维护）
- 注册/登录事实事件上报 data-panel 经营分析（register/login，fire-and-forget）
