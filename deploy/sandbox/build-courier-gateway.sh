#!/usr/bin/env bash
# build-courier-gateway.sh 构建 courier-gateway-integrator（GP-005/GP-203
# 部署面装配）并以 courier-gateway 容器接入 spawn_default 网络。
#
# 与冻结 stock 网关的差别（本装配存在的意义）：
#   - COURIER_TELLER_SKUS 启动登记 teller 价格表（stock 二进制不登记，
#     下单必 404 PAYMENT_SKU_NOT_FOUND）；
#   - COURIER_PAYMENTS_DELIVER_WEBHOOK/SECRET 把 Deliver 钩子接到
#     payments POST /internal/deliveries（stock 二进制 nil = 记账即成功）。
#
# 用法（在 spawn 仓根执行）：
#   COURIER_TELLER_SKUS='[{"id":"p_sandbox_x","productId":"p_sandbox_x",
#     "form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}]' \
#   COURIER_PAYMENTS_CHANNEL_SECRET=... \
#   COURIER_PAYMENTS_DELIVER_WEBHOOK=http://payments:8898/internal/deliveries \
#   COURIER_PAYMENTS_DELIVER_SECRET=... \
#   ./deploy/sandbox/build-courier-gateway.sh
#
# 环境变量缺省：
#   COURIER_GATEWAY_ADDR=:8080（payments/compose 默认 URL 即容器名+8080）
#   COURIER_PAYMENTS_STATE=/tmp/courier-payments-state.json（容器内临时）
# 其余变量原样透传给容器。SKUs 的 id/金额/币种须与 payments
# payment_products 对齐（否则下单金额漂移 409）。
set -euo pipefail

cd "$(dirname "$0")"

# 模块钉 courier/gateway 伪版本（go.mod），GOWORK=off 隔离 spawn workspace，
# GOTOOLCHAIN=go1.27.4 对齐 courier 模块要求（本地无则自动取）。
export GOWORK=off
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.4}"

echo "==> go build courier-gateway-integrator"
# go build 须在模块目录内起跑（deploy/sandbox 本身不在任何模块，GOWORK=off
# 下向上找不到 go.mod 会报 cannot find main module）。
(cd courier-gateway-integrator && CGO_ENABLED=0 go build -o courier-gateway-integrator .)

echo "==> docker build"
docker build -t courier-gateway-integrator:latest ./courier-gateway-integrator

echo "==> replace courier-gateway container"
docker rm -f courier-gateway 2>/dev/null || true
docker run -d --name courier-gateway --network spawn_default \
  -e COURIER_GATEWAY_ADDR="${COURIER_GATEWAY_ADDR:-:8080}" \
  -e COURIER_PAYMENTS_CHANNEL_SECRET="${COURIER_PAYMENTS_CHANNEL_SECRET:-}" \
  -e COURIER_PAYMENTS_STATE="${COURIER_PAYMENTS_STATE:-/tmp/courier-payments-state.json}" \
  ${COURIER_TELLER_SKUS:+-e COURIER_TELLER_SKUS="${COURIER_TELLER_SKUS}"} \
  ${COURIER_PAYMENTS_DELIVER_WEBHOOK:+-e COURIER_PAYMENTS_DELIVER_WEBHOOK="${COURIER_PAYMENTS_DELIVER_WEBHOOK}"} \
  ${COURIER_PAYMENTS_DELIVER_SECRET:+-e COURIER_PAYMENTS_DELIVER_SECRET="${COURIER_PAYMENTS_DELIVER_SECRET}"} \
  ${COURIER_PAYMENTS_ADMIN_TOKEN:+-e COURIER_PAYMENTS_ADMIN_TOKEN="${COURIER_PAYMENTS_ADMIN_TOKEN}"} \
  courier-gateway-integrator:latest

echo "==> done. logs: docker logs -f courier-gateway"
