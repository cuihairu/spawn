# TODO

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
- [x] 文档一致性审计：门面 + 全仓文档 82 项差异清点修复，落 docs/审计-文档一致性.md（694140c；Go 8 模块 -race 全绿 + docs build 绿）
