---
layout: home

hero:
  name: spawn
  text: 游戏社区平台
  tagline: 游戏库、攻略、社区三条业务线，Web 和移动两个客户端，六个 go-zero 微服务，一个仓库。
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
  - title: 游戏库与榜单
    details: game-catalog 管游戏、标签、精选和推荐。SQLite/MySQL 双驱动，筛选、排序、分页都在服务端做完。
  - title: 攻略与内容
    details: content-service 管攻略和评论，草稿、发布两个状态。攻略里的游戏信息跨服务从 game-catalog 拉取补齐。
  - title: 社区互动
    details: community 有帖子、话题、关注、点赞。关注流按时间倒序、同秒按 id 兜底，点赞单独一张关系表。
  - title: 接入与聚合
    details: api-gateway 管登录签发和四路反代。/home/feed 一次请求拉齐精选游戏、热帖、话题、攻略；单个上游挂了只降级那一组，其余照常返回。
  - title: 多端应用
    details: web-client 用 React 19 + Vite；mobile-app 用 Expo，带本地推送和深链分享卡，Android 出包走 gradle。小程序和运营后台还没动工。
  - title: 可观测性
    details: 六个服务各开一个 Prometheus /metrics 端口，日志 JSON 结构化进 Loki，deploy/ 下带 Prometheus、Loki、Grafana 的 compose。
---
