# Source Layer Analysis

This model was read from source on 2026-11-07 at commit `bc9866d596d7c2e16fe65b08dda2a4af0a6e7596`. Layers describe responsibilities, not directory names.

## Inventory

| Layer | Modules / services | State | Configuration | Lifecycle |
|---|---|---|---|---|
| 1. Client Experience | Expo mobile UI; React web pages | SecureStore session; browser localStorage auth; realtime client connection | Expo public API URL; Vite service URLs | AuthProvider recovers SecureStore sessions before rendering; React root owns polling and WebSocket reconnection. Evidence: `apps/mobile-app/src/app/_layout.tsx:13-19`, `apps/mobile-app/src/auth/AuthContext.tsx:35-82`, `apps/web-client/src/App.tsx:23-119`, `apps/web-client/src/api/client.ts:116-128` |
| 2. Access Gateway | `api-gateway` | Uploads volume; local proxy buffers; generated route state | `etc/gateway-api.yaml`, upstream/JWT/upload environment | Loads config, builds service context, registers REST handlers, appends `/ws`, starts go-zero. Evidence: `services/api-gateway/gateway.go:38-55`, `services/api-gateway/internal/config/config.go:8-34`, `services/api-gateway/internal/upload/upload.go:78-128` |
| 3. Domain Services | `user-service`, `game-catalog`, `community`, `content-service`, `data-panel` | SQLite/MySQL per service; process-local TTL+LRU caches; no service-owned files | `internal/config/config.go` in each service plus `etc/*.yaml`; shared JWT/upstream/cross-service environment | Each follows flag → config load → service context (ping, create tables, seed) → handler registration → server start. Evidence: `services/user-service/user.go:32-58`, `services/game-catalog/game.go:30-52`, `services/community/community.go:30-51`, `services/content-service/content.go:30-52`, `services/data-panel/data-panel.go:30-51`; `services/user-service/internal/svc/servicecontext.go:43-81` |
| 4. Realtime Transport | `realtime-hub` | In-memory client/presence registry; optional Redis relay and webhook notifications | Custom YAML plus JWT/internal-token/Redis/webhook environment | Stdlib HTTP server starts after hub/relay/notifier wiring; WebSocket calls fan in and out. Evidence: `services/realtime-hub/realtime-hub.go:34-67`, `services/realtime-hub/internal/config/config.go:16-37`, `services/realtime-hub/internal/hub/relay.go:44-119` |
| 5. Cross-Service Transport | gateway aggregation clients; user→game client; content→game resilience client | Request-scoped bodies and metrics only | Gateway/user/content upstream URL and timeout settings | Calls run per request; content calls wrap retries and breaker transitions around REST; gateway feed groups degrade independently. Evidence: `services/api-gateway/internal/logic/homefeedlogic.go:20-61`, `services/user-service/internal/integration/gamecatalog_client.go:31-91`, `services/content-service/client/gamecatalog.go:18-68` |
| 6. Persistence | `model/` packages in five domain services | SQLite or MySQL rows; generic TTL+LRU model cache | DSN and driver choice flow through each service context | Service context opens/pings storage and invokes create/seed helpers before requests. Evidence: `services/game-catalog/internal/svc/servicecontext.go:34-58`, `services/game-catalog/model/gamemodel.go:72-119`, `services/community/internal/model/postmodel.go:24-69` |
| 7. Shared Tooling | `packages/config`, `packages/ui-kit`, `packages/data-models`, `packages/shared-utils`; `tools`; `tests` | Filesystem Makefiles/env samples; some packages are documented as planned | `packages/config/env/.env.example`, workspace scripts, common Makefile | Provides scripts/Make targets and planned shared contracts; no runtime ownership of business state. Evidence: `packages/config/make/common.mk:1-19`, `packages/config/env/.env.example:1-7`, `packages/README.md:1-9` |
| 8. Platform Observability | `platform`, `deploy`, Docker Compose | Monitoring configs and logs/metrics stores outside process memory | Compose env, Prometheus/Grafana/Loki/Promtail files | Compose wires service lifecycle, metrics ports, and collection agents. Evidence: `docker-compose.yaml:3-104`, `platform/README.md:1-9`, `deploy/prometheus/prometheus.yml:1-32` |

