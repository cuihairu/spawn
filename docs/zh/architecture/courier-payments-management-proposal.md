# Courier 支付管理适配提案（草稿，待拍板）

> 状态：**已提交 courier 仓待议**（2026-11-23；courier commit `ed12a1d`，落 courier `docs/zh/todo.md` 待议节 + `todo.md` 英文节；不触碰 Frozen 契约代码）。
> 日期：2026-11-23。归属：GP-202 / GP-204 的剩余工作（todo.md 未勾项），接入设计 §3.1「管理适配作为 Courier 提案」的落点。
> 前置事实（活走查 2026-11-23 实测）：courier gateway 二进制启动**不登记 SKU**（`teller.AddSku` 仅装配期 Go 方法，无 HTTP 管理端点），`GET /v1/payments/skus` 空表；Spawn payments 下单已达 teller 但返 404「sku 不可购」（`PAYMENT_SKU_NOT_FOUND`）——全链路 200 卡在价格表为空。

## 1. 问题（现状缺口）

Courier 支付域（teller / teller-alt，`gateway/providers/teller*/`）现有玩家面与回调面齐全，但**管理面端点缺失**，导致 Spawn 侧无法经 HTTP 完成运营动作，只能依赖装配期进程内调用：

| 运营动作 | 现状 | 缺口 |
| --- | --- | --- |
| SKU 价格表登记 / 改价 | `teller.AddSku(id, productID, form, amountCents, currency)` 仅 Go 方法；`payment.md` 明确「管理面登记」，但无 HTTP 端点 | 网关二进制启动期必须代码内调用；活走查实测空表 → 订单 404 |
| 断单对账重发 | Reconcile 为进程内方法（`payment.md` §发货与对账「本 v1 为进程内方法，持久化与定时对账随部署面」） | 无 HTTP 触发面；运营无法从 Spawn 后台补单 |
| 退款 / 冲正 | `payment.md` 扩展预留：**「退款端点 = v1.1+ 提案，管理面先行」**；现仅对账面（Reconcile）内建 | Spawn GP-204 管理适配只登记 PENDING 意图，执行器无端点可调 |
| 全局订单查询 / 检索 | 仅玩家面 `GET /v1/payments/orders`（本人 scope）+ 详情；无管理检索 | Spawn 运营面读的是只读投影，**可能滞后于 Courier 事实**（回调/投递上报驱动，无主动拉取） |

契约依据：`courier/docs/zh/contract/payment.md`（端点总览、sku 数据模型「管理面登记」、§发货与对账「不经网关玩家路由」、扩展预留「退款端点 = v1.1+ 提案」）；接入设计 §3.1（「无 SKU 管理/补单/退款/全局订单查询/结算的 HTTP 管理 API」「管理适配作为 Courier 提案（Provider 接口扩展或独立管理 Provider）」）、§3.3（「新需求先回 Courier 提案做兼容扩展，不在 Spawn 旁路改造」）。

## 2. 目标（提案要解决的）

1. **SKU 价格表登记**：管理面可经 HTTP 登记/改价 SKU（服务端定价），消除「网关启动不登记 → 空表 → 全链路 200 阻塞」。
2. **退款 / 冲正执行**：为 Spawn GP-204 的 PENDING 意图提供可调执行端点（REFUND / CORRECTION），状态映射与金额上限由 Spawn 管理适配守。
3. **断单对账重发**：管理面可枚举停 `PAID` 订单并重发（幂等，不二次发货），落地 `payment.md` §31「断单可对账恢复」。
4. **全局订单检索**：管理面按 game/status/account 检索 + 有界分页，供运营面避免仅读滞后投影。

## 3. 形态选择（设计 §3.1 留两个选项，此处给倾向 + 理由）

| 选项 | 说明 | 倾向 |
| --- | --- | --- |
| **A. Provider 接口扩展** | 在现有 `PaymentsProvider` 接口上加管理方法（`AdminSku`/`AdminOrders`/`AdminRefund`/`AdminRedeliver`），teller / teller-alt 同构实现 | **推荐**：与现有异构（teller 可变表 / teller-alt 事件账本）一致性测试同构；管理面复用 Provider 接缝，不新开一个平行 Provider 的装配/路由成本 |
| B. 独立管理 Provider | 新开 `payments-admin` 域 Provider，单独挂载 | 更清晰隔离，但需新装配、新路由表、双实现 ×2 一致性成本更高；管理面与玩家面共享同一订单存储，割裂反而引入一致性问题 |

