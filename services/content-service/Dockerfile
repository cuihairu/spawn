FROM golang:1.25.4-alpine AS builder

LABEL stage=gobuilder

ENV CGO_ENABLED 0
ENV GOPROXY https://goproxy.cn,direct

WORKDIR /build

# Copy go mod files
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy source code
COPY . .

# Build
RUN go build -ldflags="-s -w" -o /app/content content.go

FROM alpine

RUN apk update --no-cache && apk add --no-cache ca-certificates tzdata
ENV TZ Asia/Shanghai

WORKDIR /app

# Copy binary and config
COPY --from=builder /app/content /app/content
COPY etc /app/etc

CMD ["./content"]
