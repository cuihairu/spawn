# syntax=docker/dockerfile:1.9

# 构建上下文为仓库根（compose 指定 context: .）：go.mod 的 replace
# ../../pkg/resilience 要求镜像内目录结构与仓库一致
# （/src/pkg/resilience、/src/services/content-service），单服务上下文装不下。
FROM golang:1.26 AS builder
WORKDIR /src
COPY pkg/resilience/go.mod pkg/resilience/go.mod
COPY services/content-service/go.mod services/content-service/go.sum services/content-service/
RUN cd services/content-service && go mod download
COPY pkg/resilience/ pkg/resilience/
COPY services/content-service/ services/content-service/
RUN cd services/content-service && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/content-service content.go

# 运行时含 glibc 与 sqlite3（CGO 驱动 mattn/go-sqlite3 需要）
FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/content-service /usr/local/bin/content-service
COPY services/content-service/etc ./etc
RUN mkdir -p /app/data
ENV DATASOURCE=file:/app/data/content.db
EXPOSE 8891
ENTRYPOINT ["/usr/local/bin/content-service","-f","etc/content-api.yaml"]
