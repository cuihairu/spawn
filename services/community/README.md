# Community Service 实现文档

## 概述

Community Service 是 spawn 社区平台的核心服务之一，提供帖子发布、话题圈子、用户互动和关注系统等功能。

## 当前状态

✅ **已完成**：
1. API 定义文件 (`community.api`)
2. 项目结构生成
3. 配置文件创建
4. Handler 和 Routes 生成
5. JWT 认证中间件（受保护接口需要 `Authorization: Bearer <token>`）
6. SQLite/MySQL 双驱动仓储层（Post/Topic/Follow/PostLike，表结构见 `internal/model`；空表启动自动写入内嵌种子）
7. 进程内 TTL+LRU 读缓存（话题/帖子单查；关注/点赞关系为写多读少不缓存）
8. 统一错误码与错误响应结构（`code` + `message`）
9. 业务逻辑层（发帖/改帖/删帖/列表/热门、话题创建/关注/取关/关注列表、用户关注/取关、关注流、我的点赞）
10. 基础单元/集成测试（仓储 + 认证/关注链路）

⏳ **待实现**：
1. 更完整的端到端测试（含服务编排、数据回放等）

## API 端点

### 帖子相关
- `POST /api/v1/posts` - 创建帖子 🔒
- `GET /api/v1/posts` - 获取帖子列表（支持 `author_id` 筛选）
- `GET /api/v1/posts/:id` - 获取帖子详情
- `GET /api/v1/posts/hot` - 获取热门帖子
- `GET /api/v1/posts/followed` - 关注流（关注的话题/用户的帖子） 🔒
- `PUT /api/v1/posts/:id` - 更新帖子 🔒
- `DELETE /api/v1/posts/:id` - 删除帖子 🔒
- `POST /api/v1/posts/:id/like` - 点赞帖子 🔒（幂等，落 post_likes 关系）
- `POST /api/v1/posts/:id/share` - 分享帖子 🔒

### 话题相关
- `POST /api/v1/topics` - 创建话题 🔒
- `GET /api/v1/topics` - 获取话题列表
- `GET /api/v1/topics/:id` - 获取话题详情
- `GET /api/v1/topics/following` - 获取关注的话题 🔒
- `POST /api/v1/topics/:topic_id/follow` - 关注话题 🔒
- `DELETE /api/v1/topics/:topic_id/follow` - 取消关注话题 🔒

### 用户关注与点赞记录
- `POST /api/v1/users/:user_id/follow` - 关注用户 🔒
- `DELETE /api/v1/users/:user_id/follow` - 取消关注用户 🔒
- `GET /api/v1/users/following` - 关注的用户列表 🔒
- `GET /api/v1/users/likes` - 我点赞过的帖子（按点赞时间倒序分页） 🔒

🔒 = 需要认证（已实现）

## 数据模型

### Post (帖子)
```go
type Post struct {
    Id           int64
    TopicId      int64      // 所属话题ID
    AuthorId     int64
    AuthorName   string
    Title        string
    Content      string
    Images       []string   // 图片URL列表
    Type         string     // discussion/question/share
    Tags         []string
    ViewCount    int64
    LikeCount    int64
    CommentCount int64
    ShareCount   int64
    IsPinned     bool       // 是否置顶
    IsHot        bool       // 是否热门
    Status       string     // draft/published/deleted
    CreatedAt    string
    UpdatedAt    string
}
```

### Topic (话题)
```go
type Topic struct {
    Id            int64
    Name          string
    Description   string
    Icon          string
    CoverImage    string
    PostCount     int64
    FollowerCount int64
    IsOfficial    bool      // 是否官方话题
    CreatedAt     string
    UpdatedAt     string
}
```

## 技术特性

### 热门算法
帖子热度计算公式：
```
热度分 = (点赞数 * 3 + 评论数 * 5 + 分享数 * 7 + 浏览数) / 时间衰减因子
```

时间衰减：
- 24小时内：衰减因子 = 1
- 24-72小时：衰减因子 = 2
- 3-7天：衰减因子 = 4
- 7天以上：衰减因子 = 8

### 关注系统
- 用户可以关注话题和其他用户
- 关注后可以看到相关内容的动态
- 支持双向关注（互相关注）

### 权限控制
- 游客：可浏览帖子和话题
- 登录用户：可发帖、评论、点赞、关注
- 作者：可编辑/删除自己的帖子
- 管理员：可管理所有内容（待实现）

## 与其他服务的集成

