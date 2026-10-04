# spawn 手机版本计划（mobile-app）

> 状态：**已拍板，实施中**（2026-10-02 定稿，M0 首批开工）。
> 实施进度批注于各里程碑行内；变更记录追加在「拍板记录」表。
>
> 关联：总体目录规划见根 `README.md`（`apps/mobile-app` 占位）；
> 后端能力与部署形态见 `docs/development-guide.md`。

## 0. 现状盘点（2026-10-02）

| 项 | 现状 |
|----|------|
| 移动端规划 | 仅根 `README.md` 目录树一行占位：`apps/mobile-app # iOS/Android 客户端（含小黑盒式数据面板）`，无独立计划 |
| 移动端代码 | 无（`apps/` 下只有 `web-client`） |
| 可复用后端 | 六个 Go 服务全部 REST + JWT：user-service（注册/登录/推荐）、game-catalog（游戏库/榜单/推荐）、content-service（攻略/评论）、community（帖子/话题/关注/点赞）、api-gateway（聚合 + 反代）、user-service-rpc（内网 gRPC，手机不直连） |
| 前端技术栈 | web-client 为 React 19 + TypeScript + Vite；无 React Native / Flutter 任何依赖 |
| 未落地后端 | README 规划中的 data-panel（战绩）、matchmaking / realtime-hub（开黑/实时）均未实现——手机版「小黑盒式数据面板」依赖 data-panel，需先行或降级 |
| 接入方式现状 | web-client 直连四服务（`apps/web-client/src/api/client.ts` 硬编码 localhost:88xx）；api-gateway(8800) 端点面窄（login / games/featured / recommendations + community/content 反代）——已过时：BFF 一/二阶段（2026-10-04）后网关已扩 users/games 反代、`/home/feed` 聚合与分享卡，见第 3 节「网络」行 |

## 1. 目标平台

| 平台 | 优先级 | 说明 |
|------|--------|------|
| **Android** | P0（首发） | 国内游戏社区主战场；模拟器 + 真机双通道调试，上架国内应用市场 |
| **iOS** | P1（紧随） | 同代码库出包，差异仅在推送证书与上架流程；无 Apple 开发者账号前以 TestFlight/侧载验证 |
| 鸿蒙（HarmonyOS NEXT） | 暂不 | RN 生态对纯血鸿蒙支持未稳；待社区方案成熟后评估，不进首期排期 |

## 2. 功能面切分

### 2.1 上手机（按后端就绪度排序）

| 功能块 | 后端依赖 | 里程碑 |
|--------|---------|--------|
| 注册 / 登录 / JWT 安全存储（SecureStore） | user-service ✅ | M1 |
| 游戏库浏览 / 搜索 / 详情 | game-catalog ✅ | M1 |
| 榜单（特色/推荐） | game-catalog ✅ | M1 |
| 攻略阅读 + 评论（浏览/发表） | content-service ✅ | M2 |
| 社区帖子流 / 话题圈子 / 发帖 / 点赞 / 分享计数 | community ✅ | M2 |
| 关注关系（话题/用户）+ 关注流 | community ✅ | M3 |
| 个人中心（资料/我的帖子/我的点赞） | user-service + community ✅ | M3 |
| 本地推送（新帖/热榜提醒，Expo Notifications） | 无后端依赖 | M3 |
| 深链与分享卡片（app links / universal links） | 网关侧路由 | M3 |

### 2.2 首期不上手机（及原因）

| 功能 | 原因 |
|------|------|
| 小黑盒式战绩数据面板 | 后端 data-panel 服务未落地；先上「游戏库 + 社区」，面板随 data-panel 落地后作为 M4 增量 |
| 开黑房 / 语音 / 实时匹配 | matchmaking / realtime-hub 均未落地 |
| 运营后台类功能 | admin-console 域，移动端不做 |
| 支付 / 会员体系 | 后端无此域 |
| 小程序壳 | 独立 `apps/mini-program`，另行规划 |

### 2.3 移动端独有交互约定

- 列表一律下拉刷新 + 触底分页（对应各服务 List 接口的 limit/offset）；
- 发帖/评论文本框多行自适应，图片选择走系统相册（首期单图，多图随 content-service 图片能力）；
- JWT 过期静默跳登录页（与 web-client 401 语义一致）。

## 3. 技术选型（与主仓技术栈的关系）

