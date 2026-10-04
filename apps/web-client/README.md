# Web Client

React 19 + TypeScript + Vite + React Router 7 构建的主站前端，直连四个后端服务
（可选经 api-gateway 聚合），覆盖登录/榜单、攻略创作与评论、社区浏览与发帖。

## 功能面

- **登录与推荐**：注册/登录（LocalStorage 持久化）、个性化推荐、精选榜单。
- **攻略与评论**：攻略列表/详情/创建/编辑（草稿/发布）、嵌套评论、点赞——详见 [GUIDE.md](./GUIDE.md)。
- **社区**：帖子流、帖子详情（发帖/点赞/分享）、话题圈子浏览。
- 未登录可浏览；写操作需登录（作者可编辑/删除自己的内容）。

## 服务集成与环境变量

直连四服务 + 可选网关，地址均可通过 `.env.local` 覆盖：

```
VITE_API_GATEWAY_URL=http://localhost:8800     # 配置后登录/推荐/精选优先走网关
VITE_USER_SERVICE_URL=http://localhost:8888
VITE_GAME_SERVICE_URL=http://localhost:8890
VITE_CONTENT_SERVICE_URL=http://localhost:8891
VITE_COMMUNITY_SERVICE_URL=http://localhost:8892
```

页面路由：`/`（首页）、`/guides`、`/guides/:id`、`/guides/new`、`/guides/:id/edit`、
`/community`、`/community/posts/:id`。

## 开发

```bash
cd apps/web-client
npm install
npm run dev
```

默认访问 `http://localhost:5173`。相关文档：[GUIDE.md](./GUIDE.md)（攻略与评论使用指南）、
[COMPLETION_SUMMARY.md](./COMPLETION_SUMMARY.md)（攻略/评论切片交付总结）。
