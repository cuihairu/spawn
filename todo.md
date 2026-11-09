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
- [ ] GP-204 实现玩家订单入口及运营订单查询/检索/审计，建立退款/冲正管理适配与状态映射；不假设 Courier 已有退款端点或全局 HTTP 管理 API（F15/F16/F26）。
- [ ] GP-205 建最小厂商对账单：实付、优惠承担、退款冲减、合同分成基数、差异与确认导出；先人工确认，不自动打款（F23）。
- [ ] GP-206 完成一款游戏跨仓沙箱验收并保留证据，再以真实渠道/游戏服/Android 设备单独验收正式链路；覆盖跨游戏/厂商越权及故障恢复（F28）。

### M3：活动、礼包与玩家服务

- [ ] GP-301 建活动基础：状态、起止时间/时区、游戏/区服/渠道/人群、触发条件、限次、预算、叠加与奖励；配置发布需版本快照（F18）。
- [ ] GP-302 在活动上实现倍率赠送，区分基础购买量、额外赠送量、实付和结算；明确总倍率/额外倍率语义，服务端计算，保留订单与奖励快照及幂等发奖（F19）。
- [ ] GP-303 按需增加首充、累充与回流规则，定义累充口径、迟到事件、退款后的资格/奖励处理和补发；不把所有促销塞入基础商品倍率（F19）。
- [ ] GP-304 建礼包展示/领取记录、码库存/核销/限领；结合 Courier F62 已有评估确定游戏内兑换边界，未实现能力不标已完成（F17）。
- [ ] GP-305 增优惠券领取、门槛/叠加、核销与退款返还；先设计兼容 Courier skuId 下单契约的服务端报价关联，不让客户端提交可信金额（F20）。
- [ ] GP-306 建区服/开服计划与预约、版本/活动通知触发，复用已有通知体系并验收远程通知；已有本地提醒不算业务推送完成（F10/F21）。
- [ ] GP-307 复用/适配 Courier SupportProvider，实现关联账号、游戏、区服角色和订单的售后工单与处理记录；官方公告、身份与置顶接现有内容社区（F09/F25）。

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
