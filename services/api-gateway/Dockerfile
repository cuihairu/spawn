# syntax=docker/dockerfile:1.9

FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api-gateway gateway.go

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /bin/api-gateway /usr/local/bin/api-gateway
COPY etc ./etc
ENV USER_SERVICE_URL=http://user-service:8888
ENV GAME_SERVICE_URL=http://game-catalog:8890
EXPOSE 8800
ENTRYPOINT ["/usr/local/bin/api-gateway","-f","etc/gateway-api.yaml"]
