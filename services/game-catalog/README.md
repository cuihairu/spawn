# Game Catalog Service 实现文档

## 概述

Game Catalog Service 是 spawn 的游戏目录与发行域：游戏库、版本/包体受控下载、提审发布流、合作方（partner）与角色体系、评价/长评/收藏、区服与预约、推广渠道归因。同时是平台角色的**单一事实源**——`GET /me` 返回 `role` 与 `partner_id`，其余服务经 HTTP 回查，不复制角色数据。

## 运行

- 端口：`8890`（配置 `etc/game-catalog-api.yaml`）
- 本地启动：`go run game.go`；测试：`go test ./... -race`
- 存储：SQLite 或 MySQL（双方言 DDL，启动自动迁移/补列）
- 上报：登录/下载/开服等事实事件 fire-and-forget 上报 data-panel；预约开服/版本发布经 community 扇出站内信

## 主要端点

### 玩家面（公开或平台 JWT）
- `GET /games`、`GET /games/:id`、`GET /games/featured`、`GET /games/recommendations`
- `GET /games/:id/versions/latest`、`GET /games/:id/versions/:versionId/download`（受控下载，可带 `?channel=` 渠道包）
- `GET /me` — 平台角色（`role`: platform_admin/operator/partner/player + `partner_id`）
- `POST /promo/resolve`、`POST /promo/download-click` — 推广链接落地归因（公开）
- 区服列表/预约、评分/长评/举报、收藏（GP-401）：`/games/:id/servers*`、`/games/:id/rating|reviews|report|favorite`

### 管理面（platform_admin / operator）
- `POST /admin/roles`（授权）、`POST /admin/partners`、`GET /admin/partners`、`PATCH /admin/partners/:id`
- `POST /admin/games/:id/ownership`（游戏归属）
- `GET /admin/reviews`、`POST /admin/reviews/:id/decision`（提审批复）
- `POST /admin/games/:id/publish|unpublish`、`GET /admin/games/:id/versions`、版本发布/撤销
- `GET /admin/audit`（操作审计回读）

### 合作方面（partner，按 game→partner 归属隔离）
- `GET /partner/games`、`POST /partner/games/:id/submit`（提审）
- `POST /partner/games/:id/versions`（传包）、`GET /partner/games/:id/versions`、`POST /partner/games/:id/versions/:versionId/package`

## 结构

- `internal/logic` — 各用例（admin/partner/评价/预约/渠道归因等）
- `internal/model` — 双方言仓储 + 审计
- `internal/middleware` — JWT 与角色校验
- `game.api` — go-zero 接口契约

## 角色模型

`platform_admin`（平台管理员）/ `operator`（运营）/ `partner`（合作方，绑定 partner_id，行级隔离按名下游戏）/ `player`（玩家）。管理与合作方端点统一经 `/me` 权威判定，越权按 403/404 不泄露存在性。
