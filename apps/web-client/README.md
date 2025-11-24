# Web Client

基于 React + Vite 的轻量演示前端，用于串联 `user-service` 与 `game-catalog`：

- 登录用户账号，直接调用 `user-service` 的认证与推荐接口。
- 展示游戏目录服务的精选榜单与推荐结果。
- 对 API 地址进行参数化，可通过 `.env` 或运行环境变量覆盖 `VITE_USER_SERVICE_URL`、`VITE_GAME_SERVICE_URL`。

## 开发

```bash
cd apps/web-client
npm install
npm run dev
```

默认使用：

- `http://localhost:8888` - 用户服务
- `http://localhost:8890` - 游戏目录服务

若需要自定义，可在 `.env.local` 中配置：

```
VITE_USER_SERVICE_URL=http://localhost:8888
VITE_GAME_SERVICE_URL=http://localhost:8890
VITE_API_GATEWAY_URL=http://localhost:8800
```

配置 `VITE_API_GATEWAY_URL` 后，前端将优先通过该网关聚合接口，自动转发登录与推荐请求。
