# Community Service 实现文档

## 概述

Community Service 是 Tappi 社区平台的核心服务之一，提供帖子发布、话题圈子、用户互动和关注系统等功能。

## 当前状态

✅ **已完成**：
1. API 定义文件 (`community.api`)
2. 项目结构生成
3. 配置文件创建
4. Handler 和 Routes 生成
5. JWT 认证中间件（受保护接口需要 `Authorization: Bearer <token>`）
6. 基于内存的仓储层（Post/Topic/Follow，带种子数据）
7. JSON 文件持久化存储（topics/posts/follows，重启后可恢复）
8. 统一错误码与错误响应结构（`code` + `message`）
9. 业务逻辑层（发帖/改帖/删帖/列表/热门、话题创建/关注/取关/关注列表、用户关注/取关）

⏳ **待实现**：
1. 更完整的单元/集成测试

## API 端点

### 帖子相关
- `POST /api/v1/posts` - 创建帖子 🔒
- `GET /api/v1/posts` - 获取帖子列表
- `GET /api/v1/posts/:id` - 获取帖子详情
- `GET /api/v1/posts/hot` - 获取热门帖子
- `PUT /api/v1/posts/:id` - 更新帖子 🔒
- `DELETE /api/v1/posts/:id` - 删除帖子 🔒
- `POST /api/v1/posts/:id/like` - 点赞帖子 🔒
- `POST /api/v1/posts/:id/share` - 分享帖子 🔒

### 话题相关
- `POST /api/v1/topics` - 创建话题 🔒
- `GET /api/v1/topics` - 获取话题列表
- `GET /api/v1/topics/:id` - 获取话题详情
- `GET /api/v1/topics/following` - 获取关注的话题
- `POST /api/v1/topics/:topic_id/follow` - 关注话题 🔒
- `DELETE /api/v1/topics/:topic_id/follow` - 取消关注话题 🔒

### 用户关注
- `POST /api/v1/users/:user_id/follow` - 关注用户 🔒
- `DELETE /api/v1/users/:user_id/follow` - 取消关注用户 🔒

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

## 快速实现指南

由于完整实现代码量较大（预计1500+行），建议按以下顺序逐步完成：

### 第一阶段：核心功能（必须）
1. 实现 PostRepository - 帖子数据存储
2. 实现 TopicRepository - 话题数据存储
3. 实现 PostLogic - 帖子业务逻辑
4. 实现 TopicLogic - 话题业务逻辑
5. 完善 Auth 中间件

### 第二阶段：增强功能（重要）
1. 实现关注系统
2. 实现热门推荐算法
3. 添加数据验证
4. 实现分页功能

### 第三阶段：优化功能（可选）
1. 添加缓存层
2. 实现全文搜索
3. 添加性能监控
4. 编写完整测试

## 与其他服务的集成

### User Service
- 获取用户信息（作者名称等）
- 验证用户登录状态

### Content Service
- 共享评论系统
- 统一的点赞机制

### API Gateway
- 统一入口
- 请求路由和负载均衡

## 配置说明

`etc/community-api.yaml`:
```yaml
Name: community-api
Host: 0.0.0.0
Port: 8892              # 社区服务端口
Timeout: 30000

DataSource:
  TopicsFile: "data/topics.json"
  PostsFile: "data/posts.json"
  FollowsFile: "data/follows.json"

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

`Dockerfile`:
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o community community.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/community .
COPY --from=builder /app/etc ./etc
EXPOSE 8892
CMD ["./community", "-f", "etc/community-api.yaml"]
```

## 性能指标

预期性能（内存存储）：
- 创建帖子：< 10ms
- 查询列表：< 5ms
- 获取详情：< 3ms
- 点赞操作：< 2ms

## 未来规划

### 短期（1-2周）
- [ ] 完成所有 Logic 实现
- [ ] 添加数据持久化（MySQL/PostgreSQL）
- [ ] 实现完整的关注系统
- [ ] 添加单元测试

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

1. **线程安全**：所有内存存储操作必须使用 sync.RWMutex
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
- [User Service 文档](../user-service/README.md)
- [Content Service 文档](../content-service/README.md)
- [项目整体架构](../../docs/architecture/overview.md)

---

**当前版本**: v1.0 (基础框架)
**最后更新**: 2025-12-12
**维护状态**: ✅ MVP 可用（内存存储）
