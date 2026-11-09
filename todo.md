# TODO

## 手游平台扩展（2026-10-29）

调研与功能差距见 [手游平台调研](docs/zh/product/mobile-platform-research.md)。本节为待实施清单，阶段编号表示依赖顺序；竞品功能不等于首版必做。保留历史完成记录，新增任务使用 `GP-*` 标识。

约定：在 Spawn 扩展统一游戏库；联运只打“联运”标签，不拆目录/运行模式；倍率充值属于活动。游戏内 SDK 与公共能力优先复用独立项目 [Courier](https://github.com/cuihairu/courier)，不另建 Spawn SDK。

- [x] GP-R01 调研 TapTap、好游快爆、小米、九游、B 站、3733、米洛、鸿茁公开功能与接入流程，记录来源与 F01–F28 差距（6da1a43；调研文档第 2–3 节）
- [x] GP-R02 静态核查 Courier 六端 SDK、身份/scope、角色、teller 支付/快照存储/发货钩子与管理面边界，SDK 由自研改为跨仓接入（6da1a43；调研文档第 4 节）

### M0：接入设计与基础治理

- [x] GP-001 确定 Spawn/Courier 权威账号源与存量绑定方案，画清登录/验票/退出/注销/游戏级会话流；验收：跨游戏 token 被拒、迁移解绑可追踪（ecba93a；接入设计 §1，F13/F27）
- [x] GP-002 定义 game ID ↔ Courier game_id/env、合作方、区服/角色映射与能力配置；验收：联运标签只作展示，scope 不入 URL/payload（ecba93a；接入设计 §2，F01/F14）
- [x] GP-003 明确交易事实源、管理 API/事件接口与跨仓兼容规则，验证 SKU/订单/密钥/存储多游戏隔离（ecba93a；接入设计 §3，F15/F16）
- [x] GP-004 为后台与生产接入补最小权限、游戏归属、操作审计与持久化/备份恢复方案（F11/F26/F28；最小实现待 GP-105）（c57505c/2ac53b0；最小权限：POST /games 与 PATCH /games/:id/tags 收窄为 platform_admin/operator，审计 game_create/game_tag_update，JWT 密钥启动守卫（空密钥拒启/演示密钥大声告警）；持久化/备份恢复方案入接入设计 §4；go test -race 全绿，docs build 绿）
- [ ] GP-005 提供 Spawn + Courier 联合沙箱配置、版本清单与一款示例游戏，记录真实渠道/游戏服/授权/设备依赖（F28；待 GP-105/GP-206）

### M1：统一游戏库与发行下载

- [x] GP-101 复用现有 Tags/Tag 后端筛选与详情展示，补 Web/mobile 游戏列表“联运”标签筛选、清除、分页和 URL/导航状态；补运营标签维护入口（F01；Web/mobile 列表「联运」筛选 + game-catalog PATCH /games/:id/tags + 网关反代）
- [x] GP-102 扩展截图/视频、更新日志、厂商资料和商店链接；按配置展示下载/进入游戏等入口，整理推荐位和榜单现有口径，不将现 score 当真实玩家评价（F02/F03；games 表新增 6 列幂等迁移 + 详情入口区块 + 评分标签改「资料评分」）
- [x] GP-103 新增最小运营后台及合作方/游戏归属管理，支持资料与版本提审、审核原因、授权记录和上/下架；开发者自助入口按权限接同一流程（F11/F12/F26；六表 + /admin/*、/partner/*、/me 路由 + 网关反代；go test -race 全绿，web tsc/eslint/build 绿）
- [x] GP-104 建立游戏版本/渠道包记录：包名、versionCode/versionName、签名指纹、大小、哈希、存储地址、更新说明、审核/发布/撤回；避免低版本包覆盖已发布更新（F07；game_versions 扩 6 列 + 发布/撤回路由 409 防回退 + 网关反代 + web 表单；go test -race 全绿，web tsc/eslint/build 绿）
- [ ] GP-105 接入包体存储与受控下载，移动端补 Android 下载队列、进度、暂停续传、校验、安装/更新和失败重试；完成真机验收，非 Android 暂以适用商店链接承接（F08/F28）。
  - 代码完成（d8b7eba）：game-catalog 本地包体存储（2 GiB 上限、临时文件+原子改名、扩展名清洗，PACKAGE_DIR 未配置则 500）+ 合作方上传端点（仅 draft/pending/rejected 可传/重传，approved 后锁定 409，写入 storage_url=package:// + sha256 + size_bytes + 审计）+ latest 已发布版本端点 + 受控下载端点（登录、仅已发布+平台托管，ServeContent 支持 Range 断点续传/ETag/X-Package-Sha256/416）；集成测试 TestRoutes_PackageDownload 全流程（匿名 401/越权 403/上传→提审→过审→发布→latest→下载→Range 206→坏 Range 416→发布后重传 409→撤回 404）+ 存储单测；go test -race 全绿。api-gateway 增流式透传通道（不缓冲请求体、无整体超时，承载 GB 级上传/下载，1 MiB+1 直通测试）；go test -race 全绿。mobile 补下载队列（进度/暂停续传/重试/取消/size_bytes 截断校验/Android ACTION_VIEW 安装）+ latest 拉取 + REQUEST_INSTALL_PACKAGES 权限；tsc/eslint 绿、expo export android 包成功。
  - 未完，保持未勾：真机验收未做（验收以实际走查/真实使用为准）；SHA-256 完整性校验需原生文件哈希模块（JS 侧暂只有大小校验，服务端 X-Package-Sha256 已下发可对）；App 重启后跨会话续传（savable 落盘）留待补齐；非 Android 商店链接承接入口未动。

### M2：复用 Courier 完成交易与发货

- [ ] GP-201 按 GP-001/002 接通平台账号与 Courier 游戏登录/验票/角色映射；补区服及充值目标关联，保留冻结契约，确需扩展时回 Courier 提案（F13/F14）。
  - 代码完成（e4b436e + 8b6bd98）：新增 courier-adapter 服务（模块 go.work 注册；courier_game_mapping 映射/能力表 + platform_courier_binding 一对一绑定表 + adapter_audit_logs；Courier 冻结契约只读复用：/v1/identity/register|login|delete|delete/cancel，scope=game_id+env 仅请求头传递并测试违规拦截，信封 data/error+traceID，env 注册表可扩展；网关 :8800 反代 + compose 接线 + topology.md 落档）。平台侧：映射管理面（GET/PUT /admin/games/:id/mapping，能力 login/payment/download，角色经 game-catalog /me 权威判定）、代登录（POST /games/:id/session → 一次性短期启动票据，注册/登录会话复用避免双调，随机密码 AES-GCM 落库）、验票（POST /session/redeem 一次性兑换）、注销同步（POST /games/:id/account/delete 与 /cancel，PENDING_DELETE 宽限 + cancelToken）、审计列表（GET /admin/audit）；集成测试 TestRoutes_CourierAdapterFlow 覆盖匿名 401/未映射 404/禁登录 403/自动绑定注册/一次性票据/绑定复用（注册 1 登录 1）/删除 PENDING_DELETE→取消→即时删/scope 越权拦截，courier/ticket/model 单测；go test -race 全绿。game-catalog 增区服目录（game_servers 表 + /partner/games/:id/servers 增改列 + 公开 GET /games/:id/servers 只对已上架游戏、隐藏 closed；充值目标即 game_id+server_id，审计 game_server_create/update）；集成测试 TestRoutes_GameServers（公开空列表/合作方建区服默认 open/跨合作方 403/合作方全状态可见/关闭后玩家侧隐藏/匿名 401/审计含两类动作）；go test -race 全绿。
  - 未完，保持未勾：跨仓沙箱联调与真机验收未做（归 GP-206）；Courier 侧未提供 F13/F14 扩展契约（区服/角色映射与充值目标在平台侧建表，扩展确需回 Courier 提案）；payment/download 能力仅登记未实现，Courier 无对应端点前不标可用。
- [ ] GP-202 复用 teller/选定 PaymentsProvider，补服务端商品配置、真实支付渠道适配及订单金额/币种/商户归属核验；验证原生渠道签名，不拿沙箱 HMAC 代替支付机构验签（F15）。
  - 代码完成（1f63dbd）：新增 payments 服务（模块 go.work 注册；payment_products 商品表：服务端定价 amount_cents/currency + 四形态 + game_id/partner_id 商户归属 + 启停；Courier /v1/payments/* 只读客户端：scope 仅 header、信封展开、契约错误码映射；下单流程 = 商品归属核验（跨游戏 404）→ Courier 下单 → 金额/币种与服务端配置比对（漂移 409）；订单查询/可购 SKU 列表（仅启用）；渠道 S2S 回调验签接缝：sandbox HMAC-SHA256（明确仅开发/沙箱）先验签后解析、签名缺失/不匹配一律 403、通过后构造 Courier HMAC 转发；real 渠道未接入返回 501，绝不拿沙箱 HMAC 冒充支付机构验签（payment.md 安全边界 1–3）；管理面角色经 game-catalog /me 权威判定（platform_admin/operator）；集成测试 TestRoutes_PaymentsFlow（admin 建商品/玩家下单 200/跨游戏 404/金额漂移 409/可购列表/回调缺签名 403/有效 HMAC 200 且转发 Courier 一次/真实渠道 501/玩家建商品 403）+ courier/channel 单测；go test -race 全绿。api-gateway 增 payments 反代（PAYMENTS_URL 未配置不注册）；compose/topology 接线落档。
  - 未完，保持未勾：真实渠道适配未接（WeChat Pay v3/Alipay 原生验签需 ChannelAdapter 实现 + 渠道密钥注入 + 商户凭据，属 Courier Provider 提案/部署面）；订单管理面（全局订单查询/退款/对账重发）属 Courier 提案（归 GP-204）；跨仓沙箱联调与真机验收归 GP-206。
- [ ] GP-203 接真实游戏服幂等发货，落订单→区服/角色/商品关联；提供有权限的补单入口和定时恢复，验证重复回调、发货失败及重启恢复；生产不得沿用 nil Deliver 的沙箱成功行为（F16）。
  - 代码完成：payments 服务新增发货接缝面（账本 delivery_orders + game_delivery_endpoints 两表；下单请求体可带 server_id/role_id，区服归属/开放状态经 game-catalog 公开区服列表权威校验（跨区/维护区 404），订单→区服/角色/商品快照落账本；teller Deliver 钩子的签名 webhook `POST /internal/deliveries`（指令形状对齐 teller.Delivery，HMAC-SHA256 验签先于解析、缺失/不匹配一律 403、Secret 未配置 fail-closed 全部 403，S2S 不进网关）；分发器按订单幂等（order_id 唯一账本，已投递直接成功，同订单不二次发货，并发同订单进程内串行化）、发货面复核商品归属/金额/币种（F15）、缺充值目标拒发、调游戏服带 X-Game-Delivery-Signature（每游戏 endpoint+secret）；管理面 PUT/GET /admin/games/:gameId/delivery、GET /admin/deliveries、POST /admin/deliveries/:orderId/redeliver（补单，已发货 409 不二次发货）、PATCH /admin/deliveries/:orderId/target（目标回填），角色经 game-catalog /me 权威判定（platform_admin/operator，player 403）；定时恢复 pending/failed 到期重投 + 指数退避（封顶 6 倍），账本落盘重启后新实例续扫）。测试：delivery 单测（幂等单次触达/并发同订单单次/失败记账+退避重投/守卫拒绝/真实重启恢复关库重开）+ 集成 TestRoutes_DeliveryFlow（配置接口+密钥不回显/越权 403/下单区服 404/验签先于解析 403·400/有效投递 200+载荷与游戏服签名/重复回调 200 单次/未关联 502/断单 502+player 补单 403+admin 补单 502→200/已发货 409/列表+权限/定时恢复投递/目标回填补单）；go test -race 全绿。api-gateway 增 5 条管理面反代（/internal/deliveries 不反代）；compose 增 PAYMENTS_DELIVERY_SECRET（缺省空=fail-closed）；topology 落档。
  - 未完，保持未勾：teller Deliver 生产接线未做——Courier 网关为冻结仓且未接 Deliver（nil=沙箱成功），真实接线须接入方构建嵌入 teller 的二进制或回 Courier 提案（如 wardenalt 同款 COURIER_PAYMENTS_DELIVER_WEBHOOK 环境变量注入），归 GP-005 联合沙箱/部署面；本服务已备好签名落地面，对接即 Deliver=POST /internal/deliveries。游戏服侧发货接口实现（验 X-Game-Delivery-Signature、按 orderId 幂等）与真机/真实走查验收归 GP-206；发货失败对账/退款/全局订单管理面归 GP-204/GP-205。
- [ ] GP-204 实现玩家订单入口及运营订单查询/检索/审计，建立退款/冲正管理适配与状态映射；不假设 Courier 已有退款端点或全局 HTTP 管理 API（F15/F16/F26）。
  - 代码完成：payments 服务新增订单运营面（投影三表 order_projection/order_audit_logs/refund_requests；玩家我的订单列表 GET /games/:gameId/orders = Courier GET /v1/payments/orders 只读转发（updatedAt 降序 + next_cursor 透传）；运营检索 GET /admin/orders（game_id/status/account_id 筛选 + 有界分页）与详情 GET /admin/orders/:orderId 经只读投影——投影为订单 id 键的事实副本（接入设计 §3.1），状态一律来自 Courier 上报（下单快照 CREATED / 渠道回调 PAID / 发货投递 DELIVERED），本服务不推进第二状态机；操作审计 order_audit_logs（order_query/order_view/refund_request/refund_list/audit_list，actor+role+ip+detail）；退款/冲正管理适配 POST /admin/orders/:orderId/refund（REFUND/CORRECTION，同订单同动作唯一，PAID/DELIVERED/CLOSED 才可退、CREATED 409、金额不超实付）登记 PENDING 意图 + 状态映射（PENDING→执行器→EXECUTED/REJECTED），执行器为 Courier 退款端点提案，响应明示 execution=pending-courier-refund-endpoint，**不假设端点存在、不伪造执行成功**；列表 GET /admin/refunds + GET /admin/audit）。测试：集成 TestRoutes_OrdersProjectionAndRefund（我的订单列表含新建单/投影 CREATED→回调 PAID→投递 DELIVERED 三态来自上报/运营筛选/审计按单与全局/越权 403/退款 200 PENDING+执行待接入/重复 409/未知订单 404/CREATED 不可退 409/退款列表/审计含 refund_request）；go test -race 全绿（payments + api-gateway 双模块 gate）。api-gateway 增 6 条反代（玩家列表 + admin orders/refund/refunds/audit）；topology 落档。
  - 未完，保持未勾：退款执行器未接（Courier 无退款端点，管理适配只登记 PENDING，接入须回 Courier 提案）；对账/差异确认归 GP-205；全局订单查询的本源数据仍在 Courier，投影可能滞后于 Courier 事实（回调/投递上报驱动，无主动拉取），跨仓沙箱走查归 GP-206。
- [ ] GP-205 建最小厂商对账单：实付、优惠承担、退款冲减、合同分成基数、差异与确认导出；先人工确认，不自动打款（F23）。
  - 代码完成：payments 服务新增对账单面（vendor_statements 快照表 + StatementModel/StatementLogic；生成 POST /admin/statements（vendor+partner_id+period+split_rate_bp 基点 0~10000，同 key 重生成更新草稿、已确认 409）从只读投影聚合：实付=周期内 PAID/DELIVERED/CLOSED 订单金额与单数（断单停 PAID 可对账）、优惠承担=0（优惠券子系统未接，GP-305 接入后自订单快照计）、退款冲减=EXECUTED 退款（执行器待 Courier 退款端点提案，接入前 0；经 refund_requests⋈order_projection 按商户维度）；合同分成基数=实付-优惠-退款，分成金额=base×bp/10000（整数）；结算单金额人工录入 PUT /admin/statements/:id/settlement 重算差异=base-结算单；先人工确认 POST /admin/statements/:id/confirm（confirmed 后不可改结算单/重生成，**不自动打款**）；CSV 导出 GET /admin/statements/:id/export（明细行+汇总行，text/csv 下载）；列表 GET /admin/statements（vendor/partner/status 可筛）；审计 statement_generate/confirm/settlement/export/list。订单投影增 partner_id 维度（下单时从服务端商品快照落定，对账按商户聚合）。测试：集成 TestRoutes_VendorStatement（两笔已付已发订单，B 退款执行 1000 → 实付 2000/优惠 0/冲减 1000/基数 1000/分成 700（bp7000）/差异 1000→结算单 700 差异 300→1000 差异 0/确认→409 改结算单+重生成/导出含明细与汇总/列表/越权 403/审计含四动作）；go test -race 全绿（payments + api-gateway 双模块 gate）。api-gateway 增 6 条反代；topology 落档。
  - 未完，保持未勾：差异来源仍为人工录入结算单（Courier 无结算/对账 HTTP API，边界 6）；优惠承担为 0 直到 GP-305 接入；退款冲减为 0 直到 Courier 退款端点接入；对账单数据来自投影（回调/投递上报驱动，可能滞后 Courier 事实），跨仓沙箱对账走查归 GP-206。
- [ ] GP-206 完成一款游戏跨仓沙箱验收并保留证据，再以真实渠道/游戏服/Android 设备单独验收正式链路；覆盖跨游戏/厂商越权及故障恢复（F28）。

### M3：活动、礼包与玩家服务

- [ ] GP-301 建活动基础：状态、起止时间/时区、游戏/区服/渠道/人群、触发条件、限次、预算、叠加与奖励；配置发布需版本快照（F18）。
  - 代码完成：新建 services/promotions 模块（go-zero，端口 8902/metrics 9103，SQLite 默认 `file:data/promotions.db` 或 MySQL，go.work + docker-compose + 网关反代已接）：活动配置域三表 activities/activity_versions/activity_audit_logs；活动状态机 draft→published→disabled→revise（回草稿改配置再发布出新版本；已发布/停用直接编辑 409，须 revise；草稿 disable 409）；起止时间/时区（RFC3339 带时区，开始须早于结束）、游戏归属、区服列表、渠道、人群（audience.type=all 等服务端解析）、触发条件（trigger.type/min_amount_cents）、限次（per_user_count/global_count/budget_cents）、叠加组（stackable+stack_group）、奖励配置（rewards.type/rate_bp 等）全在服务端配置域校验，不信任客户端可信金额；**发布生成不可变版本快照**：activity_versions.snapshot 存发布时点配置 JSON，current_version 递增（v1/v2 旧版本保留可查 GET /admin/activities/:id/versions），运行时按 current_version 引用快照；管理面角色经 game-catalog /me 权威判定（platform_admin/operator 放行，player 403）；操作审计 activity_create/update/publish/disable/revise/list。测试：集成 TestRoutes_ActivityBasics（建草稿/草稿可改/发布 v1+快照/已发布编辑 409/disable/revise/再发布 v2 且 v1 快照保留/列表筛选/详情/玩家 403×2/起止倒置 400/未知 404/草稿停用 409/审计含五动作）；go test -race 全绿（promotions + api-gateway 双模块 gate）。api-gateway 增 Promotions 上游（PROMOTIONS_URL，未配置不注册）+ 9 条反代；topology 落档。
  - 未完，保持未勾：奖励/优惠的运行时计算与发放归 GP-302/303/305（本项只建配置域与版本快照，不做运行时匹配/扣减）；限次/预算的运行时扣减（per_user_count/budget_cents 消费记账）归后续任务；活动快照的运行时消费接口（玩家侧/游戏服侧读取）未建。
- [ ] GP-302 在活动上实现倍率赠送，区分基础购买量、额外赠送量、实付和结算；明确总倍率/额外倍率语义，服务端计算，保留订单与奖励快照及幂等发奖（F19）。
  - 代码完成：promotions 增倍率赠送运行时 + payments 增评估桥。倍率语义显式（调研 §5 歧义消除）：rewards.type=multiplier 必带 mode——`total`=总倍率含基础（rate_bp≥10000，额外量=基础量×(bp−10000)/10000，如 15000=1.5 倍总量）/`extra`=额外倍率纯加赠（rate_bp≥0，额外量=基础量×bp/10000，如 5000=加赠 50%）；基础购买量=服务端商品配置 base_quantity（payments 商品新增字段，下单固化进 order_projection，后改商品不改变旧订单），**非客户端上报**；实付（paid_cents）与赠送量分列，赠币不是现金收入、不进分成基数（GP-205 基数=实付−优惠−退款不变）。运行时评估 POST /internal/reward-evaluations（payments PAID 推送订单事实；HMAC X-Reward-Signature 验签先于解析，未配置 PROMOTIONS_REWARD_SECRET fail-closed 403，与发货接缝同款）；按**已发布版本快照**匹配（ListPublishedSnapshots=current_version ⋈ activity_versions，改草稿不影响运行中版本，发布 v2 后新订单按新版本、旧账本不变）；匹配条件：游戏/区服/渠道/人群（audience.type=all 或请求携带玩家标签相等，定向人群缺省不命中 fail-closed）/起止时间/触发门槛（min_amount_cents≤实付）；叠加裁决确定性：任一 stackable=false → 独跑（额外量最大平票 id 小），否则同 stack_group 互斥组内取额外量最大者、跨组/无组叠加求和；限次消费 per_user_count/global_count（budget_cents 运行时扣减后续）。发放账本 reward_grants：(order_id, activity_id) UNIQUE 幂等——重复评估/重复回调/并发重放均不重复发放（重放返回已发放事实 replay=true）；行内固化订单+奖励快照（活动版本/倍率语义/公式/基础-额外-总量/目标/叠加决策），活动退场后查询不丢；零赠送不落账本；管理面查询 GET /admin/reward-grants（角色 /me 权威判定）+ 审计 reward_evaluate/replay/grant_list。payments 侧：PAID 回调后订单事实入队 promotion_evaluations（order_id 唯一）+ 分发扫描（100ms~interval 可配，指数退避封顶 6 倍、重启续扫；PROMOTIONS_URL 未配置不入队不分发行为不变；消费端幂等 at-least-once 安全）。测试：promotions 集成 TestRoutes_MultiplierRewards（非叠加独跑额外 200/跨组叠加 50+50/重放 replay=true 不重复/限次 skipped per-user-limit-reached/改草稿仍按 v1 计/发布 v2 后按 40000 算且旧账本不变/错签名 403/门槛不足无命中/玩家 403/审计）；payments 集成 TestRoutes_RewardEvaluationBridge（商品 base_quantity=100 落视图/PAID 后桥推送签名验真+事实断言（order/game/server/account/sku/channel/paid_cents=1000/base=100）/重复回调不重复推送）；go test -race 全绿（promotions + payments + api-gateway 三模块 gate）。api-gateway 增 /admin/reward-grants 反代（internal 评估面不进网关）；compose 接 PROMOTIONS_URL + 共享 PROMOTIONS_REWARD_SECRET；topology 落档。
  - 未完，保持未勾：发放执行面未接——账本记 granted 事实，向游戏服实际发奖走 GP-203 发货接缝/游戏服发货接口，真实走查归 GP-206；退款后的奖励撤销（revoked 状态已预留）与资格处理归 GP-303；首充/累充/回流规则归 GP-303；budget_cents 运行时扣减记账归后续；跨游戏/厂商越权评估用例的实仓走查归 GP-206。
- [ ] GP-303 按需增加首充、累充与回流规则，定义累充口径、迟到事件、退款后的资格/奖励处理和补发；不把所有促销塞入基础商品倍率（F19）。
  - 代码完成：promotions 增首充/累充运行时 + payments 增事实计算。三类规则并列（rewards.type=multiplier/first_purchase/cumulative_recharge，不折叠进基础商品倍率）。首充：订单事实 first_purchase=true（payments 按投影判定：同账号同游戏无更早 PAID/DELIVERED 订单）才参与，发放一次 + 账本兜底（该账号该活动 first_purchase 已发放即跳过 first-purchase-already-granted，迟到事件双首充不重复）。累充：档位升序 thresholds=[{cents,extra_quantity}]，累计净额达档逐档发放（Milestone=档位序号）；净额口径=同账号同游戏 PAID/DELIVERED 订单实付合计 − EXECUTED 退款合计（payments 推送时经 order_projection⋈refund_requests 聚合）；幂等为账号+活动+档位维度（UNIQUE(order_id,activity_id,milestone) 防同单重放 + 账号级 granted 档位守卫防跨订单重复发放）；退款撤销通道：评估时按推送净额重算已发档位，跌破即置 revoked（快照保留原发放事实、不追溯撤销前的消耗；重放路径同样重算），净额回升后新订单可补发。叠加裁决修复：无 stack_group 的活动各自独立成组（此前空串被当作共享组键导致无组活动互相误排），"不同组/无组可叠加"语义落实。奖励账本增 rule_type/milestone 列；快照按类型固化（倍率 formula / 首充 extra_quantity / 累充 threshold_cents+milestone+computation.cumulative_cents）。草稿校验：first_purchase 需正额、cumulative 档位升序且为正、未知 type 400。测试：promotions 集成 TestRoutes_FirstAndCumulativeRewards（首充首单+累充档0/重放不重复/跨订单档位幂等 skipped milestone-already-granted/首充兜底 skipped first-purchase-already-granted/退款重放撤销 2 档 revoked 且账本持久化/净额回升补发/未知类型与非法档位 400/审计含 first_purchase、cumulative_cents、revoked）；payments 集成 TestRoutes_RewardEvaluationGp303Facts（首单 first_purchase=true 净额 1000、第二单 false 净额 2000、退款执行后净额回落 1000、全新账号首充成立）；go test -race 全绿（promotions + payments + api-gateway 三模块 gate）。topology 落档。
  - 未完，保持未勾：回流规则仅人群定向机制就位（audience.type + 评估请求 audience_type，fail-closed 不命中），回流人群标签判定源（登录行为数据）未建，见 GP-306 通知/行为链路；发放执行面未接——账本记 granted/revoked 事实，向游戏服实际发奖/撤销走 GP-203 发货接缝/游戏服发货接口，真实走查归 GP-206；退款执行器未接（Courier 无退款端点，净额回落依赖退款 EXECUTED，执行器归 GP-204/Courier 提案）；budget_cents 运行时扣减记账归后续；跨游戏/厂商越权评估用例的实仓走查归 GP-206。
- [x] GP-304 建礼包展示/领取记录、码库存/核销/限领；结合 Courier F62 已有评估确定游戏内兑换边界，未实现能力不标已完成（F17）。
  - 代码完成：promotions 增礼包域三表（gift_packs 配置 + gift_pack_codes 码库存 + gift_pack_claims 领取记录）。管理面：POST/PUT /admin/gift-packs（草稿建/改，服务端配置域不信任客户端金额）、POST /admin/gift-packs/:id/{publish,disable,revise}（状态机 draft→published→disabled→revise；码包发布前须有可用码，直发包无库存门槛）、GET /admin/gift-packs、GET /admin/gift-packs/:id、POST/GET /admin/gift-packs/:id/codes（批量导入码，INSERT OR IGNORE/INSERT IGNORE 重复码忽略，返回 imported/duplicate）、GET /admin/gift-packs/:id/claims、GET /admin/gift-claims。玩家侧（平台 JWT，无需管理角色）：GET /gift-packs（game_id+server_id 筛，仅已发布+起止覆盖当前）、GET /gift-packs/:id、POST /gift-packs/:id/claim、GET /gift-claims/me。领取语义：per_user_limit 限领（槽位=现有数+1，UNIQUE(pack_id,account_id,claim_index) 并发抢末位唯一冲突即限额不超额）、幂等锚 UNIQUE(pack_id,account_id,request_id)（同请求键重提交返回既有领取不重复发码）、码包原子签发 IssueCode（SELECT 可用码+条件 UPDATE 重试，并发安全；无码即释放领取记录回滚报领完）、定向人群 fail-closed（audience.type 非 all 须请求携带相等人群标签）。游戏内兑换边界（F62 已评估落地）：Courier /v1/cdk（mail/cdk，Frozen v1，按账号+码幂等）已实现，码包可在游戏内经 Courier cdk 兑换；Spawn 侧只记核销事实——POST /internal/gift-pack-redemptions（S2S HMAC X-GiftPack-Signature 验签先于解析、未配置密钥 fail-closed 403、issued→used 幂等重放成功），内容实际发放走游戏服接缝（GP-203 同款签名回调）。管理面角色经 game-catalog /me 权威判定；操作审计 gift_pack_create/update/publish/disable/revise/import_codes/claim/redeem 落 activity_audit_logs。测试：集成 TestRoutes_GiftPacks（直发包+码包建/改/发布门槛 400/码导入 imported2+dup1/玩家展示 server1 双包 server2 空/领取 r1 index1 + r1 幂等 + r2 index2 + r3 限领 409/码包 c1 拿码 + c1 幂等同码 + c2 409/无签名 403 + 正确签名核销 200 + 重放 200 + 码 used/管理面码列表 used+available/领取记录/我的领取 3 条/玩家越权建包 403/非法 limit 400/停用后领取 409/审计含 create+claim+redeem）；go test 全绿（promotions 模块 gate）。topology 落档。
  - 未完，保持未勾：Spawn→Courier cdk/mail 客户端接线未建（F62 边界已评估：Courier /v1/cdk 已实现，但 Spawn 侧尚无向 Courier 兑换的 HTTP 客户端——当前核销为游戏服回调驱动，游戏内经 Courier cdk 兑换属客户端侧/游戏服接缝）；直发包（code_required=false）内容实际发放走 GP-203 发货接缝/游戏服发货接口，真实走查归 GP-206；礼包库存全量上限/预算扣减归后续；核销后内容到账与退款返还规则（礼包退款不返还已核销码的边界）待 Courier 退款端点接入后定；跨游戏/厂商越权用例实仓走查归 GP-206。
- [x] GP-305 增优惠券领取、门槛/叠加、核销与退款返还；先设计兼容 Courier skuId 下单契约的服务端报价关联，不让客户端提交可信金额（F20）。
  - 代码完成：payments 增券域三表（coupons 券定义 + user_coupons 玩家持有 + coupon_order_links 核销关联账本）。管理面：POST/PUT /admin/coupons（草稿建/改，服务端配置域）、POST /admin/coupons/:id/{publish,disable,revise}（状态机 draft→published→disabled→revise，仅草稿可改/发布，published 可停用/回草稿）、GET /admin/coupons（game_id/status 筛）、GET /admin/coupons/:id（含 used_count）。玩家侧（平台 JWT）：POST /coupons/:id/claim（幂等锚 UNIQUE(coupon_id,user_id,request_id) 重放返回既有；per_user_limit 仅新插入计数，重放不重复扣限）、GET /coupons/my?status=。下单服务端报价（兼容 Courier skuId 契约，F20）：CreateOrder 客户端仅提交 sku_id + coupon_ids[]，**无任何可信金额字段**；ApplyCoupons 全服务端计算——券 published、起止期内、游戏/sku 归属、门槛（列表价 ≥ threshold_cents）、本人 claimed 实例（按 (coupon_id,user_id,claimed) 收敛查询，他人持有不可见 400 不泄露存在性）、叠加（多券须全 stackable）、发放预算（total_count>0 时 used<total）；折让 fixed 直减 / percent_bp 基点向下取整、封顶列表价；锁券条件 UPDATE claimed→locked（并发双用安全），部分失败回滚已锁券；coupon_order_links 落折让快照 JSON（券码/类型/门槛/列表价/折让额）。PAID 回调核销 locked→used（MarkCouponUsedByOrder 幂等，无关联 no-op）；退款返还 ReturnCouponsByOrder（used/locked→claimed 清订单号，幂等）。对账联动（GP-205 生效）：StatementDiscountAggregate = coupon_order_links ⋈ order_projection（PAID/DELIVERED/CLOSED 且 paid_at 在周期）SUM(discount_cents)；order_projection 增 discount_cents 列；订单响应增 discount_cents/pay_amount_cents/coupons[] 报价快照。Courier 契约边界：Courier CreateOrder 仅收 {skuId}（Frozen v1.1 只读）无折扣入参 → 折让不下传 Courier，Courier 实收仍为列表价，折让记 Spawn 侧优惠承担账本（供分成基数冲减）。管理面角色经 game-catalog /me 权威判定。测试：集成 TestRoutes_Coupons（建券 draft/发布/限领 a1+a2 成功 a3 409 + a1 重放幂等/未达门槛 400/他人持有券不可见 400/有效下单折让 200 应付 800 + 我的券 locked 含订单号/不可叠组合 400 且不落锁/B 单独下单折让 100 + 重用 409/PAID 回调核销 used/返还 claimed 可复用/对账单 paid=1000 discount=200 base=800 split=560 count=1/玩家越权建券 403/percent>100% 400/停用后领取 409）；go test -race 全绿（payments + api-gateway 双模块 gate）。api-gateway 增 9 条反代；topology 落档。
  - 未完，保持未勾：折让未下传 Courier（CreateOrder 仅收 skuId，Frozen 只读）→ 支付渠道实际收款仍为列表价，优惠承担目前为 Spawn 侧账本口径，真实渠道折让需 Courier 促销/折扣支持（提案后接）；退款返还已实现但执行器未接（Courier 无退款端点，接入后在退款 EXECUTED 钩子触发 ReturnOrderCoupons；当前测试经 svcCtx 直调模拟）；percent_bp 券与 fixed 券混叠、券与 GP-302/303 活动奖励的叠加裁决、渠道定向券/新人券发放触达归后续；实仓走查归 GP-206。
- [x] GP-306 建区服/开服计划与预约、版本/活动通知触发，复用已有通知体系并验收远程通知；已有本地提醒不算业务推送完成（F10/F21）。
  - 代码完成：三服务通知链路（预约体系为唯一受众源，开服提醒/版本通知/活动消息共用）。game-catalog：区服 planned_open_at 开服排期（CreateServer/UpdateServer 维护 + toTypesServer 回显）、server_reservations 表（UNIQUE(server_id,user_id) + idx_reservations_game）、玩家预约/取消/我的预约（POST/DELETE /games/:id/servers/:serverId/reserve、GET /games/:id/reservations/me；预约幂等 created 标记、已开服 409、取消幂等）、公开开服表（closed 对玩家隐藏，排期随状态露出）、开服提醒（合作方 PATCH 区服 closed→open 过渡触发 notifyServerOpen：拉未提醒预约 → community 扇出 server_open → MarkReservationsNotified 只发一次）、版本发布触发 game_version（PublishVersion 后 notifyGameVersion：ListReservationUsersByGame 上限 10000）、S2S GET /internal/games/:id/reservation-users（X-Internal-Token 先于解析、未配置恒 403 fail-closed）；Notify 客户端（community /internal/notifications，3s 同步、失败仅记日志、Url 空 no-op）。community：S2S POST /internal/notifications（token 守卫 fail-closed）按 user_ids 逐人落站内信（uid<=0 跳过）并经 realtime-hub /internal/publish 远程推送在线用户（fire-and-forget，返回 created=成功数）。promotions：活动发布触发 game_activity（Publish 尾部 notifyActivityPublished：受众拉 catalog 预约人群 → community 扇出「您预约的游戏有新活动」；0 人静默、失败仅记日志不影响发布、CommunityUrl/CatalogUrl 空 no-op）。api-gateway 增 3 条预约反代（S2S /internal/* 不进网关）。测试：community TestInternalNotifications（403 无/错 token、400 空字段、created=2、ListByUser/CountUnread、hub 收到 publish user_ids [42,43]）；game-catalog TestRoutes_ServerReservations（排期落库回显/公开表隐藏 closed/预约 created=true 重放 false/我的预约视图/开服扇出一次 user_ids[7,8] server_open/维护再开不重发/已开服预约 409/版本发布 game_version 扇出/S2S 403+带 token 去重人群/取消幂等清空）；promotions TestRoutes_ActivityPublishNotify（发布扇出 game_activity user_ids[7,8] target=活动 id/空受众静默/未配置发布不受影响零请求）；go test -race 全绿（community + game-catalog + promotions + api-gateway 四模块 gate）。topology 落档（三服务行 + 网关 §3.2）。
  - 未完，保持未勾：realtime-hub WS 推送仅达在线用户，离线移动端推送通道（APNs/FCM/厂商通道）未建——远程推送验收以 hub 在线投递为准，真机推送归移动端接入；提醒置已发（MarkReservationsNotified）在扇出后无投递回执确认，community 不可达时该批提醒丢失（仅记日志），无重试/补偿队列；预约人群单次查询上限 10000，超出截断；活动消息受众=预约人群而非全量用户广播（全量触达归运营推送系统，未建）；开服排期到点自动开服未建（现为合作方手动 PATCH 触发提醒，定时扫描归后续）；实仓三服务联调走查归 GP-206。
- [x] GP-307 复用/适配 Courier SupportProvider，实现关联账号、游戏、区服角色和订单的售后工单与处理记录；官方公告、身份与置顶接现有内容社区（F09/F25）。
  - 代码完成：courier-adapter 增 Courier SupportProvider 适配代理（契约 Frozen v1.1 只读）：玩家面五端点 POST /games/:id/support/tickets（提单）、GET /games/:id/support/tickets（我的工单，limit/cursor 契约透传）、GET :ticketId 详情（处理记录 = Courier 消息流 PLAYER/AGENT/SYSTEM 升序）、POST :ticketId/messages（追加，CLOSED 409 透传语义）、GET /games/:id/support/faq（检索）；工单归属 = 平台用户 ↔ Courier 一对一绑定（EnsureSession 复用 GP-201 绑定/代登录，首用自动建号）；context 关联事实服务端构建：spawn_user_id/courier_account_id/game_id 自动附加且业务不可覆盖（契约 SDK 口径），serverId 经 game-catalog 公开区服表校验归属（不存在/不属于该游戏 400；上游不可达 fail-open 不写区服事实不阻断），orderId ≤64 字卫生检查透传，业务键形 1-32 位 [a-zA-Z0-9_.]、值 string≤256/number/bool、总量 ≤10 键（含自动附加）越限 400；Courier 域错误映射 SUPPORT_TICKET_NOT_FOUND→404（不泄露存在性）、SUPPORT_TICKET_CLOSED→409、COMMON_INVALID_ARGUMENT→400、RATE_LIMITED→429、其余 502；capability support 按游戏开通（存量行无键 fail-closed 403，mapping 读写/审计 detail 三处补 support 透传）；创建/追加留审计 support_ticket_create/append；scope 只走 X-Courier-Game-Id/X-Courier-Env header。api-gateway 增 5 条工单反代。community 增官方身份与公告：posts.author_official 列（双方言 DDL + migratePostColumns 存量补列，幂等重入）、建帖时服务端按审核管理员（AdminSet/IsAdmin）落标（客户端不可自报）、POST /api/v1/posts/:id/pin 置顶/取消（仅审核管理员，幂等，缓存失效）、GET /api/v1/announcements 公告入口公开读（仅 published+官方帖，置顶优先 id 倒序，limit 1-50 默认 20）；api-gateway 增 2 条反代。测试：courier-adapter TestRoutes_SupportTickets（未登录 401/未接入 404/能力未开 403/提单 200 且 context 自动附加+保留键不可覆盖+区服名落 context/区服不属于 400/列表/详情处理记录 PLAYER+AGENT 升序/追加 200+CLOSED 409+非本人 404/FAQ/审计含 create+append/scope 全程只走 header 零违规）；community TestOfficialPostPinAnnouncements（管理员帖 author_official=true 玩家帖 false/玩家置顶 403/管理员置顶 200 幂等/公告公开读仅官方帖置顶优先/limit=1 收口/取消置顶后顺序回落 id 倒序/置顶不存在帖 404）；修复 ListLikedPosts 内联 SELECT 列清单未含 author_official 与 scanPost 列序错位（致点赞列表空集回归）。go test -race 全绿（courier-adapter 测试 12 项 + community 测试 300 项 + api-gateway gate）。topology 落档（courier-adapter/community 行 + 网关 §3.2 两行）。
  - 未完，保持未勾：坐席侧（客服回复/关单/派单）走 Courier 管理面不经 Spawn（契约边界：croupier in-process 管理面无 HTTP 代理面），Spawn 侧处理记录只读自 Courier 消息流，客服工作台未建；orderId/角色仅卫生检查透传，订单归属强校验待 payments S2S 订单查询端点（现为用户可提交任意 order_id 字符串）；区服校验上游不可达 fail-open（不写区服事实不阻断提单）；工单限流为 Courier 侧 5/min/account，Spawn 不另设；站内私信不替代售后工单（入口分离）；实仓走查归 GP-206。

### M4：后续按需排期

- [ ] GP-401 建游戏评分/长评/举报与开发者回复；补收藏/预约/上线通知，测试招募与资格分发另按需求切片（F04/F05/F06）。
- [ ] GP-402 建下载、注册、活跃、付费、发奖、退款事件和指标口径，补漏斗/留存/活动效果；区分战绩 data-panel 与经营分析，账单从交易事实核对（F24）。
- [ ] GP-403 增单层推广链接/渠道归因与渠道包追踪，记录归因窗口和变更历史；多级代理不随此任务默认引入（F22）。
- [ ] GP-404 增合作方自助数据/账单确认及结算差异处理，按需要深化运营后台、测试发行和推荐排序；权限隔离随功能验收（F03/F06/F12/F23）。

候选未排期：账号交易、小号回收、平台储值币、付费会员、多级代理、自动打款、渠道 SDK 聚合及 iOS 第三方分发。外部实名/支付等接入按实际地区与合作要求记录，不因竞品宣传自动纳入实施。

## 历史完成记录

- [x] realtime-hub 隧道：网关 /ws 原始字节透传，JWT 校验留在 hub，网关补 X-Forwarded-For（1d38531；realtime-hub 测试 45 项）
- [x] realtime-hub 已读回执：mark-all-read 成功后向本人全部连接广播 notification.read（3c06d95；realtime-hub 测试 45 项）
- [x] realtime-hub Redis 扩展：RedisAddr 配置即启用跨实例 pub/sub 中继，坏地址启动期 fail fast（83d2fa5；realtime-hub 测试 45 项）
- [x] realtime-hub 推送桥：PushWebhook 离线推送接缝，单实例零投递用户 POST webhook，批量合并 fire-and-forget（a4b7ca9；realtime-hub 测试 45 项）
- [x] realtime-hub 语音信令：signal 上行无状态转发，Relay 模式跨实例兜底，hub 不存房间状态（acdc8ac；realtime-hub 测试 45 项）
- [x] community 私信后端：direct_messages 表 + 五端点 + message/message.read 实时帧（f08e19d；community 测试 353 项）
- [x] 私信网关反代 + web 消息页：会话列表、发送框、未读徽标轮询 + WS 加速（960541c；gateway 测试 124 项）
- [x] web 私信入口：用户主页「发私信」按钮 + /messages?peer= 直开会话（f9122d5；community 测试 353 项）
- [x] mobile 私信：/messages 私信屏、用户主页与 Profile Tab 入口、API 五函数（cc4a90b；expo export 走查绿）
- [x] community 进程内全栈 e2e：真实 rest.Server + 全量路由 + SQLite + JWT 中间件，编排旅程 + 数据回放幂等契约（574956b；community 测试 353 项）
- [x] community Like 计数幂等修复：仅关系新增时 +1，并发同用户重放不重计（574956b；community 测试 353 项）
- [x] community 帖子搜索：GET /api/v1/posts/search 公开端点 + 网关反代 + 测试四层（574ae0d；community 测试 353 项）
- [x] web 帖子搜索：社区页搜索框 + search 模式，空态与清除搜索（6eb0c5c；tsc + build 绿）
- [x] mobile 帖子搜索：社区 Tab 搜索条 + search 模式接入 load/loadMore（3545e78；expo export 走查绿）
- [x] ES 全文搜索保留为规模化升级路径，community README 长期项已改写说明（574ae0d）
- [x] Actions 版本升级：checkout/setup-node/setup-go/pnpm-action 全部升主版本，CI/docs 双绿（6d12e81；CI 绿）
- [x] Ubuntu 26 迁移评估：钉 ubuntu-24.04 消除漂移，nightly CGO gcc 为唯一镜像侧依赖，canary 与升版时间点已记录（abf2190）
- [x] 文档一致性审计：门面 + 全仓文档 82 项差异清点修复，落 docs/审计-文档一致性.zh.md（694140c；Go 8 模块 -race 全绿 + docs build 绿）
- [x] docs 双语第二轮：子目录 architecture/overview.md、architecture/topology.md、guide/getting-started.md 配对与切换器（6c04999；3 份全量英文译文，zh 侧 .zh.md 重命名，交叉引用与 vitepress 导航同步重指；另修根目录 3 份 zh 切换器段落粘连，docs build 绿）
- [x] 审核整改（2026-10-18 独立审核，见 docs/review/luna-review-2026-10-18.md）：
      表一 8 项全修（JWT 守卫+compose 参数化 87f77cb/e11be23、越权改资料 403 与
      邮箱防枚举 87f77cb、管理员演示默认 MySQL 拒启 e11be23、举报条件更新+409
      e11be23、私信未读统计报错 72f923b、Android 渠道先建 c0a8248）；表二 4 项
      全修（以码对文档）；表三 6 项只评估分期留拍板。user-service 9 包 / community
      全包 go test -race 绿，mobile lint+tsc+expo export 绿，docs build 绿；
      版本号未动、未打 tag。
- [x] 文档时效性清理（2026-10-18 用户令：很多文档已过时）：
      全仓 50 份 .md 三路并行清查（2 路 API 配额中断由本人吸收），13 处失实
      全部以代码为准修正，零业务代码改动：mobile README 撤「走网关 /ws」假声明
      （实际 30s 轮询 + 本地通知，src/lib 无 WS 代码）、community README 仓储层
      补四表 + 六服务→八服务、content-service metrics 前缀更正
      `content_service_gamecatalog_client_*`（client/metrics.go:35）与目录树根
      content-service/、web GUIDE 补 community/stats/home/feed/upload 集成面与
      网关/data-panel/realtime-hub 前置、web README env 补 VITE_REALTIME_HUB_URL
      （src/lib/realtime.ts:5-6）、COMPLETION_SUMMARY「至今仍缺」标题与 2026-10-10
      已落地更新和解、services/README data-models 标规划、realtime-hub README
      链接文字对齐 .zh.md、dev-guide 七 target（9091–9097，9096=rpc 不在根
      compose）+ 待拍板材料标注已结清。docs build 绿（vitepress v1.6.4，
      build complete in 5.12s）。

- [ ] **DESIGN-001 · P1 · 策划视角的游戏拆解库 + 设计者玩家碰撞社区（用户令 2026-11-01：给游戏设计者使用,从策划角度拆解游戏,按设计归类同类,供策划参考讨论）**
  - 定位:spawn 新增内容类型「设计拆解」——把常见游戏从策划视角结构化拆解,按设计归类聚合,形成策划与玩家碰撞讨论的社区板块。
  - 拆解模板(每游戏一篇,结构统一):核心循环(core loop)/进度系统/经济系统(产出与回收)/战斗或玩法机制/养成线/社交与协作/付费设计(如有)/反作弊与公平性/值得抄的设计点/踩坑点。
  - 品类归类:MMORPG/MOBA/SLG/卡牌/放置/FPS/生存建造/休闲益智 等,同类游戏聚合页(横向对比表:循环差异/经济差异/养成差异)。
  - 首批内容:每品类 2-3 款代表游戏拆解(公开可查的设计事实,标注来源;不编造内部数值)。
  - 社区功能:拆解页挂讨论区(复用现有 community/guide 基建);策划可发布自己的拆解,玩家可评论碰撞;优质拆解进精选。
  - 验收:内容类型上线(web+mobile 双端)、5 个品类聚合页、首批 10+ 篇拆解、讨论区可用;vitest 绿门禁。

- [ ] **DOC-GIF · P2 · 文档 · 配置开关效果动态图(每个开关 off→on 对比+GIF)**
  - 交付:枚举本仓全部「开启后有效果」的开关/配置项;自动流水线生成证据——GUI 类用 @vscode/test-electron 或应用截图(off 一张/on 一张+切换过程录屏),CLI 类用终端录屏(asciinema→GIF 或 ffmpeg);ffmpeg 缝合动态 GIF。
  - 文档:docs「配置效果」页(双语)逐开关嵌入:说明+off 图+on 图+GIF。
  - 验收:清单内每个开关都有 off/on+GIF;流水线可重跑(配置变更随时再生成);兼作配置效果回归测试。
  - 参考:kode todo C-24 同款流水线模式(2026-11-01 用户令全舰队推广)。

## 漏洞扫描遗留（2026-11-03 全舰队扫描，源 scratch/vuln-scan-*.md）

- [ ] VULN-SPAWN-001 [高危·GHSA-mrhx-c7xc-m3w5] zod 3.25.76（另有 4.4.3 实例）→≥4.6.6【跨大版本需改代码】：@expo/cli 依赖 zod3（终扫残留 1）；验收：pnpm audit --audit-level=high 中 zod 不再命中；该仓无 test 脚本，audit-only

## 源码分层分析 2026-11-07

HEAD：`bc9866d596d7c2e16fe65b08dda2a4af0a6e7596`。完整清单见 `docs/architecture/layer-analysis.md` 和 `docs/zh/architecture/layer-analysis.md`。

### 抽象层清单

- 客户端体验层：渲染领域界面并维护客户端会话、轮询与实时连接；代表 `apps/web-client/src/App.tsx:23`。
- 访问网关层：路由/缓冲/上传/实时隧道与聚合编排，不拥有业务表；代表 `services/api-gateway/gateway.go:44`。
- 领域服务层：按用户、游戏、社区、内容、战绩边界提供 REST 用例；代表 `services/community/community.go:45`。
- 实时传输层：维护在线注册表、JSON 帧分发、可选 Redis 中继与内部事件入口；代表 `services/realtime-hub/realtime-hub.go:40`。
- 跨服务传输层：承载网关聚合、用户到游戏、内容到游戏等通道及其差异化韧性策略；代表 `services/content-service/client/gamecatalog.go:18`。
- 持久层：按 DSN 选择 SQLite/MySQL，执行建表、种子、查询与写入；代表 `services/community/internal/model/postmodel.go:29`。
- 共享工具层：提供运行环境模板、Make 目标和规划中的共享契约；代表 `packages/config/README.md:1`。
- 平台观测层：装配部署拓扑、指标端口与日志/指标采集；代表 `platform/README.md:1`。

### 超大文件候选

- [ ] LAY-001 [P1] 拆分 `apps/web-client/src/api/client.ts`（1792 行）——同一文件同时承载多域类型、请求封装、页面 API 和实时/统计聚合，接口边界已不可扫读（`apps/web-client/src/api/client.ts:1`）。
- [ ] LAY-002 [P1] 拆分 `apps/mobile-app/src/api/client.ts`（1001 行）——多业务 API、统一信封处理和领域类型集中在一个客户端文件，应按服务域分文件（`apps/mobile-app/src/api/client.ts:1`）。
- [ ] LAY-003 [P1] 拆分 `services/api-gateway/internal/handler/routes_integration_test.go`（1153 行）——测试覆盖多代理族和网关自有行为，场景类型应拆成独立测试文件（`services/api-gateway/internal/handler/routes_integration_test.go:1`）。

### 目录/分类不合理处

- [ ] CLN-001 [P2] 移除 `services/user-service/user`（118081 字节的编译二进制不应入库）；补 `.gitignore`/构建输出边界（`services/user-service/user:1`）。
- [ ] CLN-002 [P2] 为 `services/user-service`、`services/game-catalog`、`services/data-panel` 补服务 README；现有 `community`、`content-service`、`api-gateway`、`realtime-hub` 已有入口说明（`services/community/README.md:1`）。
- [ ] CLN-003 [P2] 网关 1153 行集成测试按代理族拆分，避免单个文件成为全路由变更的耦合点（`services/api-gateway/internal/handler/routes_integration_test.go:1`）。
- [ ] CLN-004 [P2] 统一跨服务客户端韧性抽象；目前仅 content→game-catalog 具备重试/熔断/指标，user→game-catalog 与网关聚合策略未共享（`services/content-service/client/breaker.go:22`）。
