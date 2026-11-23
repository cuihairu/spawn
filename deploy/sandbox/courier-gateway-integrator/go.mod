module github.com/cuihairu/spawn/deploy/sandbox/courier-gateway-integrator

go 1.27.4

// 冻结契约钉版：courier/gateway 是 courier 仓内的独立嵌套模块（非根模块），
// 由 proxy 解析为伪版本 v0.0.0-20261121074842-e745a81cfe5a —— 对应本地
// checkout HEAD e745a81，即本装配依赖的冻结 /v1/* 契约版本。courier 无
// 稳定 tag（仅 nightly 滚动），故以 commit 伪版本钉版；升级契约 = 显式
// 升 require。本模块不入 spawn go.work（避免与 spawn 的 toolchain 耦合），
// CI 不构建此沙箱工具；沙箱装配见 deploy/sandbox/README.md。
require github.com/cuihairu/courier/gateway v0.0.0-20261121074842-e745a81cfe5a
