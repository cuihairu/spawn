# Web Client 攻略与评论切片交付总结

> 历史快照，记录攻略 + 评论切片交付时的状态，构建数据为当时产物。
> 当前 web-client 另已包含社区浏览/发帖（`/community`、`/community/posts/:id`）与
> 网关可选接入，最新功能面见 [README.md](./README.md)。

## 交付内容

React Router 7 多页面路由，当时 5 个页面：首页（登录与推荐）、攻略列表、攻略详情、
攻略创建、攻略编辑，配套导航栏与页脚。`src/api/client.ts` 扩展 Content Service 的
攻略与评论 API（创建、更新、查询、发布、点赞），TypeScript 全量类型标注。
评论组件 `CommentList`/`CommentItem` 支持嵌套回复、点赞、作者删除。暗色主题加
响应式布局。权限边界：未登录可浏览，写操作要登录，作者只能改删自己的内容。
登录态写 LocalStorage，刷新不丢。

## 当时新增的文件

| 文件 | 行数 | 功能 |
|------|------|------|
| `src/pages/HomePage.tsx` | 170 | 首页组件 |
| `src/pages/GuidesPage.tsx` | 135 | 攻略列表 |
| `src/pages/GuideDetailPage.tsx` | 140 | 攻略详情 |
| `src/pages/GuideEditorPage.tsx` | 165 | 攻略编辑器 |
| `src/pages/guides.css` | 435 | 攻略页面样式 |
| `src/components/comments/CommentList.tsx` | 150 | 评论列表 |
| `src/components/comments/CommentItem.tsx` | 70 | 评论项 |
| `src/components/comments/comments.css` | 180 | 评论样式 |

另有 `src/App.tsx` 重构加路由导航，`src/api/client.ts` 扩展约 200 行。合计新增约
1600 行、8 个组件、12 个 API 函数、6 个类型接口。

## 构建产物（当时）

```
dist/index.html                   0.46 kB │ gzip:  0.29 kB
dist/assets/index-QZntB9vm.css   12.61 kB │ gzip:  2.87 kB
dist/assets/index-BUcQ64ah.js   246.11 kB │ gzip: 78.06 kB
```

tsc、`vite build`、eslint 当时全部通过。

## 启动前提

user-service（:8888）、game-catalog（:8890）、content-service（:8891）、
community（:8892，社区页面所需）在跑，然后：

```bash
cd apps/web-client
npm install
npm run dev
```

访问 http://localhost:5173。

## 未做（当时登记，至今仍缺）

攻略搜索、Markdown 编辑器、图片上传、用户个人主页、攻略收藏、通知系统。
后两项依赖后端能力，前端单独做不了。

> 更新（2026-10-10）：以上六项均已落地——攻略搜索/高级筛选（`GuidesPage` +
> `fetchGuides` 的 keyword/tag/sort）、Markdown 编辑器（`GuideEditorPage` +
> `src/lib/markdown.ts`，创建/编辑可选 text|markdown）、图片上传（`CommunityPage`
> 经网关 `POST /upload`）、用户个人主页（`UserProfilePage`）、攻略收藏
> （`favoriteGuide`/`fetchMyFavorites` 等四个 API + 详情页收藏按钮）、通知系统
> （`NotificationsPage` + 导航铃铛 + 未读徽标）。当前功能面与清单见
> [GUIDE.md](./GUIDE.md)（六项均已勾选）。
