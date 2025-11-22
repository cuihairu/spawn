# Config Package

约定统一的工具链与环境配置，避免每个服务重复定义。

## Go 环境

- Go version: `1.25.x`
- `go.work` 位于仓库根目录，`use` 需要包含各服务的 go module。
- 命令行工具（`goctl`, `protoc-gen-go`, `protoc-gen-go-grpc`）安装后请将 `$(go env GOPATH)/bin` 加入 `PATH`。

## 配置模板

- `env/.env.example`: 统一本地/测试环境变量模板。
- `lint/.golangci.yaml`: 通用静态检查配置。
- `make/common.mk`: 常用 make 目标（`tidy`, `lint`, `test`, `run` 等）。

> 后续随着服务增多，在此维护共享配置并在文档中引用，保证脚手架一致性。
