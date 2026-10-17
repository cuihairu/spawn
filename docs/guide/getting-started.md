# Getting started


Two ways to run spawn locally: one `docker compose` command for all services,
or start each side separately.

## Requirements

| Dependency | Version | Notes |
| --- | --- | --- |
| Go | ≥ 1.26 | The root `go.work` declares `go 1.26.0` (raised by the api-gateway dependency group), aggregating eight modules |
| Node.js | 24 | Same as CI |
| pnpm | 10.22 | Pinned by `packageManager` in the root `package.json` |
| Docker | Optional | One command starts all services and the observability stack |

Backend toolchain (as needed): `goctl` (go-zero code generation), `protoc`
with `protoc-gen-go` and `protoc-gen-go-grpc` — see the "Go-zero development
conventions" section of the development guide for installation.

## Get the code

```bash
git clone https://github.com/cuihairu/spawn.git
cd spawn
pnpm install
```

## Start all services with compose

```bash
docker compose up --build
```

Each service is a separate image (api-gateway and realtime-hub are
CGO_ENABLED=0 builds + distroless runtime, the rest are CGO builds +
debian:bookworm-slim). Data sources are injected via the `DATASOURCE`
environment variable; the default is a local SQLite file database with
automatic table creation and embedded seed data.

| Service | Business port | /metrics port |
| --- | --- | --- |
| user-service | 8888 | 9091 |
| game-catalog | 8890 | 9092 |
| content-service | 8891 | 9093 |
| community | 8892 | 9094 |
| realtime-hub | 8893 | — (no /metrics) |
| api-gateway | 8800 | 9095 |
| data-panel | 8896 | 9097 |
| user-service-rpc | 8080 (gRPC) | 9096 |

## Start the backend services separately

Without containers, each service runs as its own process, config files in its
own `etc/` directory:

```bash
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/content-service/content.go -f services/content-service/etc/content-api.yaml
go run services/community/community.go -f services/community/etc/community-api.yaml
go run services/data-panel/data-panel.go -f services/data-panel/etc/data-panel-api.yaml
go run services/realtime-hub/realtime-hub.go -f services/realtime-hub/etc/realtime-hub.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml
```

`api-gateway` is the unified external entry: JWT verification (checked locally
for `/upload`) and cross-service route proxying, plus the `/home/feed`
aggregation, all go through it; login is proxied to user-service, which issues
the JWT. In production, swapping `DATASOURCE` for a MySQL connection string
switches the storage — table creation and seed migration complete
automatically at startup.

## Start the web client

```bash
pnpm --dir apps/web-client run dev
```

web-client connects directly to the business services for development and
debugging; build it with `pnpm --dir apps/web-client run build` (`tsc -b` type
checking first, then the vite bundle).

## Start the mobile app

```bash
pnpm --dir apps/mobile-app start
```

mobile-app is Expo-based: after `expo start` boots the dev server, open it in a
simulator or Expo Go as prompted; the Android release packaging flow is
described in "Clients and services" (`docs/clients.md`).

## Further reading

- [Architecture overview](/architecture/overview) — the vision and service boundaries.
- [Current topology](/architecture/topology) — ports, the route surface and the auth data flow as built.
- [Development guide](/development-guide) — the complete manual from environment to deployment.
