# Services

| 服务 | 职责概述 |
| --- | --- |
| `api-gateway` | 提供统一接入层（REST/GraphQL/BFF），负责鉴权、聚合、节流与监控。 |
| `user-service` | 账号、授权、好友、成就、积分、通知偏好等用户域能力。 |
| `game-catalog` | 游戏库管理、榜单、标签、推荐 feed、版本元数据。 |
| `content-service` | 攻略/资讯内容发布、主题、审核、富文本/多媒体处理。 |
| `community` | 评论、帖子、话题圈子、互动、点赞收藏、社交通知。 |
| `matchmaking` | 开黑组队、匹配大厅、房间状态同步、小游戏房间。 |
| `realtime-hub` | WebSocket/RTC、实时消息/语音信令、推送 fanout。 |
| `data-panel` | 战绩抓取、清洗、统计分析、玩家档案/榜单 API。 |
| `crawler-jobs` | 爬虫调度器，抓取商店、赛事、公告等外部数据源。 |

> 建议统一 observability、配置管理、服务模板（如 NestJS/FastAPI/Go kit），并在 packages/data-models 中维护契约。
