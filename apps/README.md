# Apps

- **web-client**: Web 端主站，承担攻略、榜单、社区浏览与互动，带通知中心与战绩面板。
- **mobile-app**: iOS/Android 客户端（Expo），覆盖游戏库、攻略（阅读/创作/编辑/收藏）、社区、战绩面板与站内通知。
- **mini-program**: 小程序端，支持快速查攻略、小游戏联机、轻量社交。
- **admin-console**: 运营后台前端，提供内容、榜单、活动配置和审核能力。

落地状态：web-client、mobile-app 已有代码；mini-program、admin-console 是规划目录，尚未创建。
多端共享设计体系、路由规范和国际化策略，统一通过 packages/ 下的工具支持。
