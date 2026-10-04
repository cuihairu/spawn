# spawn

## Monorepo 目录规划

```
.
├── apps                 # 面向最终用户的多端应用
│   ├── web-client       # Web 站点（攻略/榜单/社区）
│   ├── mobile-app       # iOS/Android 客户端（含小黑盒式数据面板）
│   ├── mini-program     # 微信/支付宝等小程序壳
│   └── admin-console    # 运营后台前端
├── services             # 后端与实时服务
│   ├── api-gateway      # BFF / GraphQL / 接入层
│   ├── user-service     # 账号、权限、好友、成长体系
│   ├── game-catalog     # 游戏库、榜单、标签、推荐
│   ├── content-service  # 攻略、资讯、CMS、评论
│   ├── community        # 帖子、话题圈子、互动、通知
│   ├── matchmaking      # 组队/匹配大厅、房间管理、语音信令
│   ├── realtime-hub     # WebSocket/RTC/推送网关
│   ├── data-panel       # 战绩抓取、统计分析、玩家档案 API
│   └── crawler-jobs     # 商店/赛事/公告爬虫与调度
├── packages             # 多端共享组件与工具
│   ├── ui-kit           # 设计体系、跨端组件库
│   ├── data-models      # Protobuf/GraphQL schema、类型定义
│   ├── shared-utils     # 工具函数、Hook、SDK
│   └── config           # 构建配置、lint、环境变量模板
├── platform             # 平台级支撑
│   ├── infra            # Terraform/IaC、K8s、Helm Chart
│   ├── devops           # CI/CD、发布脚本、灰度策略
│   └── observability    # 日志、监控、报警、SLO
├── tools                # CLI、脚本、代码生成、数据迁移
├── docs                 # 产品、技术、API、运维文档
└── tests                # 跨服务集成测试与合规测试
```

> 目录可随业务演化调整，初期可根据优先级逐步落地。

## 可观测性

各 go-zero 服务通过配置中的 `Prometheus` 段暴露 `/metrics` 指标端点（go-zero agent，独立监听端口），
内置请求量/耗时/状态码指标；content-service 另有跨服务调用自定义指标。
各服务指标端口与指标说明见 `docs/development-guide.md` 的「监控与指标（Prometheus）」章节：

| 服务 | 业务端口 | /metrics 端口 |
| --- | --- | --- |
| user-service | 8888 | 9091 |
| game-catalog | 8890 | 9092 |
| content-service | 8891 | 9093 |
| community | 8892 | 9094 |
| api-gateway | 8800 | 9095 |
| user-service-rpc | 8080 (gRPC) | 9096 |

### 模块职责概览

- `apps/`: 聚焦体验层。web-client 负责攻略/社区主站；mobile-app 聚合开黑、战绩、活动；mini-program 侧重轻量浏览与小游戏联机；admin-console 提供内容和运营配置。
- `services/`: 领域化微服务。user-service 管账号与社交关系；game-catalog 维护游戏库、榜单及推荐；content-service + community 负责攻略、帖子与互动；matchmaking + realtime-hub 承担实时房间、语音、推送；data-panel 汇聚战绩与版本数据；crawler-jobs 定时抓取外部信息。
- `packages/`: 统一设计系统与类型契约。ui-kit 输出组件、主题；data-models 存放 protobuf/graphql schema 及 TypeScript/Go SDK；shared-utils 提供跨端工具；config 集中构建、Lint、环境模板。
- `platform/`: 与基础设施相关的 IaC、CI/CD、可观测性配置，支持自动化部署、灰度、告警。
- `tools/`: CLI、脚本、代码生成器、数据迁移任务等辅助开发效率。
- `docs/`: 存放产品策划、技术方案、API、运营手册；搭配文档站或知识库。
- `tests/`: 跨服务集成测试、端到端脚本、合规/安全扫描配置。

### 下一步建议

1. 移动端 M3 收尾（详见 `docs/mobile_plan.md`）：Android release 包（EAS 或本地 gradle）与真机闭环验证——本地推送、深链分享卡等功能已落地，均待真机回归。
2. api-gateway BFF 化第二阶段：第一阶段（移动端统一入口，社区/内容/游戏/用户反代 + 分享卡）已落地；后续按需扩聚合端点（列表聚合、字段裁剪），web-client 保持直连各服务不受影响。
3. `docs/architecture/` 补充总体架构与数据流文档（服务边界、网关反代拓扑、移动端直连拓扑的现状与演进）。

### Go-zero 开发约定

