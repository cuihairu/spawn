# Backlog 台账（2026-10-10 立）

> 单一事实源：所有未完成项的状态与卡点。已完成项不在此列（见 git log 与各 README）。
> 维护纪律：每批增量落地后回填本表；新立任务先入表再动工。

## 待办（可写，按优先级）

| # | 项 | 卡点/依赖 | 备注 |
|---|----|----------|------|
| — | （当前无可写待办；web/mobile 双端功能面对称，见下方已完成批注） | | |

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
| 实时推送（WebSocket） | 需独立实时通道基建（realtime-hub 未开工） | community README:269 |
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
> 2026-10-11 续批：技术项 2 条已修（下附提交）；剩 3 条属 API 契约 / 产品决策，**待拍板，未代拍板开工**。

### 已修（2026-10-11）

| 原# | 项 | Commit |
|---|----|--------|
| 3 | `fetchMyFavorites` 补 `code !== 200` 校验：HTTP 200 + 信封 code:401 不再静默渲染空收藏，ProfilePage 转「部分内容加载失败：我的收藏」横幅（与 content 域其余函数口径一致） | `c600193` |
| 4 | 本地裸跑 JWT 密钥陷阱：content-service / community 的 etc yaml 默认 `Auth.JWTSecret` 与 user-service 对齐（`tappi-user-service-jwt-secret-key-2024`），README/ENHANCEMENT 配置快照同步；不设 JWT_SECRET 直接跑时登录态可直达内容/社区接口（裸跑三服务 + 登录令牌跨服务 curl 验证通过；compose 本就显式传 JWT_SECRET，容器行为不变） | `100291c` |

### 待拍板（属契约/产品决策，不代拍板）

| 原# | 发现 | 等级 | 位置 |
|---|------|------|------|
| 1 | web 端无注册入口：网关有 `POST /auth/register`，但 client.ts 无 register 函数、登录表单（HomePage「快速登录」）无注册链路，首次访客无法从 web 注册。需拍板：是否要 web 注册入口（以及是否走邮箱验证/邀请制） | 中 | apps/web-client/src/api/client.ts、src/pages/HomePage.tsx |
| 2 | 鉴权失败状态码口径不一：content/community 匿名访问（如 GET /api/v1/guides/favorites）回 HTTP 200 + 信封 code:401，而网关 /upload 匿名回 HTTP 401、moderation 队列越权回 HTTP 403。统一是跨服务契约变更，web/mobile 双端都在适配 | 低 | content-service/community 业务信封 vs api-gateway upload/moderation |
| 5 | register/login 响应形状不一致：register 回信封 `{code,message,data:{user_id}}`，login 回裸 `{token,user_info}`。统一需同步改两端客户端，属契约变更 | 低 | user-service auth handler |

## 验收打磨轨道

| 项 | 状态 |
|----|------|
| 文档补缺（通知/data-panel/收藏/七服务漂移） | ✅ 2026-10-10（`1e092fa`） |
| CI 全量绿 | ✅ 每次提交后核验（见各批报告） |
| 实机走查 | web 四块 ✅ 2026-10-10（替代口径见上节，发现 5 条已登记）；mobile ⏸ 无设备，替代口径见上 |
| mobile M0–M3 + M4 第一里程碑 | ✅ 2026-10-09 前全部落地 |
