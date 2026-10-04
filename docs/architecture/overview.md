# 架构概览

> 本文是愿景与方向；**当前已落地的服务边界、端口、网关反代拓扑与数据流**
> 见同目录 `topology.md`（现状文档，冲突时以现状文档为准）。

## 目标

构建一个集合“游戏库 + 攻略内容 + 社区社交 + 战绩数据 + 实时组队”的一体化平台，覆盖 Web/App/小程序，并具备可扩展的后台和自动化能力。

## 系统分层

1. **体验层 (apps/)**：Web、移动、后台、小程序端统一使用设计系统与状态管理，主要通过 GraphQL/BFF 访问后端服务，并与实时通道 (WebSocket/RTC) 交互。
2. **业务服务层 (services/)**：按领域拆分微服务，通过服务网关暴露 API，依赖共享数据模型与配置，支持水平扩展。
3. **数据与实时层**：data-panel、crawler-jobs、实时推送服务等负责抓取、计算与实时同步。
4. **平台支撑层 (platform/)**：提供基础设施、CI/CD、可观测性，保障部署、运维及弹性。

## 核心模块

- **User & Social** (`user-service`, `community`, `matchmaking`): 账号、好友、动态、组队、语音/文字沟通。
- **Content & CMS** (`content-service`, `admin-console`): 攻略/资讯创作、审核、活动配置。
- **Game Catalog & Discovery** (`game-catalog`, `web-client`): 游戏库、榜单、标签、推荐 feed。
- **Data Panel** (`data-panel`, `mobile-app`): 战绩抓取、角色/装备统计、玩家档案。
- **Automation** (`crawler-jobs`, `tools/` scripts): 自动同步外部商店数据、赛事信息、公告。

## 数据流与交互

1. **数据输入**：crawler-jobs 从官方商店/赛事/公告抓取数据，写入数据仓库或缓存；data-panel 通过官方 API 或合作接口获取战绩数据。
2. **处理**：game-catalog/content-service/community 根据数据模型存储在数据库（例如 Postgres/ElasticSearch），同时触发消息总线 (Kafka/PubSub) 更新榜单和推荐。
3. **输出**：api-gateway 将聚合后的数据提供给各端；realtime-hub 负责 WebSocket/推送，向移动端发送战绩变化、组队邀请等实时信息。
4. **反馈**：apps 的用户行为、埋点、战绩上传等通过 shared-utils SDK 上报到数据管道，为推荐和榜单提供闭环。

## 技术栈建议

- **前端**：Next.js/TurboRepo + React Native/Expo + Taro/UniApp（小程序）、Tailwind/设计系统。
- **后端**：TypeScript (NestJS) 或 Go (Gin/gRPC)，GraphQL 层 + gRPC/REST 服务。
- **数据库**：PostgreSQL (主)、Redis (缓存)、ElasticSearch (搜索)、ClickHouse/BigQuery (分析)。
- **消息与实时**：Kafka/NATS、WebSocket、Agora/自建 RTC。
- **DevOps**: Terraform + Kubernetes + ArgoCD/GitHub Actions，Prometheus/Grafana/Tempo/ Loki 监控链路。

## 演进路线

1. MVP：优先落地游戏库、攻略、社区基础功能，接入用户体系与内容发布。
2. Phase 2：上线战绩数据、玩家档案、小游戏组队，实现实时推送。
3. Phase 3：扩展爬虫、数据分析、赛事中心，打通后台运营和数据看板。

## 开发协作约定

- 所有服务/应用共享 `packages/config` 中的工具链；代码通过 pnpm workspace 管理。
- 每个服务提供 `docs/` 子目录记录 API 与运行说明。
- 使用 `tests/` 下的契约测试确保服务之间的接口兼容性。
- 通过平台层提供的 CI/CD pipeline 自动化测试、部署。
