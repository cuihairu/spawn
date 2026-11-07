# Source Layer Glossary

This glossary names concepts that recur across implementation boundaries. Each entry cites the source that defines the behavior, not a directory label.

| Term | English | Definition | Evidence |
|---|---|---|---|
| Gateway Route Registry | Gateway Route Registry | The server registers generated REST routes first, then adds the realtime WebSocket tunnel as a separate route family. | `services/api-gateway/gateway.go:48`, `services/api-gateway/gateway.go:54` |
| Upstream Catalog | Upstream Catalog | Gateway configuration maps five HTTP service families and an optional realtime hub to URLs before request dispatch. | `services/api-gateway/internal/config/config.go:11-34` |
| Buffered Proxy | Buffered Proxy | The gateway buffers request bodies under a 1 MiB cap before forwarding, converting the transport into fixed-length upstream requests. | `services/api-gateway/internal/proxy/upstream.go:45-63` |
| Per-Group Feed Degrade | Per-Group Feed Degrade | Home-feed groups fail independently: an upstream error empties one response group and records it as degraded instead of failing the aggregate. | `services/api-gateway/internal/logic/homefeedlogic.go:38-61` |
| Upload Store | Upload Store | Gateway-local disk storage accepts magic-number-checked uploads and resolves public filenames against a whitelist-safe path. | `services/api-gateway/internal/upload/upload.go:97-128` |
| JWT Claims Contract | JWT Claims Contract | User service signs identity claims; other Go services verify a shared claim structure without calling back to user service. | `services/user-service/internal/svc/servicecontext.go:31-38`, `services/game-catalog/internal/svc/servicecontext.go:64-75` |
| Dual DSN Switch | Dual DSN Switch | Persistence opens SQLite for `file:` data sources and MySQL otherwise, so local and deployed services share the model layer. | `services/user-service/internal/svc/servicecontext.go:43-58`, `services/game-catalog/internal/svc/servicecontext.go:34-47` |
| TTL + LRU Cache | TTL + LRU Cache | Model-local caches combine expiry and least-recently-used eviction to protect stores from repeated reads. | `services/user-service/internal/cache/ttlcache.go:15-104` |
| Client Resilience Client | Client Resilience Client | Content service's game-catalog client adds retry, circuit breaking, and Prometheus metrics around a bounded cross-service REST call. | `services/content-service/client/gamecatalog.go:18-22`, `services/content-service/client/breaker.go:22-68` |
| Realtime Frame Registry | Realtime Frame Registry | The hub owns in-memory WebSocket clients plus room presence and dispatches typed JSON frames. | `services/realtime-hub/internal/hub/hub.go:16-41` |
| Realtime Relay | Realtime Relay | Optional Redis fan-out transports hub events to other hub processes while preserving the in-process channel contract. | `services/realtime-hub/internal/hub/relay.go:44-119` |
| Internal Event Notifier | Internal Event Notifier | Community publishers emit service-originated events to the hub through a shared internal token rather than exposing business state to browsers. | `services/community/internal/realtime/publisher.go:14-74`, `services/community/internal/svc/service_context.go:154` |
| Shared Runtime Config | Shared Runtime Config | A packaged Makefile plus environment template gives client and service tasks a single local-development entry point. | `packages/config/make/common.mk:1-19`, `packages/config/env/.env.example:1-7` |
| Client Session Hook | Client Session Hook | Mobile authentication keeps a SecureStore-backed session and exposes login/logout/refresh state through React context. | `apps/mobile-app/src/auth/AuthContext.tsx:13-82` |
| Direct Service Fan-Out | Direct Service Fan-Out | Web clients retain explicit per-domain service URLs, so browser calls do not force all traffic through the gateway. | `apps/web-client/src/api/client.ts:116-128` |
