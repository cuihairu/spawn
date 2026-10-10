# Services

| 服务 | 职责概述 | 状态 |
| --- | --- | --- |
| `api-gateway` | 统一接入层（BFF 聚合 + 五路反代），鉴权透传与监控；GraphQL 未实现 | 已落地 |
| `user-service` | 账号、授权、好友、成就、积分、通知偏好等用户域能力（JWT 签发方） | 已落地 |
| `game-catalog` | 游戏库管理、榜单、标签、推荐 feed、版本元数据 | 已落地 |
| `content-service` | 攻略/资讯内容发布、审核、评论互动、收藏 | 已落地 |
| `community` | 帖子、话题圈子、互动、点赞收藏、关注流、站内通知 | 已落地 |
| `data-panel` | 战绩摄入（单调增量）、清洗、统计分析、玩家档案/榜单 API | 已落地 |
| `user-service-rpc` | 用户域 gRPC 通路（集群内部，etcd 注册） | 已落地 |
| `matchmaking` | 开黑组队、匹配大厅、房间状态同步、小游戏房间 | 规划（未落地） |
| `realtime-hub` | WebSocket/RTC、实时消息/语音信令、推送 fanout | 规划（未落地） |
| `crawler-jobs` | 爬虫调度器，抓取商店、赛事、公告等外部数据源 | 规划（未落地） |

> 上表「状态」列区分已落地与规划：「规划（未落地）」 服务目录暂不存在，
> 职责描述为设计目标，非现状。
> 建议统一 observability、配置管理、服务模板（如 NestJS/FastAPI/Go kit），并在 packages/data-models 中维护契约。
