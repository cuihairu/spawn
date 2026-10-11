# syntax=docker/dockerfile:1.9

FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/realtime-hub .

# 纯 Go 栈（stdlib + gorilla/websocket + jwt），无 CGO 依赖
FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/realtime-hub /usr/local/bin/realtime-hub
COPY etc ./etc
EXPOSE 8893
ENTRYPOINT ["/usr/local/bin/realtime-hub","-f","etc/realtime-hub.yaml"]
