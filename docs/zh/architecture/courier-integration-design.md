# 手游平台接入设计（M0：Courier 复用边界与基础治理）

> 日期：2026-11-01。范围：GP-001 / GP-002 / GP-003 的接入设计与边界结论，对应 [手游平台调研](../product/mobile-platform-research.md) 第 4 节「接入前必须写清的边界」1–7。本文是接入设计，**不代表下列功能已经实现或通过上线验收**；行为性验收（跨游戏 token 被拒、scope 不入 URL 等）由两仓契约内建与联合回归（GP-206）执行，此处给出设计结论、映射结构、隔离用例清单与证据引用。

依据来源（本轮重新静态核对，非引用未验证结论）：

- Spawn：`services/user-service/utils/auth.go`（JWTClaims{UserId, Username}、HS256、GenerateToken/ParseToken）、`internal/logic/loginlogic.go`（登录签发平台 token）、`middleware/auth.go`（解析 → ctx user_id/username）、`services/game-catalog/game.api`（Game{Id string, Tags []string}、ListGamesRequest{Tag form} 展示筛选）。
- Courier：`docs/zh/contract/scope.md`（Frozen v1：scope=game_id+env 全局唯一、仅 header、token 绑定 scope、A-scope token 访问 B-scope → SCOPE_MISMATCH、account 公司级 / player 游戏级）、`docs/zh/contract/auth.md`（Frozen v1.1：邮箱+密码/游客、会话按 scope 签发、access 15min/refresh 30d 缺省、注销宽限期）、`docs/zh/contract/payment.md`（Frozen v1.1：teller 默认 Provider /v1/payments/*、服务端定价、回调 HMAC-SHA256、幂等受理、订单归属 404、断单停 PAID 可对账、push 事件）、`gateway/providers/account/account.go`（AccountProvider + Verifier + /v1/identity/*）。

## 1. GP-001 权威账号源与账号绑定

### 1.1 决策

- **权威账号源 = Spawn 平台账号（user-service）**。依据：已确定产品约定「统一游戏库、账号入口和社区，不另建平台项目」；Spawn 平台会话是无 scope 的 HS256 JWT（`{user_id, username}`），是平台侧唯一身份。
- **不假设 JWT 互通**。Spawn 平台 token 无 scope、不能作为 Courier 游戏会话；Courier 会话绑定 `game_id+env`（scope.md 规则 3）。Spawn 只**代登录**，不重签、不旁路 Courier 会话。
- **绑定一对一**：一个 Spawn 用户对应一个 Courier 公司级 account；Courier account 是跨游戏实体（scope.md「与账号模型的关系」），同一账号在每游戏各自登录产出各自 scope 会话——「一个账号登录所有游戏」指账号可复用，不是 token 可跨游戏。

### 1.2 绑定表（Spawn 平台侧）

```sql
-- SQLite 首建；MySQL 回落同社区惯例（BIGINT/BIGINT AUTO_INCREMENT、utf8mb4、uk 唯一键）
CREATE TABLE IF NOT EXISTS platform_courier_binding (
  spawn_user_id      INTEGER PRIMARY KEY,
  courier_account_id VARCHAR(64) NOT NULL,
  courier_email      VARCHAR(255) NOT NULL DEFAULT '',
  status             VARCHAR(16)  NOT NULL DEFAULT 'ACTIVE', -- ACTIVE / UNBOUND / PENDING_DELETE
  bind_method        VARCHAR(16)  NOT NULL DEFAULT 'auto',   -- auto / explicit
  bound_at           VARCHAR(32)  NOT NULL,
  unbound_at         VARCHAR(32)  NOT NULL DEFAULT '',
  created_at         VARCHAR(32)  NOT NULL,
  updated_at         VARCHAR(32)  NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_binding_courier ON platform_courier_binding (courier_account_id);
```

- 主键 = spawn_user_id；`courier_account_id` 也唯一（一对一）。
- `status` 镜像 Courier 账号生命周期；`bind_method` 区分自动静默建号与玩家显式绑定；`bound_at/unbound_at` 满足「迁移/解除绑定可追踪」。
- 写库与审计联动：绑定/解绑/注销均写 GP-004 审计记录（actor、action、前后值），不在本表冗余。

### 1.3 游戏级会话流（登录 → 验票 → 退出 → 注销）

1. 玩家已在 Spawn 登录（持平台 JWT `user_id`），在游戏详情点「进入游戏」，携带 `spawn_game_id`。
2. Spawn 平台适配服务校验平台 JWT → 查 GP-002 映射表得 `courier_game_id + env`。
3. 查绑定表：无绑定 → 以 `spawn_user_id` 为稳定外部键**静默创建** Courier 账号（`bind_method=auto`，email 用平台绑定邮箱或 `spawn_<user_id>@platform.invalid` 占位，密码随机不落 Spawn 明文），写绑定表；已绑定 → 复用。
4. 用 Courier 账号凭证经 `POST /v1/identity/login` **代登录**，scope = 该 `game_id+env` → 取得该 scope 的 accessToken/refreshToken。
5. 下发**游戏启动票据**给 SDK（短期、一次性、绑定 game_id，游戏内用它经 SDK 换 scope-bound token；不在 Spawn 落 Courier token 明文）。
6. 游戏服/网关验票：由 **Courier 自身**校验 scope-bound token（Verifier；access 15min、refresh 轮换，auth.md）。Spawn 不验 Courier token。
7. **跨游戏被拒**：同一用户进 B 游戏 → 重复 4–5 用 B 的 scope；A scope 的 token 访问 B 资源 → `SCOPE_MISMATCH`（scope.md 内建）。Spawn 侧只保证「按 game 分发对应 scope 会话、不串发」，不自行实现跨游戏校验（那是 Courier 契约职责）。
8. **退出**：Spawn 登出 = 吊销平台会话（现有行为）；游戏会话登出 = 调 `POST /v1/identity/logout`（scope-bound，按游戏）。平台会话与游戏会话生命周期分离（游戏会话短效 15min，不随平台登出强撤）。
9. **注销**：Spawn 侧注销 → 同步调 Courier `POST /v1/identity/account/delete`（EMAIL 宽限期 / GUEST 立即；受理即吊销全部会话，auth.md v1.1）→ 绑定表 `status=PENDING_DELETE`；宽限期撤销（`cancelToken`）→ 回 `ACTIVE`；到期 `DELETED` → 绑定表 `status=UNBOUND` 并保留审计。双写幂等，任一失败仅记日志不中断主流程（社区同款契约）。

### 1.4 验收对照（GP-001）

| 验收 | 如何满足 |
| --- | --- |
| 跨游戏 token 被拒 | Courier scope.md 内建（SCOPE_MISMATCH）；Spawn 交付按 scope 分发会话的适配 + 记入联合回归用例（GP-206） |
| 账号迁移/解除绑定可追踪 | 绑定表 status/bound_at/unbound_at + GP-004 审计日志（actor/action/前后值） |

## 2. GP-002 目录映射与能力配置

### 2.1 映射配置表（Spawn 平台侧，独立于标签）

```sql
CREATE TABLE IF NOT EXISTS courier_game_mapping (
  spawn_game_id  VARCHAR(64) PRIMARY KEY,
  courier_game_id VARCHAR(64) NOT NULL,
  env            VARCHAR(16)  NOT NULL DEFAULT 'prod',
  partner_id     INTEGER      NOT NULL,
  capabilities   TEXT         NOT NULL DEFAULT '{"login":true,"payment":false,"download":false}',
  created_at     VARCHAR(32)  NOT NULL,
  updated_at     VARCHAR(32)  NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_mapping_scope ON courier_game_mapping (courier_game_id, env);
```

- `spawn_game_id ↔ (courier_game_id, env)` 一一映射；`partner_id` 记录合作方归属；`capabilities` JSON：`login`（需代登录进游戏）、`payment`（接支付）、`download`（平台包体下载）——能力按需开通，与标签解耦。
- 区服/角色映射：Courier archivist 已有按 scope 的账号↔角色映射（调研 §4），Spawn 不重建；区服目录与运营事件字段补差归 GP-201（M2）。本轮只定义承载：适配服务把 `courier_game_id + env` 沿 header 传下。

### 2.2 标签只作展示

- 「联运」仅写入 game-catalog 的 `Tags []string`（game.api），影响前端筛选/徽标；**不读取** capabilities，不自动开通支付/下载。
- 两条写入路径互不相交：标签写入走游戏资料维护；capabilities 写入走运营映射表。设计上无联动（代码评审点：CreateGame/资料更新不得触碰 `courier_game_mapping`）。

### 2.3 scope 只走 header

- 适配服务调 Courier 时仅在 header 注入 `X-Courier-Game-Id` / `X-Courier-Env`；Courier URL path/payload 一律不携带 scope（scope.md 传输节）。契约已冻结，适配服务只做 header 注入，不旁路、不向 URL 塞隔离字段。

### 2.4 验收对照（GP-002）

| 验收 | 如何满足 |
| --- | --- |
| 标签变化不自动开通支付/下载 | 标签与 capabilities 分表、无联动（§2.2） |
| scope 不进入 Courier URL/payload | scope.md 契约 + 适配服务 header-only（§2.3），记入联合回归用例 |

## 3. GP-003 交易事实源与版本兼容

### 3.1 决策：事实源 = 所选 PaymentsProvider

- 交易唯一事实源 = **teller（或接的真实渠道适配）**。teller 持有 sku/order 事实：服务端定价（客户端请求体只有 `skuId`）、回调 `X-Payment-Signature` HMAC-SHA256 验签先于解析、同订单重复回调幂等 200、非本人订单 404、发货失败停 `PAID` 可对账（payment.md v1.1 安全边界 1–6）。状态机 `CREATED → PAID → DELIVERED`（→ `CLOSED`）由 Courier 维护。
- **Spawn 不复制订单状态机**。Spawn 后台经管理适配只读投影（订单 id 为键的事实副本，如经营分析/账单输入），或直接经 Provider 接缝读取；不维护第二套可独立推进的状态机。
- **管理面归属 Courier**。现 Reconcile 是进程内方法，无 SKU 管理/补单/退款/全局订单查询/结算的 HTTP 管理 API（调研 §4 边界 6）。**管理适配作为 Courier 提案**（Provider 接口扩展或独立管理 Provider），Spawn 经受权限保护的管理适配读取，不旁路契约。若确需在 Spawn 建只读投影，仅存事实副本。

### 3.2 多游戏隔离用例清单（写清边界，由 GP-206 联合沙箱执行）

1. **跨游戏 SKU 越权**：A 游戏 scope 下单 B 游戏 SKU → 拒绝（`PAYMENT_SKU_NOT_FOUND` 或 scope 校验）。
2. **跨厂商订单越权**：合作方 A 账号读合作方 B 订单 → 404（订单归属校验，不泄露存在性）。
3. **回调密钥隔离**：A 渠道密钥伪造回调推进 B 订单 → 403 `PAYMENT_INVALID_SIGNATURE`。
4. **存储实例隔离**：每 scope 独立 Store/StateFile 或隔离键，重启恢复不串单。
5. **scope 校验**：A-scope token 访问 B-scope 支付资源 → `SCOPE_MISMATCH`。
6. **断单恢复**：发货失败订单停 `PAID`，管理面对账重发，不二次发货。

### 3.3 跨仓版本兼容规则

- Courier 契约 Frozen：只加不改，破坏性变更升版本（versioning.md）。Spawn 依赖冻结契约版本（scope v1 / auth v1.1 / payment v1.1）。
- 跨仓回归钉契约版本 + 一致性测试（Courier 已有 `gateway/e2e/payments_conformance_test.go`）；Spawn 侧不 fork 契约、不改 SDK 协议。
- 新需求（如订单携带区服/角色/活动奖励、退款端点）**先回 Courier 提案**做兼容扩展，不在 Spawn 旁路改造（调研 §4 边界 7）。

### 3.4 验收对照（GP-003）

| 验收 | 如何满足 |
| --- | --- |
| 事实源唯一、不复制状态机 | §3.1 决策；Spawn 只读投影/Provider 接缝 |
| 管理 API/事件接口归属明确 | 管理适配回 Courier 提案（§3.1）；push 事件 `payment.paid/payment.delivered` 为订阅口 |
| 多游戏隔离 | §3.2 用例清单（GP-206 执行） |
| 版本兼容 | §3.3 冻结契约 + 钉版本 + conformance |

## 4. GP-004 / GP-005 设计铺垫（实现待后续里程碑）

- **最小权限/审计模型（设计）**：角色 `platform_admin / operator / partner / player`；资源=游戏/版本/活动/订单；游戏归属（partner 只达自有游戏）；审计表（actor_id、actor_role、action、resource_type、resource_id、before_json、after_json、ip、created_at）。**最小实现归 GP-103**（运营后台）与 Courier 管理适配（GP-204）；本设计定模型与写入点（绑定/映射/能力变更均写审计）。
- **存储与备份**：Spawn 数据按各服务现有持久化；包体存储与受控下载方案归 GP-105；Courier OrderStore/StateFile 为可选快照接缝——**生产必须接持久存储**，默认内存不用于生产（调研 §4 F77），具体部署与备份恢复随 GP-105 / GP-206 验收。
- **持久化与备份恢复方案（GP-004）**：以各服务「存储现状」（`topology.md` §1 列）为基线：
  - *存储形态*：user-service 用 MySQL；game-catalog / content-service / community / data-panel 默认 SQLite 文件（`file:data/*.db`，生产以 `DATASOURCE` 指向 MySQL）；api-gateway 图片走本地 uploads 卷；realtime-hub 可选 Redis（跨实例 pub/sub）。无外部托管数据库依赖（除部署者自管 MySQL）。
  - *备份策略*：SQLite 用 `VACUUM INTO` 在线快照（或停机文件拷贝）；MySQL 用 `mysqldump --single-transaction` 逻辑备份；uploads 卷用 rsync/restic 增量到异机/对象存储；Redis 开 RDB+AOF。备份任务以 cron/systemd timer 每日低峰执行。
  - *恢复与验证*：恢复 = 还原备份到空库 → 启动烟测（关键表 `SELECT COUNT(*)` + `GET /games`、`GET /api/v1/posts` 冒烟）→ 通过后切流。季度性恢复演练记录结果。
  - *目标（未验收前为设计目标，不虚标完成）*：RPO ≤ 24h（每日备份）、RTO ≤ 1h（含恢复+冒烟）；演练实际达标并记录在案方算「已验证」。
  - *生产接入要求（F28）*：`JWT_SECRET` 环境变量覆盖（禁用内置演示密钥，启动命中大声告警）；生产 `DATASOURCE` 走 MySQL（SQLite 仅限本地开发/测试）；uploads 挂持久卷；备份目标置于异机/对象存储。
- **联合沙箱（GP-005）**：清单——Spawn + Courier 联合 docker compose 部署、两仓 commit 版本钉定、一款示例游戏（Courier `examples/` 引擎样本 + 游戏服发货 stub）、真实依赖记录（支付商户凭据、游戏服发货/退款接口、授权与素材、Android 真机）。**实际搭建与跨仓回归归 GP-005 / GP-206**。

## 5. 里程碑映射与后续

- 本设计交付 M0 验收口径（调研 §6）：「有接口与数据映射、隔离用例、联合沙箱部署方案，无新建重复 SDK 任务」——GP-001/002/003 设计完成；GP-004 的权限/审计模型与存储方案已设计，**最小实现随 GP-103 后台落地**；GP-005 联合沙箱清单已列，**实际搭建归 GP-005 / GP-206**。
- 后续执行顺序不变：M1（GP-101 联运标签筛选 → GP-102 资料 → GP-103 后台 → GP-104 版本 → GP-105 下载）→ M2（GP-201…206 依本设计）→ M3 / M4。
- 本设计不引入任何新 Spawn SDK、不复制 Courier 契约/订单状态机。