## Source Modules

- Mobile UI: `apps/mobile-app/src/app/_layout.tsx:13`, route groups under `apps/mobile-app/src/app`, API contract `apps/mobile-app/src/api/client.ts:9`.
- Web UI: `apps/web-client/src/App.tsx:23`, domain pages under `src/pages`, API contract `apps/web-client/src/api/client.ts:116`.
- Gateway: `services/api-gateway/gateway.go:44`, generated handlers in `internal/handler`, proxy in `internal/proxy`, aggregation in `internal/logic`, upload/realtime adapters in `internal/upload` and `internal/realtime`.
- Domain services: `services/user-service/user.go:32`, `services/game-catalog/game.go:30`, `services/community/community.go:30`, `services/content-service/content.go:30`, `services/data-panel/data-panel.go:30`.
- Realtime: `services/realtime-hub/realtime-hub.go:34`, frame/presence registry in `internal/hub`, optional relay in `internal/relay`, internal notifier in `internal/push`.
- Cross-service clients: gateway clients in `internal/integration`; user/game and content/game clients in their own integration/client packages.
- Persistence: model packages in each domain service; generic cache at `services/user-service/internal/cache/ttlcache.go:15`.
- Shared tooling and platform: `packages/config/make/common.mk:1`, `packages/README.md:1`, `platform/README.md:1`, `deploy/`.

## Call Channels

- Mobile → gateway REST and `/ws`: one gateway base URL is reused for all HTTP service constants (`apps/mobile-app/src/api/client.ts:9-13`).
- Web → selected services directly: browser clients resolve user/game/content/community endpoints from Vite runtime values (`apps/web-client/src/api/client.ts:116-128`).
- Gateway → five upstreams and realtime tunnel: compose URL mappings and gateway registration show the exact channel (`docker-compose.yaml:8-18`, `services/api-gateway/gateway.go:48-54`).
- User → game catalog: recommendations fetch through a configured REST client (`services/user-service/internal/integration/gamecatalog_client.go:31-91`).
- Content → game catalog: resilience-wrapped REST with retry, breaker, and metrics (`services/content-service/client/gamecatalog.go:18-68`, `services/content-service/client/breaker.go:22-68`).
- Community → realtime hub: internal-token REST publisher sends domain events; browsers only consume the resulting WebSocket stream (`services/community/internal/realtime/publisher.go:14-74`, `services/community/internal/svc/service_context.go:154`).

## Protocol Surfaces

- REST contracts: `.api` files under each Go service and the gateway, for example `services/api-gateway/gateway.api:1`, `services/community/community.api:1`, `services/game-catalog/game.api:1`.
- RPC file: `services/user-service-rpc/user.proto:14` defines only the go-zero Ping contract.
- Realtime JSON frames: typed `Frame` plus room/presence structures live in `services/realtime-hub/internal/hub/hub.go:16-41`; signals live in `services/realtime-hub/internal/hub/signal.go:5-36`.
- Upload protocol: multipart plus magic-number sniffing and deterministic gateway filenames (`services/api-gateway/internal/upload/upload.go:78-110`).

## Where Implementation, Schema, Protocol, and Copy Live