倾向 A 的理由：管理操作读写的正是 teller/teller-alt 的订单/SKU 状态，**同一 Provider 内加方法天然共享锁与持久化**；独立 Provider 需跨 Provider 访问订单存储，破坏「事实源唯一」。

## 4. 提议的管理面（v1.1+，只加不改）

挂载建议：`/v1/payments/admin/*`（与玩家面 `/v1/payments/*` 前缀隔离，管理面不经玩家 Bearer-scope 路由）。

| 端点 | 认证 | 说明 |
| --- | --- | --- |
| `POST /v1/payments/admin/skus` | 管理凭证 | 登记/改价 SKU（服务端定价；重复登记 = 改价，已建订单不受影响——现有 `AddSku` 语义）；请求 `{skuId,productId,form,amountCents,currency}` |
| `GET /v1/payments/admin/orders` | 管理凭证 | 全局检索（`gameId`/`status`/`accountId` 筛选 + 有界分页 `limit`/`cursor`）；按 `updatedAt` 倒序 |
| `GET /v1/payments/admin/orders/{orderId}` | 管理凭证 | 管理详情（含断单态 `PAID` 标识） |
| `POST /v1/payments/admin/orders/{orderId}/redeliver` | 管理凭证 | 断单对账重发（仅停 `PAID` 可重发；已 `DELIVERED`/`CLOSED` 409 不二次发货——幂等红线）；同步执行发货钩子，成败回 `DELIVERED`/留 `PAID` |
| `POST /v1/payments/admin/orders/{orderId}/refund` | 管理凭证 | 退款/冲正（REFUND / CORRECTION 两动作）；登记 + 状态映射回 `EXECUTED`/`REJECTED`；金额不超实付（冲减口径归 Spawn GP-205 分成基数） |

管理凭证（认证）：**不经玩家 Bearer-scope**。倾向 **S2S `X-Internal-Token`**（与 Spawn 既有 S2S 守卫同款，见接入设计 GP-404/GP-307/GP-202 映射端点；未配置恒 403 fail-closed），或独立管理 Verifier（若需操作者身份审计则按 Spawn 管理面 `/me` 角色模式透传——**留开放问题 Q3**）。操作审计：写入 `adapter_audit_logs` 同款审计（GP-004 最小权限/审计口径），记操作者 + IP + 详情。

## 5. 兼容性与验证

- **只加不改**：新端点、新错误码可选；现有玩家面/回调面/信封/分页零改动。破坏性变更升版本（versioning.md，接入设计 §3.3）。
- **版本**：`payment.md` 标 v1.1 或 v1.2（按 courier 仓提案批次惯例，见 courier `docs/zh/todo.md`「批次 N 契约 vX.Y 提案」）。
- **一致性**：teller / teller-alt 同构实现 + `gateway/e2e/payments_conformance_test.go` 增管理面 wire 场景（两实现同一结果，GP-202 口径）。
- **Spawn 侧对接**（不在本提案内，落地时另做）：payments 管理适配把 PENDING 退款意图调该端点、补单入口接 `redeliver`、运营检索可切换事实源或做主动拉取校准。

## 6. 开放问题（需 courier 仓与用户拍板）

- **Q1**：挂载前缀 `/v1/payments/admin/*` 还是独立 `/v1/admin/payments/*`？倾向前者（同域前缀，契约表清晰）。
- **Q2**：管理凭证 = S2S `X-Internal-Token`（倾向，零新鉴权面）还是管理 Verifier + 角色？若需操作者身份审计，S2S 令牌无法表达「谁操作」→ 需 Verifier 或由 Spawn 管理适配把操作者写入审计后再调。
- **Q3**：退款冲减/分成基数（GP-205）的金额口径由 Courier 契约保证还是 Spawn 管理适配守？倾向后者（Spawn 已是事实投影 + 分成计算方）。
- **Q4**：全局订单检索的分页游标是否与玩家面 `nextCursor` 同构（倾向是）。

## 7. 与 todo 的关系

- 本提案是 **GP-202**（teller SKU 管理适配）与 **GP-204**（退款端点提案）注记中列明的剩余工作；起草不勾任何框。
- 落地（courier 仓实现 + 契约版本 + conformance）后，GP-202/GP-204 的「Courier 侧」阻塞解除，剩真实渠道商户凭据 / 真实游戏服 / 真机（外部资产）。
