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

Courier 网关（croupier）不在本仓，镜像 `ghcr.io/cuihairu/croupier-server:main`，
gRPC 网关契约冻结。接线方式：

1. 起 croupier 实例（自备/自行编排），加入 Spawn 的 compose 网络：
   `docker network connect spawn_default <croupier容器>`。
2. `sandbox.env` 中设 `COURIER_GATEWAY_URL=http://courier-gateway:8080`
   （compose 默认值，croupier 侧容器名需对齐）。
3. 映射与能力声明走 courier-adapter 管理面（引导脚本已覆盖）：
   `PUT /admin/games/:id/mapping {courierGameId, env, partnerId, capabilities}`。

未接 croupier 时沙箱照常可用：courier-adapter/payments 能启动，游戏会话票据/
代登录/下单等触达 Courier 的端点以 502 呈现上游不可达；映射/商品/礼包/活动等
本地管理面与玩家冒烟不受影响。

### 回调面（Courier → Spawn）

- teller Deliver 发货 webhook：`POST /internal/deliveries`（payments，
  `X-Game-Delivery-Signature` HMAC，密钥 `PAYMENTS_DELIVERY_SECRET`，
  未配置 = 分发 fail-closed 全 403）。沙箱联调发货必须显式给值。
- 沙箱渠道支付回调：`POST /callback/sandbox/...`（payments，HMAC
  `PAYMENTS_SANDBOX_SECRET`，仅开发/沙箱）。
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