- 工具准备：`go install github.com/zeromicro/go-zero/tools/goctl@latest`，并将 `$(go env GOPATH)/bin` 添加到 `PATH`，同样安装 `protoc-gen-go` 与 `protoc-gen-go-grpc`。
- Workspace：根目录维护 `go.work`，把各个服务模块 (`services/user-service`, `services/user-service-rpc`...) 纳入，避免相互引用时走远程依赖。
- 代码生成：使用 `goctl api new <service>`/`goctl rpc new <service>` 搭建骨架，服务内的 API/RPC 定义通过 `*.api`、`*.proto` 维护，执行 `goctl api go`/`goctl rpc protoc` 生成功能代码。
- 配置共享：`packages/config` 下提供 `.env.example`、`golangci-lint` 模板、公共 `make` 目标等，服务中可直接引用或通过 `Makefile` include。

## 新增功能概览

### 已完成的核心服务

#### 1. Game Catalog Service（`services/game-catalog`）
使用 go-zero 构建，内置游戏数据仓库、列表筛选、创建接口以及推荐/精选能力。
- `GET /api/v1/games` - 游戏列表（支持筛选和分页）
- `GET /api/v1/games/:id` - 游戏详情
- `GET /api/v1/games/featured` - 精选游戏
- `GET /api/v1/games/recommendations` - 推荐游戏
- 端口：8890

#### 2. User Service（`services/user-service`）
用户账号、权限、好友、成长体系管理服务。
- `POST /api/v1/auth/login` - 用户登录
- `POST /api/v1/auth/register` - 用户注册
- `GET /users/:id` - 获取用户信息
- `GET /users/:id/recommendations` - 用户游戏推荐（跨服务调用）
- 集成JWT认证中间件
- 端口：8888

#### 3. Content Service（`services/content-service`）✨ 新增
攻略、资讯、CMS、评论管理服务，支持完整的内容创作和社区互动。
- **攻略管理**：
  - `POST /api/v1/guides` - 创建攻略
  - `PUT /api/v1/guides/:id` - 更新攻略
  - `GET /api/v1/guides/:id` - 获取攻略详情
  - `GET /api/v1/guides` - 攻略列表（支持筛选）
  - `POST /api/v1/guides/:id/publish` - 发布攻略
  - `POST /api/v1/guides/:id/like` - 点赞攻略
- **评论系统**：
  - `POST /api/v1/comments` - 发表评论
  - `GET /api/v1/comments` - 评论列表
  - `DELETE /api/v1/comments/:id` - 删除评论
  - `POST /api/v1/comments/:id/like` - 点赞评论
- 支持嵌套评论和回复功能
- 端口：8891

#### 4. API Gateway（`services/api-gateway`）
统一聚合认证、推荐、榜单接口，对外只暴露统一的API。
- `POST /auth/login` - 用户登录
- `GET /games/featured` - 精选游戏
- `GET /users/:id/recommendations` - 用户推荐
- 端口：8800

#### 5. Web Client（`apps/web-client`）✨ 大幅增强
React + Vite + React Router 构建的完整前端应用，支持账号登录、跨服务推荐展示、游戏榜单渲染、**攻略浏览与创作、评论互动**。

**新增核心功能**：
- **多页面路由**：使用 React Router 实现单页应用多页面导航
- **攻略列表页**：浏览所有已发布攻略，支持筛选"全部攻略"和"我的攻略"
- **攻略详情页**：查看完整攻略内容、点赞、阅读统计、标签展示
- **攻略创建/编辑**：富文本编辑器，支持草稿保存和一键发布
- **评论系统**：
  - 发表评论和嵌套回复
  - 点赞评论
  - 删除自己的评论
  - 实时评论数统计
- **权限控制**：未登录用户可浏览，登录后可创建攻略和评论
- **响应式设计**：暗色主题，适配桌面和移动设备

### 技术特性

- **Service-to-Service 调用**：`user-service` 通过 `GameCatalogClient` 拉取游戏推荐，展示跨服务集成能力。
- **测试补全**：为游戏仓库与用户推荐逻辑新增单元测试，覆盖过滤、推荐稳定性与跨服务失败兜底。
- **Docker 支持**：`docker-compose.yaml` 一键拉起所有服务，各服务目录下提供独立 `Dockerfile`。
- **线程安全**：所有服务的内存存储都实现了线程安全的并发控制。

### 快速体验

```bash
# 启动全部服务（需 Docker）
docker compose up --build

# 单独运行服务
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/content-service/content.go -f services/content-service/etc/content-api.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml

# 前端
cd apps/web-client && npm install && npm run dev
```

### 服务端口规划

| 服务 | 端口 | 描述 | 状态 |
|------|------|------|------|
| user-service | 8888 | 用户服务 | ✅ 完成 |
| api-gateway | 8800 | API网关 | ✅ 完成 |
| game-catalog | 8890 | 游戏目录服务 | ✅ 完成 |
| content-service | 8891 | 内容服务（攻略+评论） | ✅ 完成 |
| community | 8892 | 社区服务（帖子+话题） | ✅ MVP |
| web-client | 5173 | Web前端 | ✅ 完成 |
