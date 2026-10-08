<div align="center">
  <img src="docs/public/logo.svg" alt="spawn logo" width="64" />
  <h1>spawn</h1>
  <p>游戏社区平台：攻略、榜单与社区</p>
  <p><a href="README.md">English</a> | <a href="README.zh.md">简体中文</a></p>
</div>

## 下载与安装

[nightly 滚动 Release](https://github.com/cuihairu/spawn/releases/tag/nightly)（每日自动构建，tag 固定 `nightly`，清旧传新）提供现成产物：

| 产物 | 内容 |
| --- | --- |
| `spawn-<服务名>-linux-amd64` ×6 | 六个后端服务二进制（CGO 构建，内嵌 SQLite 驱动），下载后 `chmod +x` 直接运行，配置在仓库各服务 `etc/` 目录，`DATASOURCE` 注入 MySQL 连接串可切换存储 |
| `spawn-web-client-linux-amd64.tar.gz` | Web 主站静态产物，解压后任意静态服务器可托管 |
| `SHA256SUMS.txt` | 全部资产校验和，同目录 `sha256sum -c SHA256SUMS.txt` 核对 |

移动端 APK 与服务容器镜像不在 nightly 交付面（Android 出包依赖本地 gradle 流程；镜像按 `services/*/Dockerfile` 自行构建）。正式版本尚未发布；文档站见 <https://cuihairu.github.io/spawn/>。

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

> 上树是规划图。落地状态以 `services/README.md`、`apps/README.md` 为准：六服务与 web-client、
> mobile-app 有代码，matchmaking/realtime-hub/data-panel/crawler-jobs、mini-program、admin-console 尚未创建。

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

1. 移动端 M3 收尾（详见 `docs/mobile_plan.md`）：Android release 本地 gradle 出包已落地（2026-10-04：`android.package` 配置 + `build:android:release` 脚本，aapt/apksigner 静态走查通过）；剩真机安装与功能回归，本地推送、深链分享卡都还没在真机上验过。
2. api-gateway BFF 第二阶段已落地（2026-10-04）：`GET /home/feed` 列表聚合端点（精选游戏/热帖/话题/攻略一次拉齐，逐组字段裁剪 + 单上游故障降级）+ mobile `fetchHomeFeed` 消费点预留；web-client 保持直连各服务不受影响。后续按需扩更多聚合口径。
3. `docs/architecture/` 总体架构与数据流文档已补齐（2026-10-04）：`topology.md` 记录现状（服务边界/端口/网关反代路由面/认证数据流/演进路线），`overview.md` 保持愿景层；后续架构变更随切片同步更新现状文档。

### Go-zero 开发约定

- 工具准备：`go install github.com/zeromicro/go-zero/tools/goctl@latest`，并将 `$(go env GOPATH)/bin` 添加到 `PATH`，同样安装 `protoc-gen-go` 与 `protoc-gen-go-grpc`。
- Workspace：根目录维护 `go.work`，把各个服务模块 (`services/user-service`, `services/user-service-rpc`...) 纳入，避免相互引用时走远程依赖。
- 代码生成：使用 `goctl api new <service>`/`goctl rpc new <service>` 搭建骨架，服务内的 API/RPC 定义通过 `*.api`、`*.proto` 维护，执行 `goctl api go`/`goctl rpc protoc` 生成功能代码。
- 配置共享：`packages/config` 下提供 `.env.example`、`golangci-lint` 模板、公共 `make` 目标等，服务中可直接引用或通过 `Makefile` include。

## 服务与前端现状

#### 1. Game Catalog Service（`services/game-catalog`）
使用 go-zero 构建，SQLite/MySQL 双驱动存储（默认本地 SQLite + 内嵌种子数据）、列表筛选、创建接口（JWT 保护）以及推荐/精选能力。
- `GET /games` - 游戏列表（支持筛选和分页）
- `GET /games/:id` - 游戏详情
- `GET /games/featured` - 精选游戏
- `GET /games/recommendations` - 推荐游戏
- 端口：8890

#### 2. User Service（`services/user-service`）
用户账号、权限、好友、成长体系管理服务，JWT 签发方。
- `POST /auth/login` - 用户登录
- `POST /auth/register` - 用户注册
- `GET /users/:id` - 获取用户信息
- `GET /users/:id/recommendations` - 用户游戏推荐（跨服务调用 game-catalog）
- 全局 JWT 校验中间件（白名单注册/登录等公开路径）
- 端口：8888

#### 3. Content Service（`services/content-service`）
攻略、资讯、评论服务，go-zero 实现，SQLite/MySQL 双驱动存储。
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
BFF 聚合 + 四路反代的统一接入层，对外只暴露统一的 API（mobile-app 全量调用面经此单地址）。
- `POST /auth/login` - 用户登录（透传 user-service）
- `GET /games/featured` - 精选游戏
- `GET /users/:id/recommendations` - 用户推荐
- `GET /home/feed` - 首页列表聚合（精选游戏/热帖/话题/攻略四路并发，字段裁剪 + 单上游故障分组降级）
- `GET /s/p/:id` - 帖子分享卡跳板页（og meta + `spawn://` 深链 + Web 入口）
- 反代：community（帖子/话题/关注/点赞）、content（攻略/评论）、users（注册/资料）、games（列表/详情）全量路由
- 端口：8800

#### 5. Web Client（`apps/web-client`）
React 19 + Vite + React Router 7 前端。七个页面：首页（登录与推荐）、攻略列表、攻略详情、
攻略创建/编辑、帖子流、帖子详情。攻略支持「全部/我的」筛选和草稿、发布两种保存态；评论支持
嵌套回复、点赞，作者可删自己的评论；社区页直连 community 服务，帖子可点赞、分享。未登录能浏览，
写操作要登录。暗色主题，响应式布局。

### 技术特性

跨服务调用走 HTTP：user-service 经 `GameCatalogClient` 调 game-catalog 拿游戏数据拼推荐，
失败降级兜底；content-service 调 game-catalog 取游戏标题带熔断、重试和 Prometheus 指标。
游戏仓库与用户推荐逻辑有单元测试。`docker-compose.yaml` 一条命令拉起全部服务，各服务目录
有独立 `Dockerfile`。六服务数据面均已切 SQLite/MySQL 双驱动（DSN 含 `file:` 走 SQLite，
生产注入 MySQL 连接串），进程内 TTL+LRU 读缓存，并发安全由连接池钳制与缓存自身保证。

### 快速体验

```bash
# 启动全部服务（需 Docker）
docker compose up --build

# 单独运行服务
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/content-service/content.go -f services/content-service/etc/content-api.yaml
go run services/community/community.go -f services/community/etc/community-api.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml

# 前端
cd apps/web-client && npm install && npm run dev
```

### 服务端口规划

| 服务 | 端口 | 描述 | 状态 |
|------|------|------|------|
| user-service | 8888 | 用户服务 | 已完成 |
| api-gateway | 8800 | API网关（BFF 聚合+反代） | 已完成 |
| game-catalog | 8890 | 游戏目录服务 | 已完成 |
| content-service | 8891 | 内容服务（攻略+评论） | 已完成 |
| community | 8892 | 社区服务（帖子/话题/关注/点赞） | 已完成 |
| user-service-rpc | 8080 (gRPC) | 用户服务 RPC（集群内部） | 已完成 |
| web-client | 5173 | Web前端 | 已完成 |
| mobile-app | — | React Native（Expo）客户端 | 实施中（见 `docs/mobile_plan.md`） |
