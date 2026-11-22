# Apps

- **web-client**: Web 端主站，承担攻略、榜单、社区浏览与互动、设计拆解库（`/design` 品类聚合 + `/design/:id` 拆解详情与讨论区），带通知中心与战绩面板。
- **mobile-app**: iOS/Android 客户端（Expo），覆盖游戏库、攻略（阅读/创作/编辑/收藏）、社区、设计拆解（社区 Tab 入口，`/design` 与 `/design/[id]`）、战绩面板与站内通知。
- **mini-program**（规划中，目录未创建）: 小程序端，快速查攻略、小游戏联机、轻量社交。
- **admin-console**（规划中，目录未创建）: 运营后台前端，内容、榜单、活动配置和审核能力。

落地状态：web-client、mobile-app 已有代码；mini-program、admin-console 是规划目录，尚未创建。
packages/ 目前只落地 config（公共 make 目标、golangci-lint 配置、.env 模板）；
设计体系、路由规范、国际化策略属规划。
