# TODO

> 生成于 2026-10-11（无 todo 文件，来源：docs/backlog.md「待办」表 + CI 复核记录）。

## realtime-hub M3 子项（M1/M2/M3-1 已落地 2026-10-11；余项 2026-10-12 用户令连续推进，全部落地）
- [x] 隧道（tunnel）：网关 `/ws` 原始字节透传（Hijack + 双向 splice，JWT 校验仍在 hub，网关补 X-Forwarded-For）；顺带修 hub gorilla 默认 CheckOrigin 误伤浏览器跨源握手（web 走网关隧道默认化，`ws://localhost:8800`）（1d38531）
- [x] 已读回执：mark-all-read 成功后向本人全部连接广播 `notification.read`，多端徽标即时归零（3c06d95）
- [x] Redis 扩展（跨实例投递）：`RedisAddr` 配置即启用 pub/sub relay（尽力而为、回环剔除、坏地址启动期 fail fast；miniredis 全测）；go-redis v9 订阅惰性建立已用 Receive 确认堵漏（83d2fa5）
- [x] 推送桥（push bridge）：`PushWebhook` 离线推送接缝——单实例零投递用户 POST webhook（X-Push-Token，批量合并，fire-and-forget）；多实例本地零投递 ≠ 离线故不推（分布式推送决策留部署面）（a4b7ca9）
- [x] 语音信令：`signal` 上行无状态转发（offer/answer/ICE 透传，发端连接排除、多端全收；Relay 模式跨实例兜底；hub 不存房间状态）（acdc8ac）

## 观察项（CI 快照复核 2026-10-11，非红不动）
- [x] Actions 版本升级：四个 action 均已出新主版本并升级（checkout@v7 / setup-node@v7 / setup-go@v7 / pnpm-action@v6，node24 运行时）——2026-10-11 落地（`6d12e81`），CI/docs 双绿验证，run 日志 deprecation 警告清零；ios.yml 同批升级，macOS 位以手动点火 run 实测（见 backlog 同日登记）
- [ ] Ubuntu 26 迁移评估（ubuntu-latest 已钉 ubuntu-24.04，2026-10-19 起 runner-images#14748 迁移，波及面明确后再决定是否跟进；10-19 前维持观察不动作）

验收：CI 零红；每项完成后 backlog.md 同步勾选/更新。