| 决策点 | 选型 | 理由 |
|--------|------|------|
| 框架 | **React Native（Expo 托管工作流）** | ① 与 web-client 同为 React + TypeScript：类型、工具函数、团队心智直接复用，README 规划的 `packages/shared-utils` / `packages/data-models`（TS 类型契约）天然双端共享；② 后端全为 REST + JWT，无自定义原生协议需求，RN 完全覆盖；③ Expo 托管原生构建（EAS），双平台出包不需要本地维护 Android/iOS 双套原生工程 |
| 备选否决 | Flutter | 技术面优秀但引入 Dart 第二语言体系，与本仓 React 前端零复用，单人/小队规模下双栈维护成本不成比例（如拍板偏好 Flutter，功能面与里程碑不变，仅替换本节） |
| 语言 | TypeScript（strict） | 与 web-client 一致 |
| 状态/数据 | 轻量自研 fetch 封装 + React Query（TanStack Query） | 后端接口规整（统一 code/message 信封），无需重客户端；React Query 覆盖缓存/重试/分页 |
| 网络 | **经 api-gateway 统一入口（BFF 第一阶段，2026-10-04 切换）**：`EXPO_PUBLIC_API_URL` 覆盖网关域名，开发默认 `host:8800`（Android 模拟器 10.0.2.2）。网关侧自有聚合 handler（登录/精选/推荐/分享卡）+ 四路反代（community/content/users/games）覆盖 App 全量调用面；曾为手机版先行扩网关会拉长关键路径的顾虑随 BFF 第一阶段落地解除 |
| 认证 | JWT Bearer + expo-secure-store | 与六服务现有中间件零改动对接 |
| 目录 | `apps/mobile-app`（README 已占位），pnpm workspace 接入 | 与 monorepo 规划一致 |
| 类型契约 | `packages/data-models`：先手写 TS 类型 + 一个 `make types` 校验脚本对照 `.api` 文件；后端 goctl 无 TS 生成器，不为此引代码生成链 | 避免为类型同步先建一套生成基建 |

## 4. 里程碑排期

> 节奏按「1 名全职前端 + 后端按需支持」估算；每期含联调与真机验证，
> 验收标准写入各期。日历以拍板日为 T0 顺延。

