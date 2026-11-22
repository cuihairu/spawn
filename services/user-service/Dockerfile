# syntax=docker/dockerfile:1.9

# 构建上下文为仓库根（compose 指定 context: .）：go.mod 的 replace
# ../../pkg/resilience 要求镜像内目录结构与仓库一致
# （/src/pkg/resilience、/src/services/user-service），单服务上下文装不下。
FROM golang:1.26 AS builder
# 模块代理可用 --build-arg GOPROXY=... 覆盖（默认官方，不改默认行为）
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
WORKDIR /src
COPY pkg/resilience/go.mod pkg/resilience/go.mod
COPY services/user-service/go.mod services/user-service/go.sum services/user-service/
RUN cd services/user-service && go mod download
COPY pkg/resilience/ pkg/resilience/
COPY services/user-service/ services/user-service/
RUN cd services/user-service && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/user-service user.go

FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/user-service /usr/local/bin/user-service
COPY services/user-service/etc ./etc
COPY services/user-service/test.db ./data/users.db
ENV DATASOURCE=file:/app/data/users.db
ENV GAMECATALOG_BASE_URL=http://game-catalog:8890
EXPOSE 8888
ENTRYPOINT ["/usr/local/bin/user-service","-f","etc/user-api.yaml"]
