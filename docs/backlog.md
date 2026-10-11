# Backlog 台账（2026-10-10 立）

> 单一事实源：所有未完成项的状态与卡点。已完成项不在此列（见 git log 与各 README）。
> 维护纪律：每批增量落地后回填本表；新立任务先入表再动工。

## 待办（可写，按优先级）

| # | 项 | 卡点/依赖 | 备注 |
|---|----|----------|------|
| 1 | realtime-hub 实时通道基建立项（2026-10-11 拍板开工） | 无外部依赖 | M1 骨架 + M2 接入 + M3-1 在线状态已落地（2026-10-11）；余 M3 子项（隧道/已读回执/Redis 扩展/推送桥/语音信令）逐项再拍板 |
| 2 | ~~CI runner 预防性钉版~~ 已处理（2026-10-11 五处 `ubuntu-latest` 钉 `ubuntu-24.04`：ci×2/docs×2/nightly） | 2026-10-19 起 ubuntu-latest 迁移 Ubuntu 26（runner-images#14748） | 已消除漂移风险；Ubuntu 26 正式迁移波及面明确后再评估是否跟进升级 |

### CI 快照复核（2026-10-11）

> 口径：`gh run list` 近 30 笔 run + 最新 run（38112498701 @ `03c8924`）job 日志逐包核对。

**结论：零红、零 flake，六模块全绿，无需修、无需 rerun。**

| 模块 | CI 结论 | 备注 |
|------|--------|------|
| api-gateway | ✅ 绿 | 11 包 ok（-race） |
| community | ✅ 绿 | 14 包 ok（-race） |
| content-service | ✅ 绿 | 11 包 ok（-race） |
| data-panel | ✅ 绿 | 10 包 ok（-race） |
| game-catalog | ✅ 绿 | 8 包 ok（-race） |
| user-service | ✅ 绿 | 10 包 ok（-race） |
| user-service-rpc | ✅ 绿 | 4 包 ok（-race） |
| web + mobile | ✅ 绿 | lint + typecheck job 34s |

- 近 30 笔中 8 笔 `cancelled`：全部为 `concurrency: cancel-in-progress`（`ci-${{ github.ref }}` 组）在连续小步推送时取消旧 run——注解「Canceling since a higher priority waiting request exists」，非失败非 flake，同 commit 终态由后续绿 run 覆盖，不 rerun。
- 最新 run 两 job 全绿（gofmt+build+test 1m41s；web+mobile 34s），`FAIL` 行数 0，68 包全 ok。
- Watch 项（非红不动配置）：① ubuntu-latest → Ubuntu 26 迁移（2026-10-19 起）记入待办 #2；② Node 20 deprecation 警告——已处理（2026-10-11 升新主版本：checkout@v7 / setup-node@v7 / setup-go@v7 / pnpm-action@v6，均 node24 运行时，CI 全绿验证）。

### 本批已完成（2026-10-10，拉起令批次）

| 项 | Commit |
|----|--------|
| mobile 草稿发布闭环（编辑保存自动补发布 + 详情页草稿徽标） | `2127126` |
| mobile 写攻略 Markdown 预览切换（对齐 web 编辑器） | `5998d22` |
| mobile 帖子评论（列表/发表/回复/作者删除，对齐 web） | `a2144c5` |
| mobile 攻略评论回复/点赞/删除（对齐 web，客户端组树） | `f92ccab` |
| mobile 写攻略「存草稿，稍后再发」（对齐 web 双态保存） | `93fde67` |

## 顺延（外部依赖 / 拍板不开工，不卡主进度）

| 项 | 卡点 | 依据 |
|----|------|------|
| ~~实时推送（WebSocket）~~ | 已解除（realtime-hub M1/M2 落地，2026-10-11） | docs/realtime-hub.md |
| 全文搜索（Elasticsearch） | 外部中间件未引入 | community README:270 |
| 内容推荐系统（ML） | 外部模型服务未引入 | community README:271 |
| 视频内容 | 对象存储/转码基建未立项 | community README:272 |
| matchmaking / realtime-hub / crawler-jobs / admin-console / mini-program | 拍板不开工（2026-10-09） | mobile_plan 第 4 节 |
| 真机回归（本地推送/深链分享卡/通知中心） | 环境无设备无模拟器；替代口径 = CI 门禁 + `expo export --platform android` 产物走查 | mobile_plan 风险节 |
| iOS 填空（TestFlight 侧载/Universal Links/APNs） | 外部凭据（Apple 团队/域名 AASA/推送证书） | mobile_plan 拍板记录 2026-10-09 |

