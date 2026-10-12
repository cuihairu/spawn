# TODO

> 生成于 2026-10-11（无 todo 文件，来源：docs/backlog.md「待办」表 + CI 复核记录）。

## realtime-hub M3 子项（M1/M2/M3-1 已落地 2026-10-11；余项 2026-10-12 用户令连续推进，全部落地）
- [x] 隧道（tunnel）：网关 `/ws` 原始字节透传（Hijack + 双向 splice，JWT 校验仍在 hub，网关补 X-Forwarded-For）；顺带修 hub gorilla 默认 CheckOrigin 误伤浏览器跨源握手（web 走网关隧道默认化，`ws://localhost:8800`）（1d38531）
- [x] 已读回执：mark-all-read 成功后向本人全部连接广播 `notification.read`，多端徽标即时归零（3c06d95）
- [x] Redis 扩展（跨实例投递）：`RedisAddr` 配置即启用 pub/sub relay（尽力而为、回环剔除、坏地址启动期 fail fast；miniredis 全测）；go-redis v9 订阅惰性建立已用 Receive 确认堵漏（83d2fa5）
- [x] 推送桥（push bridge）：`PushWebhook` 离线推送接缝——单实例零投递用户 POST webhook（X-Push-Token，批量合并，fire-and-forget）；多实例本地零投递 ≠ 离线故不推（分布式推送决策留部署面）（a4b7ca9）
- [x] 语音信令：`signal` 上行无状态转发（offer/answer/ICE 透传，发端连接排除、多端全收；Relay 模式跨实例兜底；hub 不存房间状态）（acdc8ac）

## 私信（2026-10-12 巡检点火批次，自定位自 docs/realtime-hub.md 目标「私信等实时语义」）
- [x] community 后端：`direct_messages` 表 + 五端点（发送/会话列表/会话线/会话已读/未读总数）+ `message`/`message.read` 实时帧（f08e19d）
- [x] 网关反代 + web 消息页：/messages 会话与线、发送框、✉️ 徽标 30s 轮询 + WS 加速（960541c）

## 私信入口补全（2026-10-12 第二批，自定位「私信无发现入口 + mobile 未接」）
- [x] web：用户主页「发私信」按钮 + `/messages?peer=` 直开会话（f9122d5）
- [x] mobile：/messages 私信屏（会话列表 ↔ 聊天线，inverted 列表）、用户主页「私信」按钮、Profile Tab「我的私信」行、API 五函数（cc4a90b）

## community 端到端测试（2026-10-12 巡检推进批次，自定位自 community README 待实现 #1「服务编排、数据回放」）
- [x] 进程内全栈 e2e（`internal/e2e`）：真实 rest.Server + 全量路由 + SQLite + JWT 中间件 + realtime-hub 发布桩，编排旅程（话题→帖子→关注→点赞→评论→关注人→通知→已读→私信→回执）+ 数据回放幂等契约（574956b）
- [x] 顺带修复：`Like` 计数幂等（README 契约「幂等」此前只落在关系表，重复点赞 like_count 仍 +1；现只在关系新增时 +1，并发同用户重放不重计）（574956b）

## 观察项（CI 快照复核 2026-10-11，非红不动）
- [x] Actions 版本升级：四个 action 均已出新主版本并升级（checkout@v7 / setup-node@v7 / setup-go@v7 / pnpm-action@v6，node24 运行时）——2026-10-11 落地（`6d12e81`），CI/docs 双绿验证，run 日志 deprecation 警告清零；ios.yml 同批升级，macOS 位以手动点火 run 实测（见 backlog 同日登记）
- [x] Ubuntu 26 迁移评估——已完成（2026-10-12，评估零配置改动，动作门仍 10-19）。波及面已发布（runner-images#14748）：`ubuntu-latest` 2026-10-19 起数周内滚移 26.04.1，11-19 收官；工具差异表内 Docker Buildx/Rust/Java 等全同版本，内核 6.17→7.0.0。本仓 Ubuntu job（ci×2/docs×2/nightly）盘点：Go/Node/pnpm 全经 setup action 装载与镜像无关，无 docker、无 apt 依赖；**唯一镜像侧依赖 = nightly 的 CGO gcc**（mattn/go-sqlite3，24.04=gcc13 / 26.04=gcc14，该库对 gcc14 编译广泛验证）。结论：钉 `ubuntu-24.04` 已消除滚动期漂移，**不跟进标签滚动**；10-19 后可将 nightly 单工作流切 `ubuntu-26.04` 做 canary 实测（gcc14 + -race 全绿为门槛），11-19 收官后视 canary 结果再定全局升版，此前不动任何配置

验收：CI 零红；每项完成后 backlog.md 同步勾选/更新。
