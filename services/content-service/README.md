# Content Service

内容服务，负责管理游戏攻略和评论系统。

## 功能特性

### 攻略管理
- 创建攻略
- 更新攻略
- 查看攻略详情
- 攻略列表查询（支持按游戏ID、作者ID、标签筛选）
- 发布攻略
- 点赞攻略
- 浏览计数

### 评论管理
- 发表评论
- 评论列表查询
- 删除评论
- 点赞评论
- 支持嵌套回复

## 技术架构

- **框架**: go-zero
- **数据存储**: 内存存储（JSON种子数据）
- **端口**: 8891

## 目录结构

```
content/
├── content.go              # 主入口文件
├── go.mod                  # Go模块定义
├── etc/
│   └── content-api.yaml    # 配置文件
├── internal/
│   ├── config/
│   │   └── config.go       # 配置结构
│   ├── handler/            # HTTP处理器
│   │   ├── routes.go
│   │   ├── createguidehandler.go
│   │   ├── updateguidehandler.go
│   │   ├── getguidehandler.go
│   │   ├── listguideshandler.go
│   │   ├── publishguidehandler.go
│   │   ├── likeguidehandler.go
│   │   ├── createcommenthandler.go
│   │   ├── listcommentshandler.go
│   │   ├── deletecommenthandler.go
│   │   └── likecommenthandler.go
│   ├── logic/              # 业务逻辑
│   │   ├── mapper.go
│   │   ├── createguidelogic.go
│   │   ├── updateguidelogic.go
│   │   ├── getguidelogic.go
│   │   ├── listguideslogic.go
│   │   ├── publishguidelogic.go
│   │   ├── likeguidelogic.go
│   │   ├── createcommentlogic.go
│   │   ├── listcommentslogic.go
│   │   ├── deletecommentlogic.go
│   │   └── likecommentlogic.go
│   ├── svc/
│   │   └── servicecontext.go  # 服务上下文
│   └── types/
│       └── types.go        # 类型定义
└── model/                  # 数据模型
    ├── guiderepository.go
    └── commentrepository.go
```

## API接口

### 攻略相关

#### 创建攻略
```bash
POST /api/v1/guides
Content-Type: application/json

{
  "game_id": "game-elden-ring",
  "title": "新手入门指南",
  "content": "攻略内容...",
  "summary": "攻略摘要",
  "cover_image": "https://example.com/cover.jpg",
  "tags": ["新手", "入门"]
}
```

#### 更新攻略
```bash
PUT /api/v1/guides/:id
Content-Type: application/json

{
  "title": "更新后的标题",
  "content": "更新后的内容"
}
```

#### 获取攻略详情
```bash
GET /api/v1/guides/:id
```

#### 攻略列表
```bash
GET /api/v1/guides?game_id=game-elden-ring&page=1&page_size=20
```

#### 发布攻略
```bash
POST /api/v1/guides/:id/publish
```

#### 点赞攻略
```bash
POST /api/v1/guides/:id/like
```

### 评论相关

#### 发表评论
```bash
POST /api/v1/comments
Content-Type: application/json

{
  "target_type": "guide",
  "target_id": 1,
  "content": "很棒的攻略！"
}
```

#### 评论列表
```bash
GET /api/v1/comments?target_type=guide&target_id=1&page=1&page_size=20
```

#### 删除评论
```bash
DELETE /api/v1/comments/:id
```

#### 点赞评论
```bash
POST /api/v1/comments/:id/like
```

## 启动服务

### 1. 安装依赖
```bash
cd services/content-service
go mod tidy
```

### 2. 启动服务
```bash
go run content.go
```

或者编译后运行：
```bash
go build -o content content.go
./content
```

### 3. 指定配置文件
```bash
go run content.go -f etc/content-api.yaml
```

## 数据存储说明

服务使用内存存储，数据在服务重启后会重置为种子数据。种子数据包含：

### 攻略种子数据
- 艾尔登法环新手指南
- Valorant枪法训练指南

### 评论种子数据
- 针对攻略的示例评论

## 配置说明

配置文件 `etc/content-api.yaml`：

```yaml
Name: content-api
Host: 0.0.0.0
Port: 8891

DataSource:
  GuidesFile: "data/guides.json"      # 攻略数据文件（可选）
  CommentsFile: "data/comments.json"  # 评论数据文件（可选）
```

如果数据文件不存在，服务会自动使用内置的种子数据。

## 后续改进建议

1. **用户认证集成**
   - 当前用户ID和用户名是硬编码的
   - 需要集成JWT中间件从token中获取用户信息

2. **游戏信息集成**
   - 创建攻略时应该调用game-catalog服务获取游戏名称
   - 可以增加游戏存在性验证

3. **持久化存储**
   - 可以将内存存储替换为MySQL或MongoDB
   - 保持Repository接口不变，只需实现新的Repository

4. **缓存优化**
   - 热门攻略可以使用Redis缓存
   - 减少重复查询

5. **搜索功能**
   - 集成ElasticSearch实现全文搜索
   - 支持按关键词搜索攻略内容

## 测试示例

### 创建攻略
```bash
curl -X POST http://localhost:8891/api/v1/guides \
  -H "Content-Type: application/json" \
  -d '{
    "game_id": "game-elden-ring",
    "title": "Boss战斗技巧",
    "content": "详细的Boss战斗攻略...",
    "tags": ["Boss", "战斗"]
  }'
```

### 获取攻略列表
```bash
curl http://localhost:8891/api/v1/guides?page=1&page_size=10
```

### 发表评论
```bash
curl -X POST http://localhost:8891/api/v1/comments \
  -H "Content-Type: application/json" \
  -d '{
    "target_type": "guide",
    "target_id": 1,
    "content": "非常实用的攻略！"
  }'
```
