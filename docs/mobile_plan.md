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
| 接入方式现状 | web-client 直连四服务（`apps/web-client/src/api/client.ts` 硬编码 localhost:88xx）；api-gateway(8800) 端点面窄（login / games/featured / recommendations + community/content 反代） |

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
| 网络 | 直连各服务（沿用 web-client 模式），配置化 base URL | 现状 api-gateway 端点面太窄（仅 3 端点 + 2 反代），为手机版先行扩网关会拉长关键路径；**演进项**：api-gateway 扩为完整 BFF 后统一切换（见 M3 演进出口） |
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
| **M3 关系 + 收尾** | 关注/关注流、个人中心、本地推送、深链分享卡；Android 出 release 包（EAS 或本地 gradle） | Android 可安装包走完全部已上功能；iOS TestFlight 侧载验证 | 2 周 |
| **M4（增量，另立项）** | data-panel 落地后的战绩面板；api-gateway BFF 化后统一切网关 | — | 不在本计划内 |

风险与依赖：

1. **后端 CORS / 明文 HTTP**：手机真机默认禁 http 明文（Android `usesCleartextTraffic`），开发期走局域网 IP + 放行配置，生产必须 HTTPS——上架前需备公网域名 + 证书（当前 compose 无 TLS 终结，属部署域新增项）；
2. **iOS 开发者账号**：无账号则 iOS 仅模拟器/侧载验证，不影响 Android 首发节奏；
3. **图片上传**：content-service / community 现无对象存储，首期图片功能限「选图预览」，上传能力另立后端任务。

## 5. 拍板记录

| 日期 | 决议 | 批注 |
|------|------|------|
| 2026-10-02 | 按推荐定稿：Android P0 / iOS P1；功能面按第 2 节切分；React Native（Expo）+ TypeScript；里程碑 M0–M3 共约 7–8 周 | 用户授权免审直接拍板；M0 同日开工。Flutter 备选否决理由维持有效 |
