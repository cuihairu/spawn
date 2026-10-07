# syntax=docker/dockerfile:1.9

FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/content-service content.go

# 运行时含 glibc 与 sqlite3（CGO 驱动 mattn/go-sqlite3 需要）
FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/content-service /usr/local/bin/content-service
COPY etc ./etc
RUN mkdir -p /app/data
ENV DATASOURCE=file:/app/data/content.db
EXPOSE 8891
ENTRYPOINT ["/usr/local/bin/content-service","-f","etc/content-api.yaml"]
