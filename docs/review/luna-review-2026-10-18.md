# 独立审核报告（luna，2026-10-18）与整改台账

> **报告来源说明**：首次审核运行因 API 地域封锁中断（日志
> `~/.hermes/cache/scratch/opencode/spawn-210200.log`，退出码 1，无产出），
> 中断时的原始转录曾误提交为本文件（可在 git 历史 8758a4f 查阅，已删除）。
> 2026-10-18 21:49 用同一 harness（`opencode_task.sh`，模型
> `opencode-go/gpt-6-luna`）原样重跑成功（日志
> `~/.hermes/cache/scratch/opencode/spawn-214543.log`，基线 HEAD 0279691、
> 工作区未提交 0），下列三张表取自该次真实产出。
>
> **整改规则**（todo.md 第 22 项）：表一按风险排序修；表二以代码为准改文档、
> 禁止为对齐文档改业务代码；表三只做评估不实现（需用户拍板）。

## 表一 缺陷（按风险排序，全部已修）

| # | 风险 | 缺陷（审核定位） | 整改 | 证据 |
|---|------|------------------|------|------|
| 1 | P1 | 固定 JWT 密钥可伪造任意用户令牌（`docker-compose.yaml` 六服务共用 `tappi-user-service-jwt-secret-key-2024` 字面量，`REALTIME_INTERNAL_TOKEN` 同样字面量） | 双保险：① user-service / community 启动期守卫——空密钥拒绝启动、命中内置演示密钥大声告警（`services/user-service/utils/auth.go` `WellKnownDevJWTSecret` + `ValidateJWTSecret`；`internal/svc/servicecontext.go` 顶部；community 同款 `utils/auth.go` + `internal/svc/service_context.go` 顶部）；② compose 六处 `JWT_SECRET`、两处 `REALTIME_INTERNAL_TOKEN` 参数化为 `"${JWT_SECRET:-演示默认}"`——一键 quickstart 不变，生产用宿主环境变量覆盖（`docker-compose.yaml:14-16,43,58,70,73,86,97,99`） | `go test -race` 两服务全绿；`docker compose config -q` 通过；`JWT_SECRET=prod-secret docker compose config` 验证 6 处注入生效 |
| 2 | P1 | 任一登录用户可改他人邮箱/昵称（`services/user-service/internal/logic/updateuserinfologic.go` 只信 `req.Id`，无属主校验） | JWT 中间件注入的 `user_id` 与 `req.Id` 不一致 → `httperr.Forbidden`（403，真实状态码）；越权请求不改库 | 新测 `TestUpdateUserInfoLogic_CrossUserForbidden`（403 + 数据未被改动）、handler 层 `PUT /users/999` 403（`routes_integration_test.go`） |
| 3 | P1 | 默认配置把用户 ID 1（首位注册者）设为社区审核管理员（`services/community/etc/community-api.yaml:20` `AdminUserIds: [1]`，compose 未覆盖） | 启动期校验 `validateModerationAdmins`（`services/community/internal/svc/service_context.go` 尾部）：MySQL（生产姿态）+ 演示默认 `[1]` → 拒绝启动；SQLite（开发）放行仅告警；空名单视为显式停用审核处置、仅告警 | `TestValidateModerationAdmins` 六分支表测 + `TestNewServiceContext_MySQLDemoAdminsPanic`（`internal/svc/service_context_test.go`） |
| 4 | P1 | 已登录用户可枚举他人邮箱（`services/user-service/internal/logic/getuserinfologic.go` 无条件回传 `Email`） | 邮箱仅本人可见：`ctx user_id == req.Id` 才回传，他人查询邮箱置空、公开字段照常 | 新测 `TestGetUserInfoLogic_EmailVisibleOnlyToSelf`；handler 层 `GET /users/2`（他人）邮箱为空（`routes_integration_test.go`） |
| 5 | P2 | 举报处置非原子（`handle_report_logic.go` 先查后改 TOCTOU；`reportmodel.go` UPDATE 无 status 守卫，并发处置可覆盖状态） | `ReportStore` 新增 `UpdateStatusIfPending`（`UPDATE ... WHERE id=? AND status='pending'`，返回是否占位成功，`internal/model/reportmodel.go`）；处置动作改走条件更新，竞争失败/早已处置 → **409 Conflict**（`handle_report_logic.go` `alreadyHandledConflict`）；resolve 保留「先删内容后落状态」次序（resolved ⇒ 内容已删 的不变量不破） | 模型层双处置 `TestReportStore_UpdateStatusIfPending_DoubleHandle`、缺目标 `TestReportUpdateStatusIfPending_Missing`、关库 `TestReportUpdateStatusIfPending_ClosedDBErrors`；竞态 `TestHandleReport_ConcurrentHandleLosesRace`（过期快照 + 真实条件更新 → 409、先到者结果不被覆盖）；handler 流程测二次处置 409 |
| 6 | P2 | 私信已读统计失败静默返回 0（`mark_dm_thread_read_logic.go` CountUnread 出错仅记日志，响应 `Count:0` → 多端徽标被错误清零） | 统计失败 → `httperr.Internal`（500）显式报错，且不广播 `message.read` 帧 | 新测 `TestMarkDMThreadRead_CountUnreadErrorPropagates`（`internal/logic/message/message_read_countfail_test.go`：500 + 零发布） |
| 7 | P2 | Android 13+ 通知权限/渠道顺序颠倒（`apps/mobile-app/src/lib/notifications.ts` 先 `ensureReminderPermission()` 后建渠道，与其自身注释「权限弹窗要先存在至少一个通知渠道才会出现」矛盾） | `scheduleDailyCommunityReminder` 先 `ensureAndroidChannel()` 再问权限 | `npx expo lint` 0 错、`npx tsc --noEmit` 0 错、`npx expo export --platform android` 出包成功（android entry hbc 3.1MB） |
| 8 | P2 | 文档宣称 `/users/:id` 公开，实际未登录 401（skipAuth 名单不含该路径，`middleware/auth.go`） | 归入表二 #1 以文档对齐实现（见下） | — |

