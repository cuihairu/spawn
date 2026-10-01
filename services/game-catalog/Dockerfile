# syntax=docker/dockerfile:1.9

FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/game-catalog game.go

# 运行时含 glibc 与 sqlite3（CGO 驱动 mattn/go-sqlite3 需要）
FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/game-catalog /usr/local/bin/game-catalog
COPY etc ./etc
RUN mkdir -p /app/data
ENV DATASOURCE=file:/app/data/games.db
EXPOSE 8890
ENTRYPOINT ["/usr/local/bin/game-catalog","-f","etc/game-api.yaml"]
