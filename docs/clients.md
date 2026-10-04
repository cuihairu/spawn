# 各端说明

spawn 的客户端与服务端现状一览。规划中的端只列状态，不写未实现的功能。

## 客户端

| 端 | 目录 | 技术栈 | 状态 |
| --- | --- | --- | --- |
| Web 主站 | `apps/web-client` | React 19 · Vite 7 · React Router 7 | 已落地 |
| 移动客户端 | `apps/mobile-app` | Expo 57 · React Native 0.86 · expo-router | 已落地（Android release 出包流程已就绪） |
| 小程序壳 | `apps/mini-program` | — | 规划中，尚未创建 |
| 运营后台 | `apps/admin-console` | — | 规划中，尚未创建 |

## Web 端（web-client）

- 面向攻略、榜单、社区的主站入口，路由用 React Router 组织。
- 开发调试时直连各业务服务；经 `api-gateway` 的聚合接口（`/home/feed`）可一次拿到首页四组数据。
- 常用脚本：`pnpm --dir apps/web-client run dev`（开发）、`build`（tsc + vite 打包）、`lint`。

## 移动端（mobile-app）

- 基于 Expo 与 expo-router 的文件式路由，覆盖开黑、战绩、活动等场景。
- 集成能力：expo-notifications（推送）、expo-linking（深链与分享卡）、expo-secure-store（凭据安全存储）。
- 首页数据走 `api-gateway` 的 `fetchHomeFeed` 聚合消费点。
- Android 出包：`pnpm --dir apps/mobile-app run prebuild:android` 生成原生工程，`build:android:release` 走 gradle assembleRelease。
- 真机回归项（本地推送、深链分享卡）尚待设备验证，进度见[移动端计划](/mobile_plan)。

## 服务端（六个 go-zero 微服务）

| 服务 | 目录 | 职责 | 存储 |
| --- | --- | --- | --- |
| api-gateway | `services/api-gateway` | 接入层：JWT 认证、路由反代、BFF 聚合 | — |
| user-service | `services/user-service` | 账号、登录注册、用户信息与推荐 | SQLite / MySQL 双驱动 |
| game-catalog | `services/game-catalog` | 游戏库、标签、精选、推荐 | SQLite / MySQL 双驱动 + 进程内读缓存 |
| content-service | `services/content-service` | 攻略、评论、发布流 | SQLite / MySQL 双驱动 + 进程内读缓存 |
| community | `services/community` | 帖子、话题、关注、点赞、关注流 | SQLite / MySQL 双驱动 |
| user-service-rpc | `services/user-service-rpc` | 用户域 gRPC 内部接口（zrpc + etcd） | 复用 user-service 存储 |

统一口径：业务端口与 `/metrics` 指标端口见[快速上手](/guide/getting-started)的端口表；服务边界、网关路由面与认证数据流的现状记录见[现状拓扑](/architecture/topology)。

规划中的服务：matchmaking（组队匹配）、realtime-hub（实时推送）、data-panel（战绩数据面板）、crawler-jobs（外部信息抓取）尚未创建，见仓库 README 的目录规划。
