# Packages

- `ui-kit`: 设计系统、跨端组件库、主题 tokens、icon/font 资源。
- `data-models`: GraphQL schema、Protobuf、OpenAPI、TypeScript/Go 客户端。
- `shared-utils`: 工具函数、Hook、数据格式化、埋点 SDK、联机工具。
- `config`: pnpm workspace、tsconfig、eslint、prettier、commitlint、环境变量模板。

落地状态：只有 `config` 已落地，其余三个是规划条目，目录暂未创建。
packages 统一发布 version（例如 via changeset），由 apps 和 services 复用，避免重复造轮子。
