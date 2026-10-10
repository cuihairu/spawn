# Backlog 台账（2026-10-10 立）

> 单一事实源：所有未完成项的状态与卡点。已完成项不在此列（见 git log 与各 README）。
> 维护纪律：每批增量落地后回填本表；新立任务先入表再动工。

## 待办（可写，按优先级）

| # | 项 | 卡点/依赖 | 备注 |
|---|----|----------|------|
| 1 | mobile 草稿发布闭环：编辑模式保存草稿后自动补发布（`updateGuide` 后接 `publishGuide`）；详情页作者可见草稿徽标 | 无 | 本批（notification/edit/publish）遗留漏洞：草稿在移动端无发布路径 |

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

## 验收打磨轨道

| 项 | 状态 |
|----|------|
| 文档补缺（通知/data-panel/收藏/七服务漂移） | ✅ 2026-10-10（`1e092fa`） |
| CI 全量绿 | ✅ 每次提交后核验（见各批报告） |
| 实机走查 | ⏸ 无设备，替代口径见上 |
| mobile M0–M3 + M4 第一里程碑 | ✅ 2026-10-09 前全部落地 |