## 实机走查登记（2026-10-10 web 批）

> 口径：本机无浏览器自动化，替代口径 = 全栈拉起（六服务 + vite dev，`VITE_API_GATEWAY_URL` 指网关，compose 同款 JWT_SECRET）
> + 按 `src/api/client.ts` 逐函数走通 API 契约（curl 同路径同载荷）+ SPA 深链 HTML 探活 + web 三连门禁（tsc/eslint/build）。
> 四块结论：通知中心 / 收藏 / 图片上传 / 举报 **主链路全通**（含防重、幂等、越权 403、大小/类型闸、静态回源、read-all 归零）。
> 2026-10-11 续批：技术项 2 条已修（下附提交）；另 3 条拍板项已按用户授权拍板并落地（见「待拍板」节结论）。

### 已修（2026-10-11）

| 原# | 项 | Commit |
|---|----|--------|
| 3 | `fetchMyFavorites` 补 `code !== 200` 校验：HTTP 200 + 信封 code:401 不再静默渲染空收藏，ProfilePage 转「部分内容加载失败：我的收藏」横幅（与 content 域其余函数口径一致） | `c600193` |
| 4 | 本地裸跑 JWT 密钥陷阱：content-service / community 的 etc yaml 默认 `Auth.JWTSecret` 与 user-service 对齐（`tappi-user-service-jwt-secret-key-2024`），README/ENHANCEMENT 配置快照同步；不设 JWT_SECRET 直接跑时登录态可直达内容/社区接口（裸跑三服务 + 登录令牌跨服务 curl 验证通过；compose 本就显式传 JWT_SECRET，容器行为不变） | `100291c` |

### 待拍板（属契约/产品决策，不代拍板）

> 2026-10-11 用户授权按本仓决策记录代拍板（推荐 #2），三项已拍板并落地，记录如下。

| 原# | 拍板结论 | 依据与口径 | Commit |
|---|---------|-----------|--------|
| 1 | **做 web 注册入口**（登录卡内注册态切换，注册成功即自动登录；不做邮箱验证/邀请制——本地部署无邮件服务，属后续独立事项） | web GUIDE 本就列注册为面向访客的功能；web 已有 register API 面（网关反代 /auth/register），补 UI 即可闭环 | `0bd03e5` |
| 2 | **统一：认证/权限失败回真 HTTP 状态（401/403）+ 信封体；业务码（400/404/409/500）保持 HTTP 200 + 信封 code** | 认证失败回 200 会绕过客户端 401 统一处理（mobile 登出跳登录、web 报错横幅）；网关 /upload、moderation 已是此口径，向多数派对齐。content-service 8 个 logic 已迁移 httperr（可选鉴权 GET 匿名不再静默 200+code:401） | `d2b7c93` |
| 5 | **login 与 register 同款业务信封** `{code,message,data:{token,user_info}}`；失败走 httperr → 真 401/400 信封。网关 /auth/login 由聚合 handler 改为与 /auth/register 同款反代（信封原样透传，形状永不漂移）；web/mobile 登录解析 data 字段 | 消除裸 `{token,user_info}` 与信封双轨；实测错密码从「HTTP 400 login failed 纯文本」变为 401 信封可读文案。user-service/网关/web 三层 + mobile 同批落地（全绿后一次推送） | `beb726f` `ec1509f` `548cb49` |

## 验收打磨轨道

| 项 | 状态 |
|----|------|
| 文档补缺（通知/data-panel/收藏/七服务漂移） | ✅ 2026-10-10（`1e092fa`） |
| CI 全量绿 | ✅ 每次提交后核验（见各批报告） |
| 实机走查 | web 四块 ✅ 2026-10-10（替代口径见上节，发现 5 条已登记）；mobile ⏸ 无设备，替代口径见上 |
| mobile M0–M3 + M4 第一里程碑 | ✅ 2026-10-09 前全部落地 |
