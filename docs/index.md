---
layout: home

hero:
  name: spawn
  text: 游戏社区平台
  tagline: 攻略、榜单与社区的 monorepo 实现——React Web 端、Expo 移动端与六个 go-zero 微服务在同一仓库内演进。
  actions:
    - theme: brand
      text: 快速上手
      link: /guide/getting-started
    - theme: alt
      text: 架构总览
      link: /architecture/overview
    - theme: alt
      text: 开发指南
      link: /development-guide

features:
  - icon: 🎮
    title: 游戏库与榜单
    details: game-catalog 维护游戏库、标签、精选与推荐；SQLite/MySQL 双驱动存储，列表筛选、排序与分页在服务端完成。
  - icon: 📖
    title: 攻略与内容
    details: content-service 承载攻略与评论，草稿—发布两态流转，跨服务拉取游戏信息补充标题与封面。
  - icon: 💬
    title: 社区互动
    details: community 提供帖子、话题、关注与点赞；关注流按时间与 id 双序合并，点赞关系独立成表。
  - icon: 🌐
    title: 接入与聚合
    details: api-gateway 负责 JWT 认证、路由反代与 BFF 聚合——/home/feed 一次拉齐精选游戏、热帖、话题与攻略，单上游故障逐组降级。
  - icon: 📱
    title: 多端应用
    details: web-client（React 19 + Vite）覆盖攻略与社区主站；mobile-app（Expo / React Native）含推送、深链分享卡与 Android 出包流程；小程序与运营后台在规划中。
  - icon: 📈
    title: 可观测性
    details: 六个服务各自暴露 Prometheus /metrics 指标端点，JSON 结构化日志接入 Loki，配套 Prometheus / Loki / Grafana 观测栈。
---
