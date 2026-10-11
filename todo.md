# TODO

> 生成于 2026-10-11（无 todo 文件，来源：docs/backlog.md「待办」表 + CI 复核记录）。

## realtime-hub M3 子项（M1/M2/M3-1 已落地 2026-10-11，余项逐项再拍板）
- [ ] 隧道（tunnel）
- [ ] 已读回执
- [ ] Redis 扩展（跨实例投递）
- [ ] 推送桥（push bridge）
- [ ] 语音信令
（口径：每项需用户拍板后再开工，不主动启动）

## 观察项（CI 快照复核 2026-10-11，非红不动）
- [x] Actions 版本升级：四个 action 均已出新主版本并升级（checkout@v7 / setup-node@v7 / setup-go@v7 / pnpm-action@v6，node24 运行时）——2026-10-11 落地（`6d12e81`），CI/docs 双绿验证，run 日志 deprecation 警告清零；ios.yml 同批升级，macOS 位以手动点火 run 实测（见 backlog 同日登记）
- [ ] Ubuntu 26 迁移评估（ubuntu-latest 已钉 ubuntu-24.04，2026-10-19 起 runner-images#14748 迁移，波及面明确后再决定是否跟进；10-19 前维持观察不动作）

验收：CI 零红；每项完成后 backlog.md 同步勾选/更新。
