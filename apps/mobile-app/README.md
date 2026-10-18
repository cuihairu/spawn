# mobile-app

Expo / React Native 客户端（Expo 57 · React Native 0.86 · expo-router 文件式路由）。
详细进度与里程碑见 [`docs/mobile_plan.md`](../../docs/mobile_plan.zh.md)。

## 页面路由（`src/app/`，文件即路由）

- Tab（`src/app/(tabs)/`）：`index`（首页）、`discover`（发现，home/feed 聚合）、
  `community`（社区，含帖子搜索）、`stats`（战绩面板）、`profile`（我的）。
- 其余：`login`、`guides` / `guide/`（攻略列表与详情）、`guide-compose` / `compose`
  （攻略创作）、`game/`（游戏详情）、`topics`、`rankings`、`post/`（帖子详情）、
  `stats/`（单游戏战绩）、`user`（用户主页）、`notifications`（通知中心）、
  `messages`（私信）。

## 配置

- `EXPO_PUBLIC_API_URL`：api-gateway 地址；不设则开发期默认 `http://<devHost>:8800`
  （见 `src/api/client.ts`）。生产指向网关域名。
- 实时通道：当前未接 WS——通知未读数走 30s 轮询（`src/app/(tabs)/community.tsx`）
  + 本地通知（`src/lib/notifications.ts`）；接入网关 `/ws` 隧道列为规划
  （web 端已接，见 `apps/web-client/src/lib/realtime.ts`）。

## 常用脚本

```bash
npx expo start          # 开发服务（或 pnpm --dir apps/mobile-app start）
npm run typecheck       # tsc --noEmit
npm run lint            # eslint .
npm run prebuild:android        # 生成 android/ 原生工程
npm run build:android:release   # gradle assembleRelease 出包
```

## 状态

- M0–M3 功能切片与 M4 第一里程碑已落地；真机回归项（本地推送、深链分享卡、
  通知中心）待设备验证（本环境无模拟器/真机，CI 跑 lint + typecheck，
  另有 `expo export` 产物走查记录，见 mobile_plan）。
- iOS 填空（TestFlight 侧载/Universal Links/APNs）待外部凭据，走 macOS runner。