| Layer | Implementation | Configuration schema | Dedicated protocol files | User-facing copy |
|---|---|---|---|---|
| Client Experience | Expo routes and React pages under `apps/*/src` and `apps/*/app` | Build-time/runtime variables in app API clients; package scripts | Typed interfaces embedded in each API client | Screen strings and inline fallbacks in route/page components |
| Access Gateway | `services/api-gateway/gateway.go:44`, `internal/proxy`, `internal/logic`, `internal/upload`, `internal/realtime` | `services/api-gateway/internal/config/config.go:8`, `etc/gateway-api.yaml` | `services/api-gateway/gateway.api` | Gateway-owned share-card HTML template |
| Domain Services | Entry/service-context/logic/model files in each service | `internal/config/config.go` per service, `etc/*.yaml` | `.api` files per service | Error/status and moderation text in logic/model code |
| Realtime Transport | `services/realtime-hub/realtime-hub.go:34`, `internal/hub` | `services/realtime-hub/internal/config/config.go:16`, `etc/realtime-hub.yaml` | In-code JSON frame and signal structures | Connection/error JSON returned by hub handlers |
| Cross-Service Transport | `internal/integration`, user game client, content resilience client | Service config schemas carrying upstream URLs/timeouts | No dedicated cross-service file; generated `.api` plus in-code REST clients | Upstream-mapped messages from integrated services |
| Persistence | model packages in the five domain services | DSN fields in service config schemas | SQL embedded in model create/migration helpers | Database constraint errors surfaced as API envelopes |
| Shared Tooling | `packages/*`, `tools`, `tests` | `packages/config/env/.env.example`, Makefiles, workspace scripts | No dedicated business protocol file | Package READMEs |
| Platform Observability | Dockerfiles, compose, deploy configs, planned platform package | `docker-compose.yaml`, `deploy/**` | Prometheus/Loki/Grafana configuration formats | Dashboard/monitor labels |

## Why These Eight Layers

1. **Client experience is split because two transports already exist.** Mobile funnels every HTTP constant through one gateway URL (`apps/mobile-app/src/api/client.ts:9-13`), while web resolves four direct domain-service URLs (`apps/web-client/src/api/client.ts:116-128`); collapsing them would hide an intentional topology choice.
2. **The gateway is separate from domain logic because it owns transport-only state.** It buffers request bodies under a fixed cap (`services/api-gateway/internal/proxy/upstream.go:45-63`), keeps uploads on a gateway volume (`services/api-gateway/internal/upload/upload.go:97-128`), and tunnels WebSocket upgrades (`services/api-gateway/internal/realtime/tunnel.go:54-94`) without owning business tables.
3. **Domain services remain one layer because they share a concrete lifecycle and auth contract.** Five entry points perform the same flag/load/context/register/start sequence (`services/user-service/user.go:32-58`, `services/game-catalog/game.go:30-52`, `services/community/community.go:30-51`, `services/content-service/content.go:30-52`, `services/data-panel/data-panel.go:30-51`), and non-issuing services verify the same claims shape (`services/game-catalog/internal/svc/servicecontext.go:64-75`).
4. **Persistence is separated because one service-context pattern fans out to different model states.** The context decides SQLite/MySQL by DSN (`services/game-catalog/internal/svc/servicecontext.go:34-47`), then initializes model create/seed paths (`services/game-catalog/internal/svc/servicecontext.go:53-58`); that generic wiring is distinct from domain rules.
5. **Realtime is separate because its state and failure semantics differ from REST.** The hub owns in-memory presence and typed JSON frames (`services/realtime-hub/internal/hub/hub.go:16-41`), and Redis relay availability is optional rather than a domain-table dependency (`services/realtime-hub/internal/hub/relay.go:44-119`).
6. **Cross-service transport is distinct because resilience is uneven.** Content wraps its game-catalog call in retry, breaker, and metrics (`services/content-service/client/gamecatalog.go:18-68`, `services/content-service/client/breaker.go:22-68`), while gateway feed groups instead degrade independently (`services/api-gateway/internal/logic/homefeedlogic.go:38-61`); these policies are not equivalent to business logic.
7. **Shared tooling is a layer because it is build/runtime glue, not a product state owner.** Common Make targets drive workspace tasks (`packages/config/make/common.mk:1-19`) and the environment template centralizes development inputs (`packages/config/env/.env.example:1-7`).
8. **Platform/observability is separate because deployment and collection outlive process state.** Compose maps all service URLs, ports, and storage (`docker-compose.yaml:3-104`) while Prometheus, Loki, Grafana, and Promtail own their own collection configuration under `deploy/` (`deploy/prometheus/prometheus.yml:1-32`).

## Source-Read Inventory Limits

- `packages/ui-kit`, `packages/data-models`, and `packages/shared-utils` are represented by planning/README material rather than runtime source.
- `services/user-service-rpc` contains a Ping-only protobuf contract; no business RPC is wired into the current call graph.
