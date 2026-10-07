# syntax=docker/dockerfile:1.9

FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /bin/user-service user.go

FROM debian:bookworm-slim
ENV TZ=Asia/Shanghai
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/user-service /usr/local/bin/user-service
COPY etc ./etc
COPY test.db ./data/users.db
ENV DATASOURCE=file:/app/data/users.db
ENV GAMECATALOG_BASE_URL=http://game-catalog:8890
EXPOSE 8888
ENTRYPOINT ["/usr/local/bin/user-service","-f","etc/user-api.yaml"]
