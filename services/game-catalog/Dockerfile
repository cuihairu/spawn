# syntax=docker/dockerfile:1.9

FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/game-catalog game.go

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /bin/game-catalog /usr/local/bin/game-catalog
COPY etc ./etc
COPY data ./data
EXPOSE 8890
ENTRYPOINT ["/usr/local/bin/game-catalog","-f","etc/game-api.yaml"]
