# tappi

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

### 模块职责概览

- `apps/`: 聚焦体验层。web-client 负责攻略/社区主站；mobile-app 聚合开黑、战绩、活动；mini-program 侧重轻量浏览与小游戏联机；admin-console 提供内容和运营配置。
- `services/`: 领域化微服务。user-service 管账号与社交关系；game-catalog 维护游戏库、榜单及推荐；content-service + community 负责攻略、帖子与互动；matchmaking + realtime-hub 承担实时房间、语音、推送；data-panel 汇聚战绩与版本数据；crawler-jobs 定时抓取外部信息。
- `packages/`: 统一设计系统与类型契约。ui-kit 输出组件、主题；data-models 存放 protobuf/graphql schema 及 TypeScript/Go SDK；shared-utils 提供跨端工具；config 集中构建、Lint、环境模板。
- `platform/`: 与基础设施相关的 IaC、CI/CD、可观测性配置，支持自动化部署、灰度、告警。
- `tools/`: CLI、脚本、代码生成器、数据迁移任务等辅助开发效率。
- `docs/`: 存放产品策划、技术方案、API、运营手册；搭配文档站或知识库。
- `tests/`: 跨服务集成测试、端到端脚本、合规/安全扫描配置。

### 下一步建议

1. `docs/architecture/` 中补充总体架构与数据流，确保团队对领域边界达成共识。
2. 制定 `packages/config/` 的基础工具链（pnpm workspace、tsconfig、eslint、prettier 等），方便后续初始化各 app/service。
3. 先落地核心服务骨架（user-service、game-catalog、content-service），并在 `tests/` 内准备契约测试样板，保障接口演进。

### Go-zero 开发约定

- 工具准备：`go install github.com/zeromicro/go-zero/tools/goctl@latest`，并将 `$(go env GOPATH)/bin` 添加到 `PATH`，同样安装 `protoc-gen-go` 与 `protoc-gen-go-grpc`。
- Workspace：根目录维护 `go.work`，把各个服务模块 (`services/user-service`, `services/user-service-rpc`...) 纳入，避免相互引用时走远程依赖。
- 代码生成：使用 `goctl api new <service>`/`goctl rpc new <service>` 搭建骨架，服务内的 API/RPC 定义通过 `*.api`、`*.proto` 维护，执行 `goctl api go`/`goctl rpc protoc` 生成功能代码。
- 配置共享：`packages/config` 下提供 `.env.example`、`golangci-lint` 模板、公共 `make` 目标等，服务中可直接引用或通过 `Makefile` include。

## 新增功能概览

- **Game Catalog Service**（`services/game-catalog`）：使用 go-zero 构建，内置游戏数据仓库、列表筛选、创建接口以及推荐/精选能力。服务启动后可通过 `GET /games`, `GET /games/:id`, `GET /games/featured`, `GET /games/recommendations` 获取数据。
- **Service-to-Service 调用**：`user-service` 通过 `GameCatalogClient` 拉取 `/games/recommendations`，新增 `GET /users/:id/recommendations` 供前端/其他服务复用。
- **Web Client**（`apps/web-client`）：React + Vite 实现的基础界面，支持账号登录、跨服务推荐展示、游戏榜单渲染。API 地址可通过 `VITE_USER_SERVICE_URL`、`VITE_GAME_SERVICE_URL` 注入。
- **API Gateway**（`services/api-gateway`）：统一聚合认证、推荐、榜单接口，对外只暴露 `POST /auth/login`、`GET /games/featured`、`GET /users/:id/recommendations`，方便前端与未来其他客户端统一接入。
- **测试补全**：为游戏仓库与用户推荐逻辑新增单元测试，覆盖过滤、推荐稳定性与跨服务失败兜底。
- **Docker 支持**：`docker-compose.yaml` 一键拉起 `game-catalog`、`user-service`、`web-client`，各服务目录下提供独立 `Dockerfile`。

快速体验：

```bash
# 启动全部服务（需 Docker）
docker compose up --build

# 单独运行服务
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml

# 前端
cd apps/web-client && npm install && npm run dev
```