| 期 | 内容 | 验收标准 | 周期 |
|----|------|---------|------|
| **M0 工程骨架** | Expo 脚手架落 `apps/mobile-app`、pnpm workspace 接入、TS strict、ESLint 对齐 web-client、导航框架（底部 Tab：游戏/社区/我的）、CI 接入现有 gofmt/build 门禁旁路（lint + tsc） | Android 模拟器跑通空壳 App，三 Tab 可切换，CI 绿 | 1 周 | ✅ 2026-10-02 首批落地（环境无模拟器，以 `tsc` + `eslint` + `expo export --platform android` 产物走查替代真机验收，真机回归并入 M1） |
| **M1 浏览 + 账号** | 登录/注册（含 JWT SecureStore 与 401 跳转）、游戏库列表/搜索/详情、榜单页 | 真机登录 → 浏览 → 搜索 → 详情全链路；token 重启存活 | 2 周 | ✅ 2026-10-03 功能落地（环境无模拟器/真机：后端契约走查用本地双服务 + curl 全链路验证；App 侧以 `tsc` + `eslint` + `expo export --platform android` 产物走查替代；token 重启存活依赖 SecureStore 真机行为，M0+M1 真机回归顺延至下一有设备批次） |
| **M2 内容 + 社区** | 攻略阅读 + 评论、帖子流/话题/发帖/点赞；下拉刷新 + 触底分页 | 真机完成「看攻略→评论」「看帖→点赞→发帖」闭环 | 2–3 周 | ✅ 2026-10-03 功能落地（环境无模拟器/真机：后端契约走查用本地 content/community/user 三服务 + curl 全链路验证——攻略列表/详情/评论发表、话题/帖子流/发帖/点赞/分享计数增减；App 侧以 tsc + eslint + expo export --platform android 产物走查替代；「看攻略→评论」「看帖→点赞→发帖」真机闭环并入 M0+M1，顺延至下一有设备批次） |
| **M3 关系 + 收尾** | 关注/关注流、个人中心、本地推送、深链分享卡；Android 出 release 包（EAS 或本地 gradle） | Android 可安装包走完全部已上功能；iOS TestFlight 侧载验证 | 2 周 | 🟡 2026-10-03 第一切片落地：关注关系（话题/用户）+ 关注流（派单称 community 后端「已就绪」，实际仅有关注/取关与 /topics/following，关注流接口 GET /posts/followed、GET /users/following 由本切片补齐）；App 侧社区 Tab「关注」模式（含未登录/空态/分页）、圈子管理页、帖子详情关注作者。环境无模拟器/真机：后端 curl 全链路 + `tsc`/`eslint` + `expo export --platform android` 产物走查替代。第二切片落地：个人中心「我的帖子」（community GET /posts?author_id=）+「我的攻略」（content-service GET /guides?author_id=，中间件可选鉴权核对——作者带 Bearer 查自己可见草稿、匿名仅已发布、草稿详情仅作者可读，curl 三服务走查通过）。第三切片落地（2026-10-04）：「我的点赞」——community 新增 post_likes 关系表（启动建表，SQLite/MySQL 双格式）+ GET /users/likes 按点赞时间倒序分页（Like 幂等落关系、计数保持单调累加契约），App 个人中心第三区块，curl 双服务走查通过（匿名 401/空态/倒序/幂等）；README 下一步建议为架构文档与工具链类远期项，未在本切片内转交付。第四切片落地（2026-10-04）：「本地推送 + 深链分享卡」——App 侧 expo-notifications（SDK 57 插件接入，Android 13 先建通知渠道再弹权限；每日 20:05 本地调度提醒，个人中心开关，恢复/权限拒绝降级，点击通知深链落地社区 Tab，冷启动经 useLastNotificationResponse 覆盖）；帖子/社区流分享改拉系统分享面板携带 `spawn://` 深链（Linking.createURL，真正点分享才计数、取消不计，游客可分享但不计数）；网关新增 GET /s/p/:id 分享卡跳板页（html/template og meta + spawn 深链按钮 + Web 入口按钮，帖子详情拉不到时通用卡片兜底、深链不失效）。走查登记并修复两处：网关社区反代漏注册 M3 三条路由（GET /posts/followed、/users/following、/users/likes——第三切片 App 已在用，经网关会 404，本切片补齐并 curl 验证匿名 401 透传）；分享卡 CSS 误用 Sprintf 式 `88%%`（html/template 原样输出非法 CSS，改为 `88%` 并加回归断言）。expo export 产物走查：提醒文案/开关文案/`/community` 落地路径/POST_NOTIFICATIONS 均在包内（`spawn:///post` 由 Linking 运行时拼装，包内不含字面量属预期）。第五切片落地（2026-10-04）：Android release 本地 gradle 出包——`app.json` 补 `android.package`（com.tappi.spawn）+ `prebuild:android`/`build:android:release` 脚本，`expo prebuild` 生成 android/（CNG，gitignore）后 `gradlew assembleRelease` 走通（toolchain 自动下载 JDK 17；Expo 模板 release 用 debug signingConfig 本地可装）；aapt/apksigner 静态走查通过（包名/权限含 POST_NOTIFICATIONS/深链 scheme/签名）。真机安装与全功能闭环回归仍顺延（无设备环境） |
| **M4（增量，另立项）** | data-panel 落地后的战绩面板；api-gateway BFF 化后统一切网关 | — | 🟡 BFF 第一阶段提前于 M4 完成（2026-10-04）：网关补 users/games 反代（POST /auth/register、GET /users/:id、GET /games、GET /games/:id，与既有 community/content 反代及自有 handler 共存，集成测试按生产装配全量注册暴露路由冲突），mobile-app base URL 收敛到网关单地址（`EXPO_PUBLIC_API_URL` 覆盖）；走查登记并修复跨切缺陷：反代直转 r.Body 时长度未知走 chunked，go-zero 上游解析 chunked 请求体失败（user-service 直连 chunked curl 亦复现 `field is not set`）——proxy 改为缓冲定长转发（1 MiB 上限，超限 413，附契约测试）；五服务 curl 全链路走查通过（注册/登录/资料、游戏列表/详情/精选、攻略/评论、话题/帖子/点赞/分享、关注流/我的点赞、匿名 401 透传、分享卡），App 全量调用面仅经 :8800；web-client 直连拓扑与网关既有路由不变。第二阶段同日落地：网关 `GET /home/feed` 列表聚合端点（精选游戏/热帖/话题/攻略四路并发拉取，逐组字段裁剪——正文/图片等大字段不下发、正文仅截断 60 字摘要；单上游故障降级为空组并记 `degraded`，不做整体 5xx），App 侧消费点同日落地：新增 Tab「发现」（`(tabs)/discover.tsx`）一次请求渲染精选游戏/热帖/话题/攻略四区块，下拉刷新、degraded 降级提示条、空态/错误重试，expo export 产物走查文案与路由均在包内；web-client 不受影响。data-panel 面板仍属 M4 另立 |

风险与依赖：

1. **后端 CORS / 明文 HTTP**：手机真机默认禁 http 明文（Android `usesCleartextTraffic`），开发期走局域网 IP + 放行配置，生产必须 HTTPS——上架前需备公网域名 + 证书（当前 compose 无 TLS 终结，属部署域新增项）；
2. **iOS 开发者账号**：无账号则 iOS 仅模拟器/侧载验证，不影响 Android 首发节奏；
3. **图片上传**：content-service / community 现无对象存储，首期图片功能限「选图预览」，上传能力另立后端任务。

## 5. 拍板记录

| 日期 | 决议 | 批注 |
|------|------|------|
| 2026-10-02 | 按推荐定稿：Android P0 / iOS P1；功能面按第 2 节切分；React Native（Expo）+ TypeScript；里程碑 M0–M3 共约 7–8 周 | 用户授权免审直接拍板；M0 同日开工。Flutter 备选否决理由维持有效 |
