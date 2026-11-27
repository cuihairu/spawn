# GP-005 联合沙箱（Spawn + Courier）

Spawn 平台栈 + Courier（croupier，外部冻结仓）联运沙箱的装配配置与引导工具。
目标：一套 compose 栈 + 一个引导脚本 = 一款示例游戏的端到端资产（建游戏 →
提审 → 过审 → 发布 → 区服 → Courier 映射 → 支付商品 → 礼包 → 活动 → 玩家冒烟）。

## 快速开始

```bash
# 1. 配置（含管理员引导与密钥；sandbox.env 勿提交）
cp deploy/sandbox/sandbox.env.example sandbox.env
#    ADMIN_USER_IDS=1：全新沙箱中第一个注册的用户将被写为 platform_admin。

# 2. 起栈（必须显式 --build）
docker compose --env-file sandbox.env up -d --build
#    不加 --build 时 compose 会复用本机已存在镜像；跨版本更新后本机可能是旧镜像，
#    新管理面路由会以 404 静默失效（例如 POST /admin/partners），表现为「代码有、
#    接口没有」。沙箱装配务必每次显式构建。
#
#    另一个坑：镜像重建后若环境变量未变，up -d 只会报 "Container Running"，
#    不会把新镜像换进容器，路由依旧 404。改了服务代码后必须：
#    docker compose --env-file sandbox.env up -d --force-recreate <服务>

# 3. 装配示例游戏（注册管理员/合作方/玩家并打通全链路）
./deploy/sandbox/seed-example-game.sh
```

引导脚本行为：注册 `sandbox-admin` / `sandbox-partner` / `sandbox-player` 三个账号
（密码默认 `Sandbox!12345678`），完成合作方授权、游戏创建、版本提审/过审/发布、
区服、Courier 映射与能力声明、支付商品、礼包（per_user 2 / total 5）、倍率活动，
最后以玩家身份冒烟（活动列表 / 区服预约 / 礼包领取）。任何一步失败即中止并回显
响应体。可重复执行：每次运行产生一组新资产（游戏标题带 `SEED_TAG` 时间戳）。

## 服务端口

| 服务 | 业务端口 | 说明 |
| --- | --- | --- |
| api-gateway | 8800 | 玩家侧/只读运营面统一入口（/metrics 9095） |
| user-service | 8888 | 注册/登录（JWT 签发方） |
| game-catalog | 8890 | 游戏目录/版本/区服/审核（/metrics 9092） |
| content-service | 8891 | 攻略内容（/metrics 9093） |
| community | 8892 | 社区帖子/评论（/metrics 9094） |
| realtime-hub | 8893 | WS 实时推送 |
| data-panel | 8896 | 战绩/漏斗分析 |
| courier-adapter | 8897 | 平台↔Courier 绑定/映射/会话票据（/metrics 9098） |
| payments | 8898 | 商品/下单/订单/沙箱渠道回调（/metrics 9099） |
| promotions | 8902 | 活动/礼包/奖励评估（/metrics 9103） |
| web-client | 4173 | React Web 端 |

沙箱引导脚本直连各服务业务端口（`SP_*` 环境变量可覆盖）：管理面创建路由
（如 `POST /games`）不经 api-gateway 代理，网关只反代玩家侧与只读运营面。

## 管理员引导（ADMIN_USER_IDS）

`game-catalog` 读取 `ADMIN_USER_IDS`（user-service 账号 id，逗号分隔），启动期
幂等写入 `platform_admin` 角色——运营后台的首个入口。沙箱默认 `1`（第一个注册
用户）。若管理员账号不是第一个注册者，先注册拿到 id 再改写重启：

```bash
ADMIN_USER_IDS=<id> docker compose --env-file sandbox.env up -d game-catalog
```

生产环境必须改为真实运营账号 id，或留空禁用引导。

## Courier 接线（外部冻结仓）

