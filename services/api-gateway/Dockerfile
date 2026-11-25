# syntax=docker/dockerfile:1.9

# 构建上下文为仓库根（compose 指定 context: .）：go.mod 的 replace
# ../../pkg/resilience 要求镜像内目录结构与仓库一致
# （/src/pkg/resilience、/src/services/api-gateway），单服务上下文装不下。
FROM golang:1.26-alpine AS builder
# GOPROXY 构建期可配（proxy.golang.org 在部分网络下超时，传
# --build-arg GOPROXY=https://goproxy.cn,direct 走国内镜像；与 game-catalog
# Dockerfile 同口径）。
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY pkg/resilience/go.mod pkg/resilience/go.mod
COPY services/api-gateway/go.mod services/api-gateway/go.sum services/api-gateway/
RUN cd services/api-gateway && go mod download
COPY pkg/resilience/ pkg/resilience/
COPY services/api-gateway/ services/api-gateway/
RUN cd services/api-gateway && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api-gateway gateway.go

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /bin/api-gateway /usr/local/bin/api-gateway
COPY services/api-gateway/etc ./etc
ENV USER_SERVICE_URL=http://user-service:8888
ENV GAME_SERVICE_URL=http://game-catalog:8890
ENV CONTENT_SERVICE_URL=http://content-service:8891
ENV COMMUNITY_SERVICE_URL=http://community:8892
EXPOSE 8800
ENTRYPOINT ["/usr/local/bin/api-gateway","-f","etc/gateway-api.yaml"]
