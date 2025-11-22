# Tests

- `contracts/`: 对 BFF/服务的契约测试，确保 schema 向前兼容。
- `integration/`: 跨服务 e2e 测试、数据流回归。
- `performance/`: 压力测试、容量评估、实时链路延迟监控脚本。
- `compliance/`: 安全、隐私、内容合规扫描。

> 建议通过统一的 test runner（如 Turborepo/pnpm workspace）调度，结合 CI matrix 自动运行。