承载 Spawn 依赖的冻结 `/v1/*` 契约（identity/payments/cdk/support）的是
**`github.com/cuihairu/courier` 的 gateway 二进制**（模块 `.../courier/gateway`，
入口 `gateway/cmd/gateway`；默认路由表 identity→account、payments→teller、
support→croupier、cdk→voucher，可经 `COURIER_GATEWAY_CONFIG` 换主实现）。
监听 `COURIER_GATEWAY_ADDR`（缺省 `:8080`）；teller 渠道回调密钥
`COURIER_PAYMENTS_CHANNEL_SECRET`、订单状态快照 `COURIER_PAYMENTS_STATE`
（缺省纯内存）。

注意：`ghcr.io/cuihairu/croupier-server:main` **不是**该网关——它是
`/api/v1/*` 的运维平台（incidents/cicd/hotpatches/dbmon 等 661 条路由），
不提供 identity/payments/cdk 契约域，接它无法通过 Spawn 侧契约调用。

接线方式（compose 默认 `COURIER_GATEWAY_URL=http://courier-gateway:8080`，
故容器名须为 `courier-gateway` 且监听 8080）：

1. 构建 gateway：`cd <courier>/gateway && CGO_ENABLED=0 go build -o /tmp/courier-gateway ./cmd/gateway`
   （go.mod 要求 1.27.4，本机 GOTOOLCHAIN=auto 自动取）。
2. 打成最小镜像（alpine 基 + 二进制）并起名为 `courier-gateway` 的容器接入网络：
   `docker run -d --name courier-gateway --network spawn_default \
     -e COURIER_PAYMENTS_CHANNEL_SECRET=... -e COURIER_PAYMENTS_STATE=/tmp/state.json \
     <image>`。
3. 映射与能力声明走 courier-adapter 管理面（引导脚本已覆盖）：
   `PUT /admin/games/:id/mapping {courierGameId, env, partnerId, capabilities}`。

**env 注册表**：gateway 的 env 注册表缺省仅 `{dev,staging,prod}`，`sandbox`
未注册（scope 中间件对未注册 env 返 `COMMON_INVALID_ARGUMENT`）。引导脚本
映射钉的是 `env=sandbox`，直接对接活网关会被拒；活走查须先把映射 env 改为
`dev`/`staging`/`prod`，或按注册表制在接入方启动期注册 `sandbox`
（属 Courier 侧扩展口径，不在本沙箱改动）。

未接 courier gateway 时沙箱照常可用：courier-adapter/payments 能启动，游戏会话票据/
代登录/下单等触达 Courier 的端点以 502 呈现上游不可达；映射/商品/礼包/活动等
本地管理面与玩家冒烟不受影响。

### 支付链路装配（courier-gateway-integrator）

冻结 stock 网关二进制（`gateway/cmd/gateway`）跑不通支付链路，缺两处只能
**装配期**给的接线：teller 价格表只经 Go 方法 `AddSku` 登记（stock 启动为空表
→ 下单恒 `PAYMENT_SKU_NOT_FOUND`；2026-11-26 起 courier v1.2 已补 HTTP 管理端点
`/v1/payments/admin/skus`，Spawn 侧对应 `POST /admin/games/:gameId/products/sync`
把 `payment_products` 运行期新建商品登记/改价到 teller，不必重排此 env）；发货钩子
`teller.Options.Deliver` 未接线 = 记账即发货成功（不通知 Spawn）。GP-203 注记的合法路径是「接入方构建
嵌入 teller 的二进制」，即本目录的独立 Go 模块
`deploy/sandbox/courier-gateway-integrator/`：只组合 courier 的导出包
（providers/account + providers/teller + auth/middleware/routing），不改 courier 仓。

```bash
# SKUs 的 id/金额/币种须与 payments payment_products 对齐（否则金额漂移 409）
COURIER_PAYMENTS_CHANNEL_SECRET=sandbox-channel-secret-2026 \
COURIER_PAYMENTS_DELIVER_WEBHOOK=http://payments:8898/internal/deliveries \
COURIER_PAYMENTS_DELIVER_SECRET=<同 payments 的 PAYMENTS_DELIVERY_SECRET> \
COURIER_TELLER_SKUS='[{"id":"<payment_products.id>","productId":"<同左>",
  "form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}]' \
./deploy/sandbox/build-courier-gateway.sh
```

