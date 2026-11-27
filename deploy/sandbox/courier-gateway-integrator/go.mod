module github.com/cuihairu/spawn/deploy/sandbox/courier-gateway-integrator

go 1.27.4

// 冻结契约钉版：courier/gateway 是 courier 仓内的独立嵌套模块（非根模块），
// 由 proxy 解析为伪版本 v0.0.0-20261127083713-ffca3718aa05 —— 对应
// courier checkout ffca371（2026-11-27 提交契约 v1.3：payments admin face
// 延续 v1.2 四端点并对 SKU 登记接受可选 gameId/env scope——scoped SKU 只在
// 登记 scope 可见可购、跨 scope 按不存在处理，空 scope = 全局零行为变化），
// 即本装配依赖的冻结 /v1/* 契约版本。courier 无稳定 tag（仅 nightly 滚动），
// 故以 commit 伪版本钉版；升级契约 = 显式升 require。本模块不入 spawn
// go.work（避免与 spawn 的 toolchain 耦合），CI 不构建此沙箱工具；
// 沙箱装配见 deploy/sandbox/README.md。
require github.com/cuihairu/courier/gateway v0.0.0-20261127083713-ffca3718aa05
