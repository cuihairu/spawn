# Web Client 攻略与评论功能使用指南

## 功能概览

攻略的浏览、搜索、创作、编辑，评论回复与点赞都在这个客户端里，写操作需要登录。

## 页面路由

| 路由 | 功能 | 权限要求 |
|------|------|----------|
| `/` | 首页 - 游戏推荐和精选 | 无 |
| `/guides` | 攻略列表页 | 无 |
| `/guides/:id` | 攻略详情页 | 无 |
| `/guides/new` | 创建新攻略 | 需要登录 |
| `/guides/:id/edit` | 编辑攻略 | 需要登录且是作者 |
| `/community` | 社区帖子流（本文档范围外，另见 README） | 无 |
| `/community/posts/:id` | 社区帖子详情 | 发帖/点赞需登录 |

## 使用流程

### 1. 浏览攻略

**不需要登录**即可：
- 浏览所有已发布的攻略
- 查看攻略详情和评论
- 查看攻略的点赞数、阅读数和评论数

访问 `/guides` 页面即可查看攻略列表。

### 2. 创建攻略

**需要登录**后：
1. 点击首页或攻略列表页的"创建攻略"按钮
2. 填写攻略信息：
   - 游戏 ID（可选）
   - 标题（必填）
   - 内容（必填）
   - 标签（可选，用逗号分隔）
3. 选择操作：
   - **保存草稿**：保存但不发布
   - **保存并发布**：立即发布攻略

### 3. 编辑攻略

如果你是攻略的作者：
1. 在攻略详情页点击"✏️ 编辑"按钮
2. 修改攻略内容
3. 保存或发布更新

### 4. 发表评论

**需要登录**后：
1. 在攻略详情页底部找到评论区
2. 在文本框中输入评论内容
3. 点击"发表"按钮

### 5. 回复评论

1. 点击评论下方的"💬 回复"按钮
2. 输入回复内容
3. 发表回复

### 6. 点赞

**需要登录**后可以：
- 点赞攻略：点击攻略详情页的"👍 点赞"按钮
- 点赞评论：点击评论下方的"👍 赞"按钮

### 7. 删除评论

如果你是评论的作者：
- 点击评论下方的"🗑️ 删除"按钮
- 确认删除操作

## 筛选功能

在攻略列表页，登录用户可以：
- 点击"全部攻略"：查看所有已发布的攻略
- 点击"我的攻略"：只查看自己创建的攻略

## 页面特性

### 攻略列表页特性
- 卡片式展示，每张卡片包含：
  - 攻略标题
  - 标签（最多显示3个）
  - 内容预览（最多150字符）
  - 作者信息
  - 发布日期
  - 统计数据（阅读、点赞、评论数）

### 攻略详情页特性
- 完整的攻略内容展示
- 作者和发布时间信息
- 更新时间（如果有）
- 所有标签展示
- 点赞和统计数据
- 完整的评论系统

### 评论系统特性
- 嵌套回复支持
- 实时评论数更新
- 点赞功能
- 作者可删除自己的评论
- 时间戳显示

## 环境变量配置

在 `.env` 文件中配置服务地址：

```env
VITE_USER_SERVICE_URL=http://localhost:8888
VITE_GAME_SERVICE_URL=http://localhost:8890
VITE_CONTENT_SERVICE_URL=http://localhost:8891
VITE_COMMUNITY_SERVICE_URL=http://localhost:8892
VITE_API_GATEWAY_URL=http://localhost:8800  # 可选，配置后登录/推荐/精选优先走网关
```

## 快速启动

```bash
# 安装依赖
cd apps/web-client
npm install

# 开发模式
npm run dev

# 构建生产版本
npm run build

# 预览生产构建
npm run preview
```

## 技术栈

React 19、React Router 7、TypeScript strict、Vite，暗色主题一套样式走到底。

## API 集成

Web Client 集成了以下后端服务：

### Content Service API
- `GET /api/v1/guides` - 获取攻略列表
- `GET /api/v1/guides/:id` - 获取攻略详情
- `POST /api/v1/guides` - 创建攻略
- `PUT /api/v1/guides/:id` - 更新攻略
- `POST /api/v1/guides/:id/publish` - 发布攻略
- `POST /api/v1/guides/:id/like` - 点赞攻略
- `GET /api/v1/comments` - 获取评论列表
- `POST /api/v1/comments` - 发表评论
- `DELETE /api/v1/comments/:id` - 删除评论
- `POST /api/v1/comments/:id/like` - 点赞评论

### User Service API
- `POST /auth/login` - 用户登录
- `POST /auth/register` - 用户注册
- `GET /users/:id/recommendations` - 获取个性化推荐（user-service 跨服务调用 game-catalog）

### Game Catalog API
- `GET /games/featured` - 获取精选游戏

## 响应式设计

响应式布局：窄屏单列，宽屏网格，两端共用同一套暗色主题。

## 注意事项

1. **需要后端服务运行**：确保 user-service、game-catalog、content-service 与 community（社区页面）都在运行
2. **登录状态持久化**：登录状态会写入 LocalStorage，刷新页面后仍可继续使用
3. **草稿功能**：草稿保存后不会在攻略列表中显示，需要发布后才可见
4. **权限控制**：只有作者可以编辑和删除自己的攻略和评论

## 未来改进方向

- [x] 添加 LocalStorage 持久化登录状态
- [x] 支持 Markdown 格式的攻略内容（创建/编辑可选 text 或 markdown 格式，
  详情页按 format 渲染（marked + DOMPurify 净化防 XSS），编辑器带编辑/预览切换）
- [x] 添加图片上传功能（发帖附图经网关 `POST /upload` 落盘换 URL，详情页
  `resolveImageUrl` 拼网关来源渲染）
- [x] 实现攻略搜索和高级筛选（关键词搜索外新增标签筛选条与排序下拉：默认/
  最新/最多点赞/最多浏览，content-service 列表接口支持 `tag`/`sort` 参数）
- [x] 添加用户个人主页（公开 /users/:id：头像首字 + 昵称/@用户名 + 该作者已
  发布攻略列表，攻略详情页作者名可点击跳转）
- [ ] 支持攻略收藏功能
- [x] 添加通知系统（社区站内通知：导航铃铛 + /notifications 通知中心）
