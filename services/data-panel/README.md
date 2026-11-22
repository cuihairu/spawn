# Data Panel Service 实现文档

## 概述

Data Panel Service 承载两块数据面：**战绩面板**（player_stats，玩家视角只读展示 + 对局记录写入）与**经营分析**（platform_events 事实表：注册/活跃/下载/付费/发奖/退款事件 + 漏斗/留存/活动效果/渠道效果/对账聚合）。两域分表，权限口径不同。

## 运行

- 端口：`8896`（配置 `etc/data-panel-api.yaml`）
- 本地启动：`go run data-panel.go`；测试：`go test ./... -race`
- 存储：SQLite 或 MySQL（双方言 DDL，启动自动建表/补列/写演示种子）
- 角色权威：合作方自助端点经 game-catalog `GET /me` 判角色与 partner 归属（`GameCatalog.BaseURL`），不可达 fail-closed

## 战绩面板（/api/v1/stats）

- `GET /stats/users/:user_id/summary` — 总战绩汇总（对局/胜率/KDA/积分，读侧计算）
- `GET /stats/users/:user_id/games` — 分游戏战绩列表
- `GET /stats/users/:user_id/games/:game_id` — 单游戏战绩
- `POST /stats/records` — 对局记录上报（需 JWT；对账聚合只认服务端事实）

## 经营分析（/api/v1/analytics，AdminUserIds 名单闸）

- `GET /analytics/overview|funnel|retention` — 总览/漏斗/留存
- `GET /analytics/activities/:id/effect`、`/analytics/channels`、`/analytics/reconciliation` — 活动效果/渠道效果/账单核对
- `GET /analytics/partner` — 合作方自助数据（partner 角色专用，行级隔离：优先按 game-catalog 名下游戏收窄，回落 partner_id 命中，两者皆空返回空结果）
- `POST /internal/events` — S2S 事件摄入（X-Internal-Token，未配令牌恒 403；event_id 幂等，批次上限 500）——**不进网关**

## 结构

- `internal/logic/stats` — 战绩读写；`internal/logic/analytics` — 摄入/聚合/合作方自助
- `internal/model` — player_stats 与 platform_events 双方言仓储
- `internal/middleware` — JWT（名单管理员判定在 logic 层）
- `data-panel.api` — go-zero 接口契约（战绩面）