脚本 go build → docker build（alpine 基）→ 替换容器名 `courier-gateway` 接入
`spawn_default`。模块钉 courier/gateway 的 commit 伪版本（冻结契约），
不入 spawn go.work（toolchain 隔离），CI 不构建此沙箱工具。

**已验证链路（2026-11-23 活栈走查）**：guest 登录取 Courier opaque token →
玩家下单（平台 JWT + `X-Courier-Authorization` 双带）→ CREATED →
渠道回调 `/v1/payments/callback`（`X-Payment-Signature`）→ PAID → Deliver
钩子 POST payments `/internal/deliveries` → dispatcher 按发货账本 `server_id`
调游戏服发货接口（200）→ **DELIVERED**。走查时另需：

- 下单体带 `server_id` 指向该游戏的 **open** 区服（发货账本目标；关服未开
  → `区服不存在或未开放`），并在 payments `PUT /admin/games/:id/delivery`
  配置该游戏发货接口，否则停 PAID 等对账。沙箱无真实游戏服时，该接口指任一
  「POST 恒 200」的桩即可（宿主 python3 写几十行最小 handler；
  `python3 -m http.server` 不行——POST 返 501）；桩返非 200 时 dispatcher
  记账 failed 并按退避重投，账本状态在 `GET /admin/deliveries` 可查。
- 网关 scope 中间件先于 auth 生效：`/v1/payments/*` **含 callback** 一律要
  `X-Courier-Game-Id` + `X-Courier-Env`，缺则 `COMMON_INVALID_ARGUMENT`。
- 直连网关取 token/查单时 Courier 令牌走标准 `Authorization: Bearer` 头；
  `X-Courier-Authorization` 是客户端→Spawn payments 的头（Spawn 才转成
  `Authorization` 透传给 Courier）。

### 回调面（Courier → Spawn）

- teller Deliver 发货 webhook：`POST /internal/deliveries`（payments，
  `X-Delivery-Signature` HMAC，密钥 `PAYMENTS_DELIVERY_SECRET`，
  未配置 = 分发 fail-closed 全 403）。沙箱联调发货必须显式给值。
  出站向游戏服发货则用 `X-Game-Delivery-Signature`（配置接口自带的密钥）。
- 沙箱渠道支付回调：`POST /callback/sandbox/...`（payments，HMAC
  `PAYMENTS_SANDBOX_SECRET`，仅开发/沙箱）。payments 验签后转发 Courier
  callback 时**按订单投影解析 scope 并带头**（orderId → 投影 game_id →
  映射 courierGameId/env）：网关 scope 中间件对 callback 同样强制要求
  这两个头，裸转发必 400（spawn 侧表现 502「转发支付回调失败」）。
  解析不到（投影缺失/映射未配）即 502 fail-closed，绝不推到错误 scope。
- 订单事实 S2S：payments `GET /internal/orders/:id`，`X-Internal-Token`
  （`INTERNAL_TOKEN`，与 courier-adapter 同值，供售后工单归属校验）。

## 密钥与安全边界

`sandbox.env.example` 全部为仓库内置演示值，仅限本机沙箱；生产必须逐项覆盖
（JWT_SECRET / CREDENTIAL_KEY / PAYMENTS_SANDBOX_SECRET / PROMOTIONS_REWARD_SECRET
/ INTERNAL_TOKEN / PAYMENTS_DELIVERY_SECRET）。服务对内置演示密钥启动期告警。
真实渠道密钥/商户凭据不在沙箱范围（GP-202 红线：无凭据不接真渠道）。

## 版本清单与依赖记录

组件版本与「真实渠道/游戏服/授权/设备」依赖记录见
[沙箱验收指南](https://github.com/cuihairu/spawn/blob/main/docs/guide/sandbox-acceptance.md)（docs/guide/sandbox-acceptance.md，中文版 docs/zh/guide/sandbox-acceptance.md）。