## 表二 文档与实现不一致（以代码为准改文档，全部已修）

| # | 不一致（审核定位） | 整改 | 证据 |
|---|--------------------|------|------|
| 1 | `apps/web-client/GUIDE.md:193` 称用户主页「公开 /users/:id」，实现为登录后可见 | 改为「登录后可见 /users/:id」 | GUIDE.md 个人主页条目；与表一 #4 一致（skipAuth 名单不含该路径） |
| 2 | `docs/development-guide.zh.md:1286` 「静态抓取六服务」，同文件 :1280 与 `deploy/prometheus/prometheus.yml` 均为七 target（9091–9097） | 六 → 七 | development-guide.zh.md 交付物清单首行 |
| 3 | `docs/realtime-hub.md:4` / `realtime-hub.zh.md:4` 状态「M1 kicked off / M1 开工」，实际 M1–M3 已于 2026-10-12 全落地（同文档 M3 小节） | 状态行改为「M1–M3 已落地（2026-10-12，见下方 M3 小节）」 | realtime-hub.md / realtime-hub.zh.md 头部引用块 |
| 4 | `README.md:79` / `README.zh.md:76` packages 条目宣称 ui-kit/data-models/shared-utils/config 四包齐备；`packages/README.md` 自述「只有 config 已落地，其余三个是规划条目」 | 两处 README 改为 as-built 口径：config 已落地，其余三个为规划条目 | README.md / README.zh.md packages 行 |

## 表三 值得补的能力（只评估不实现，需用户拍板）

| # | 能力 | 现状证据 | 评估 | 建议分期 |
|---|------|----------|------|----------|
| 1 | 限流/滥用防护 | 各服务无速率限制中间件 | 单实例 compose 形态下被刷的爆破面主要是登录/注册/举报/私信；go-zero 有内置 `maxConns` 与第三方限流中间件可接 | 二期：先网关层对 /auth/* 与举报入口加令牌桶；服务内 maxConns 兜底 |
| 2 | 邮箱验证与密码找回 | user-service 无邮件集成，注册即生效 | 需引入 SMTP/邮件服务商与令牌表，涉及新外部依赖与凭据管理 | 二期：先做「邮箱已验证」标记位 + 找回令牌表，SMTP 渠道后接 |
| 3 | 可靠事件投递（outbox/重试） | community 事件为 fire-and-forget（`internal/realtime/publisher.go` 发布失败仅记日志；relay 同款） | realtime-hub 停机时点赞/评论/已读帧丢失但主操作不受影响（契约如此）；要 at-least-once 需 outbox 表 + 投递器，成本主要在社区库 | 三期：与 realtime-hub 多实例运维成熟度一起做，先 outbox 表后 relay 重试 |
| 4 | 对象存储/CDN 与图片处理清理 | 上传落本地盘 `uploads/`（content/upload 路径），无缩略/清理 | 单机可跑；多实例或容量上来后需要 S3 兼容存储 + 生命周期策略 | 三期：抽象 Storage 接口，本地盘实现保底，S3 实现按部署形态启用 |
| 5 | 移动端构建与 E2E 纳入 CI | `.github/workflows/ci.yml` 未含 expo 构建与端到端用例 | 移动门禁目前靠本机 expo export 走查；CI 化需 EAS 或本地 gradle runner，计费/时长需权衡 | 二期：先 lint+tsc+export 三连进 CI（无签名产物），EAS 构建另立项 |
| 6 | 私信索引与游标分页 | `direct_messages` 查询按 `(sender,recipient)` 双向 OR + OFFSET 分页（`internal/model/directmessagemodel.go`） | 当前量级（演示/自托管）无感知；会话与消息上量后 OFFSET 深翻页变慢 | 三期：加 `(recipient_id, id)` / `(sender_id, id)` 索引 + created_at/id 游标，接口向后兼容 |

**拍板口径**：以上六项均不动代码；列此供用户逐项勾选立项。

## 整改验证

- `services/user-service`：`gofmt -l .` 无输出、`go build ./...` 通过、`go test -race ./...` 9 包全绿（含 handler 集成）。
- `services/community`：同口径全绿（handler/逻辑/模型/svc/utils）。
- `apps/mobile-app`：`npx expo lint`、`npx tsc --noEmit` 零错；`npx expo export --platform android` 出包成功。
- `docker-compose.yaml`：`docker compose config -q` 通过；`JWT_SECRET=prod-secret` 覆盖后 6 处生效。
- 文档改动：docs 站构建通过（本轮 `pnpm docs:build`，见 commit 记录）。
- 版本号未动；未打 tag。