> 说明：community 是独立域，**无出向调用**——JWT 用共享密钥本地校验（密钥来自
> `Auth.JWTSecret`，user-service 签发），不回调 user-service 验证身份。

### User Service
- 仅共享 JWT 密钥做本地令牌校验（无网络调用）

### Content Service
- 评论/点赞体系各自独立：content-service 管攻略评论，community 管帖子点赞记录（post_likes）

### API Gateway
- 统一入口：`/api/v1/topics**`、`/api/v1/posts**`、`/api/v1/users/{following,likes}`、
  `/api/v1/users/:user_id/follow` 反代路由（见 `services/api-gateway/internal/community/routes.go`）；
  另有 integration client（热帖/话题列表/帖子详情）供网关 `/home/feed` 聚合与 `/s/p/:id` 分享卡

## 配置说明

`etc/community-api.yaml`:
```yaml
Name: community-api
Host: 0.0.0.0
Port: 8892              # 社区服务端口
Timeout: 30000

# 社区数据库：DSN 含 file:/.db 走 SQLite（默认本地文件，零配置），
# 生产用 MySQL 连接串 + DATASOURCE 环境变量覆盖。空表启动时自动写入内嵌种子。
MySQL:
  DataSource: "file:data/community.db"

Auth:
  JWTSecret: your-secret-key-change-in-production
```

## 启动服务

```bash
cd services/community
go run community.go -f etc/community-api.yaml
```

## 测试示例

### 创建话题
```bash
curl -X POST http://localhost:8892/api/v1/topics \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "name": "《黑神话：悟空》",
    "description": "讨论黑神话相关的一切",
    "icon": "https://example.com/icon.png"
  }'
```

### 创建帖子
```bash
curl -X POST http://localhost:8892/api/v1/posts \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "topic_id": 1,
    "title": "新手入坑指南",
    "content": "分享一些新手必知的技巧...",
    "type": "discussion",
    "tags": ["新手", "攻略"]
  }'
```

### 获取热门帖子
```bash
curl http://localhost:8892/api/v1/posts/hot?limit=10
```

## Docker 支持

`Dockerfile`（CGO 构建，运行时含 sqlite3；数据落 `/app/data/community.db`）：
```dockerfile
FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/community community.go

FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/community /usr/local/bin/community
COPY etc ./etc
RUN mkdir -p /app/data
ENV DATASOURCE=file:/app/data/community.db
EXPOSE 8892
ENTRYPOINT ["/usr/local/bin/community","-f","etc/community-api.yaml"]
```

## 性能指标

预期性能（SQLite 本地文件 + 进程内缓存）：
- 创建帖子：< 10ms
- 查询列表：< 5ms
- 获取详情（缓存命中）：< 1ms
- 点赞操作：< 5ms

## 未来规划

### 短期（1-2周）
- [x] 完成所有 Logic 实现
- [x] 数据持久化（SQLite/MySQL 双驱动 + 进程内读缓存）
- [x] 实现完整的关注系统
- [x] 添加单元测试

### 中期（1-2月）
- [ ] 实现通知系统
- [ ] 添加内容审核功能
- [ ] 实现图片上传服务
- [ ] 优化热门推荐算法

### 长期（3-6月）
- [ ] 实现实时推送
- [ ] 添加全文搜索（Elasticsearch）
- [ ] 实现内容推荐系统（ML）
- [ ] 支持视频内容

## 开发注意事项

1. **线程安全**：仓储层并发安全由数据库与连接池钳制（`SetMaxOpenConns(1)`，规避 SQLite 写锁冲突）保证；进程内缓存自身并发安全
2. **数据验证**：严格验证用户输入，防止XSS和注入攻击
3. **性能优化**：对热点数据使用缓存
4. **错误处理**：统一的错误码和错误信息
5. **日志记录**：记录关键操作和异常情况

## 贡献指南

如需完整实现，请按照以下步骤：

1. Fork 项目
2. 创建 feature 分支
3. 实现对应的 Logic 文件
4. 编写单元测试
5. 提交 Pull Request

## 相关文档

- [API 详细文档](./community.api)
- [开发指南（六服务总览）](../../docs/development-guide.md)
- [Content Service 文档](../content-service/README.md)
- [项目整体架构](../../docs/architecture/overview.md)

---

**当前版本**: v1.2 (SQLite/MySQL 双驱动 + M3 关注流/我的点赞)
**最后更新**: 2026-10-04
**维护状态**: ✅ 可用（SQLite 默认 / MySQL 生产）
