# TODO

## 手游平台扩展（2026-10-29）

调研与功能差距见 [手游平台调研](docs/zh/product/mobile-platform-research.md)。本节为待实施清单，阶段编号表示依赖顺序；竞品功能不等于首版必做。保留历史完成记录，新增任务使用 `GP-*` 标识。

约定：在 Spawn 扩展统一游戏库；联运只打“联运”标签，不拆目录/运行模式；倍率充值属于活动。游戏内 SDK 与公共能力优先复用独立项目 [Courier](https://github.com/cuihairu/courier)，不另建 Spawn SDK。

- [x] GP-R01 调研 TapTap、好游快爆、小米、九游、B 站、3733、米洛和鸿茁的公开功能与接入流程；记录来源、证据限制及 F01–F28 差距（见调研文档第 2–3 节）。
- [x] GP-R02 静态核查 Courier 六端 SDK、身份/scope、角色、teller 支付/快照存储/发货钩子及管理面边界；将从零开发 SDK 改为跨仓接入任务（见调研文档第 4 节；本次未重新运行 SDK 测试）。

### M0：接入设计与基础治理

- [x] GP-001 确定 Spawn/Courier 权威账号源与存量账号绑定方案，画清登录、验票、退出、注销和游戏级会话流；不假设 JWT 互通。验收：跨游戏 token 被拒，账号迁移/解除绑定可追踪（F13/F27）。设计见 [接入设计](docs/zh/architecture/courier-integration-design.md) §1。
- [x] GP-002 定义 Spawn game ID ↔ Courier game_id/env、合作方、区服/角色映射及能力配置；“联运”标签只作展示筛选。验收：标签变化不自动开通支付/下载，scope 不进入 Courier URL/payload（F01/F14）。设计见 [接入设计](docs/zh/architecture/courier-integration-design.md) §2。
- [x] GP-003 明确交易事实源、管理 API/事件接口及跨仓版本兼容规则，验证 SKU/订单/回调密钥/存储的多游戏隔离；不复制 Courier 订单状态机（F15/F16）。设计见 [接入设计](docs/zh/architecture/courier-integration-design.md) §3。
- [ ] GP-004 为后台与生产接入补最小权限、游戏归属和操作审计，确认 Spawn 数据/包体持久化与 Courier OrderStore/StateFile 的部署和备份恢复方案；不把已有快照接缝列为未实现（F11/F26/F28）。模型与方案见 [接入设计](docs/zh/architecture/courier-integration-design.md) §4；最小实现待 GP-103 后台与 GP-105 存储。
- [ ] GP-005 提供 Spawn + Courier 联合沙箱配置、版本清单与一款示例游戏；记录真实渠道、游戏服、授权和设备依赖，串联跨仓回归而非重复生成六端 SDK（F28）。沙箱清单见 [接入设计](docs/zh/architecture/courier-integration-design.md) §4；实际搭建待 GP-105 / GP-206。

### M1：统一游戏库与发行下载

- [x] GP-101 复用现有 Tags/Tag 后端筛选与详情展示，补 Web/mobile 游戏列表“联运”标签筛选、清除、分页和 URL/导航状态；补运营标签维护入口。验收：联运与普通游戏同库且原浏览正常（F01）。Web/mobile 列表「联运」筛选 + game-catalog PATCH /games/:id/tags 运营入口 + 网关反代。
- [x] GP-102 扩展截图/视频、更新日志、厂商资料和商店链接；按配置展示下载/进入游戏等入口，整理推荐位和榜单现有口径，不将现 score 当真实玩家评价（F02/F03）。落地：games 表新增 screenshots/video_url/changelog/developer_website/store_link/download_url/play_url（幂等列迁移），Web/mobile 详情新增截图/宣传视频/更新日志/厂商资料与「入口」区块，评分标签改「资料评分」。
- [x] GP-103 新增最小运营后台及合作方/游戏归属管理，支持资料与版本提审、审核原因、授权记录和上/下架；开发者自助入口按权限接同一流程（F11/F12/F26）。落地：game-catalog platform_roles/partners/game_ownership/game_reviews/game_versions/audit_logs 六表 + /admin/*、/partner/*、/me 路由（角色守卫、审计、审核原因、上下架、归属、合作方、版本草稿/提审），网关反代透传，web 运营后台/合作方后台两页 + 角色门禁导航；go test -race 全绿，web tsc/eslint/build 绿。
- [x] GP-104 建立游戏版本/渠道包记录：包名、versionCode/versionName、签名指纹、大小、哈希、存储地址、更新说明、审核/发布/撤回；避免低版本包覆盖已发布更新（F07）。落地：game_versions 扩 6 列（package_name/version_code/sign_fingerprint/size_bytes/hash/storage_url，幂等列迁移）+ 发布/撤回路由（仅过审可发布、仅发布可撤回、version_code 低于已发布版本拒绝 409）+ 网关反代 + web 合作方版本表单/版本表与运营发布撤回；go test -race 全绿，web tsc/eslint/build 绿。
- [ ] GP-105 接入包体存储与受控下载，移动端补 Android 下载队列、进度、暂停续传、校验、安装/更新和失败重试；完成真机验收，非 Android 暂以适用商店链接承接（F08/F28）。

### M2：复用 Courier 完成交易与发货

- [ ] GP-201 按 GP-001/002 接通平台账号与 Courier 游戏登录/验票/角色映射；补区服及充值目标关联，保留冻结契约，确需扩展时回 Courier 提案（F13/F14）。
- [ ] GP-202 复用 teller/选定 PaymentsProvider，补服务端商品配置、真实支付渠道适配及订单金额/币种/商户归属核验；验证原生渠道签名，不拿沙箱 HMAC 代替支付机构验签（F15）。
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
