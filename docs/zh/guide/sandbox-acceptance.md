# 联合沙箱与验收（GP-005）

联合沙箱把 Spawn compose 栈与外部 Courier 网关（croupier，冻结仓）接在一起，
一款示例游戏可端到端装配：合作方入驻 → 建游戏 → 版本提审 → 审核 → 发布 →
区服 → Courier 映射 → 支付商品 → 礼包 → 活动 → 玩家侧冒烟。工具集在
[`deploy/sandbox/`](https://github.com/cuihairu/spawn/tree/main/deploy/sandbox)：
环境变量样例、接线 README 与 `seed-example-game.sh` 引导脚本。本页记录版本清单，
以及沙箱内无法满足、必须真实环境承接的验收依赖（真实渠道/游戏服/授权/设备）。

## 版本清单

以 2026-11 仓库固定版本为准；版本移动时以引用文件复核。

| 组件 | 版本 | 来源 |
| --- | --- | --- |
| Go 工具链 | go 1.27.3（模块 go 1.26.0） | `go.work` |
| 服务框架 | go-zero v1.10.3 | `services/*/go.mod` |
| 网关/微服务 | docker compose，镜像由 `services/*` Dockerfile 构建 | `docker-compose.yaml` |
| Web 端 | React ^19.3.0，Vite ^8.3.2 | `apps/web-client/package.json` |
| 移动端 | Expo SDK 57（`expo ~57.0.26`），React Native New Architecture | `apps/mobile-app/package.json` |
| Courier 网关 | croupier（外部冻结仓 [cuihairu/courier](https://github.com/cuihairu/courier)，gRPC 契约） | [手游平台调研](../product/mobile-platform-research.md) 第 4 节 |
| 数据库 | 各服务 SQLite 文件库（`DATASOURCE=file:...`） | compose 环境变量 |

引导脚本产出的沙箱账号：`sandbox-admin`（经 `ADMIN_USER_IDS` 引导为
platform_admin）、`sandbox-partner`、`sandbox-player`，默认密码
`Sandbox!12345678`——仅限本机沙箱；对外暴露前必须改密或清库。

## 沙箱覆盖范围

compose 栈内可跑的部分由 `deploy/sandbox/seed-example-game.sh` 全链路演练，
任何装了 Docker 的机器可复现：

- 管理员引导、合作方授权、游戏/归属创建
- 版本提审 → 过审 → 游戏+版本发布（含草稿期平台托管包体上传）
- 网关受控下载（匿名 401，Bearer 命中包体内容，流式通道）
- 区服创建与玩家预约
- Courier 映射与能力声明（adapter 管理面，本地侧）
- 支付商品配置、礼包领取（单人/全量上限）、倍率活动评估面
- 玩家侧冒烟：活动列表、区服预约、礼包领取

依赖 Courier 的端点（会话票据、代登录、经 teller 下单）在 croupier 实例接入
compose 网络前以 502 呈现——沙箱钉住的是接口契约，不是活的实例。

## 沙箱外的验收依赖

以下验收步骤需要的资产无法由仓库配置铸造。按 GP-005 在此记录，归各自
待办条目把门：

| 依赖 | 阻塞 | 缺什么 |
| --- | --- | --- |
| 真实渠道凭据 | GP-202 真渠道支付 | 渠道商户号/密钥、回调密钥；沙箱 HMAC 密钥不得用于真渠道 |
| 真实游戏服 | GP-206 端到端游玩 | 已接 croupier、沙箱网络可达的游戏会话服 |
| 授权材料 | GP-202/203 生产接线 | 渠道签约文件 / 生产网关凭据（teller Deliver） |
| 真机设备 | GP-105 Android 安装链路 | Android 真机或模拟器走 APK 下载→安装→启动；SHA-256 包体校验需原生文件哈希模块（expo-crypto SDK 57 无流式摘要接口） |
| 发布拍板 | 正式版本发布 | 按仓库红线，验收通过且用户明确给出版本号后才可发版 |

涉及上表的验收项归 GP-105/GP-202/GP-206 的验收范围，在本页只记录不展开。

## 验收清单

`seed-example-game.sh` 跑通后，在全新沙箱上逐项核验。1/2 由 seed 脚本内置断言（草稿期上传包体，网关可见性、匿名 401、Bearer 下载内容一致）；1/2/3/5 已于 2026-11-21 本机沙箱实测（3：单人第 3 次 409、全量领完 409、同 request_id 重放返回同领取 id；5：无 croupier 时会话端点 502）；4/6 于 2026-11-23 用 courier-gateway-integrator 活走查（下单经 Courier `POST /v1/payments/orders`，回调经 spawn `/callback/sandbox` 验签转发）。

- [x] 示例游戏玩家可见（经网关 8800 `GET /games/:id`）
- [x] 已发布版本可经网关流式路由下载（需 Bearer）
- [x] 礼包领取遵守单人/全量上限（领完 409，幂等重放优先）
- [x] 支付订单触发活动奖励评估（payments → promotions S2S）——2026-11-23 活走查（courier-gateway-integrator）：PAID 后 facts 入队 `promotion_evaluations` done → `POST /internal/reward-evaluations` 200 → `reward_grants` granted（rate_bp 15000 倍率，extra 50）
- [x] Courier 会话端点：无 croupier 时 502；有 croupier 时票据链路走通归 GP-206
- [x] 沙箱支付回调（`POST /callback/sandbox/...`）以合法 HMAC 将订单推至 PAID——2026-11-23 活走查：HMAC 验签后按订单投影解析 scope 转发（order_01a2064c… CREATED → PAID → 发货账本 DELIVERED）
