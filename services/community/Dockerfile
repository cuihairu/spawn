FROM golang:1.25.4-alpine AS builder

LABEL stage=gobuilder

ENV CGO_ENABLED 0
ENV GOPROXY https://goproxy.cn,direct

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /app/community community.go

FROM alpine

RUN apk update --no-cache && apk add --no-cache ca-certificates tzdata
ENV TZ Asia/Shanghai

WORKDIR /app

COPY --from=builder /app/community /app/community
COPY etc /app/etc

EXPOSE 8892
CMD ["./community"]

