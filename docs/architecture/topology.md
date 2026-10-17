# Current topology and service boundaries (updated 2026-10-10)

[English](/architecture/topology) | [简体中文](/zh/architecture/topology)

> This document records the currently landed service boundaries, ports, call
> topology and data flow; the vision-level architecture direction is in
> `overview.md` in the same directory. When the two conflict, this document
> wins.

## 1. Service inventory and boundaries

| Service | Language/framework | Business port | /metrics | Responsibility boundary | Storage as-built |
|---|---|---|---|---|---|
| user-service | Go / go-zero | 8888 | 9091 | Accounts (register/login/JWT issuance), user profile; pulls game recommendations cross-service via GameCatalogClient | MySQL (default DSN points at localhost 3306) |
| game-catalog | Go / go-zero | 8890 | 9092 | Game catalog, list filtering, details, featured/recommended leaderboards | SQLite (default `file:data/games.db`) or MySQL |
| content-service | Go / go-zero | 8891 | 9093 | Guide CRUD/publish/like, comments (with nested replies) | SQLite (default `file:data/content.db`) or MySQL |
| community | Go / go-zero | 8892 | 9094 | Post feed/hot list/keyword search, topic circles, follow relations (topics/users), like/share counters, follow feed, my likes, in-app notifications (four kinds: like/comment/reply/follow, deduped by a unique key), direct messages, report moderation | SQLite/MySQL (post_likes, notifications relation tables etc. created at startup) |
| data-panel | Go / go-zero | 8896 | 9097 | Stats ingest (JWT-protected, monotonic increment upsert) and player stats queries (public, win_rate/kd read-side to 4 decimal places) | SQLite/MySQL + TTL/LRU cache |
| api-gateway | Go / go-zero | 8800 | 9095 | BFF: own aggregate handlers + five-way proxy (see §3) + unified image upload entry (see §3.1) | uploads volume (images on local disk, option A) |
| user-service-rpc | Go / gRPC | 8080 (gRPC) | 9096 | Internal gRPC channel (neither mobile nor web connects directly; the protoc code-generation gap is kept as-is) | — |

Not landed (README planning placeholders): matchmaking, crawler-jobs,
admin-console, mini-program (realtime-hub was kicked off and landed on
2026-10-11).

## 2. Client access topology

Two access paths coexist without affecting each other:

```
web-client (React/Vite, :5173)
  └─ connects to services directly (src/api/client.ts hardcodes localhost:88xx)
       ├─ user-service :8888        (login/register/profile/recommendations)
       ├─ game-catalog  :8890       (game list/details/featured)
       ├─ content-service :8891     (guides/comments)
       └─ community :8892           (posts/topics/follows/likes/notifications)

mobile-app (Expo/RN)
  └─ unified through api-gateway :8800 (BFF phase 1, switched 2026-10-04)
       EXPO_PUBLIC_API_URL overrides the gateway domain;
       dev default host:8800 (Android emulator 10.0.2.2)
```

The web-client direct topology is unchanged; expanding gateway routes does not
force web to migrate.

## 3. api-gateway route surface

### 3.1 Own aggregate handlers (internal/handler + internal/logic)

| Route | Description | Upstream |
|---|---|---|
| GET /games/featured | Featured passthrough | game-catalog |
| GET /users/:id/recommendations | Cross-service recommendations (Bearer passed through) | user-service → game-catalog |
| GET /s/p/:id | Share-card jump page (html/template og meta + `spawn://` deep-link button) | community |
| GET /home/feed | BFF phase-2 list aggregation: featured games/hot posts/topics/guides fetched concurrently, per-group field trimming (large fields such as body and images are not sent, post body truncated to a 60-character summary); a single upstream failure degrades only that group to empty and marks `degraded`, no wholesale 5xx | game-catalog + community + content-service |
| POST /upload | Unified image upload entry (option A landed 2026-10-08): Bearer JWT verified locally (HS256, same secret as user-service), 512-byte magic-number sniffing whitelisting png/jpeg/webp/gif, 10 MB cap, random filename written with `O_EXCL` to the uploads volume, returns `/uploads/<hash>.<ext>`; business services store only the URL and never touch files | local disk (uploads volume) |
| GET /uploads/:file | Static hosting for uploaded images: filename regex whitelist (`^[0-9a-f]{32}\.(png\|jpg\|webp\|gif)$`) against path traversal, then read back with `ServeContent` | local disk (uploads volume) |

### 3.2 Proxy routes (internal/{users,games,community,content}, proxy.NewUpstream passthrough)

`**` below marks a whole route family rather than a literal character.

| Prefix | Routes | Upstream |
|---|---|---|
| users | POST /auth/register, POST /auth/login, GET /users/:id (from 2026-10-11 login also goes through the proxy, envelope passed through verbatim) | user-service |
| games | GET /games, GET /games/:id | game-catalog |
| community | `/api/v1/topics**`, `/api/v1/posts**` (including /posts/search and :id/comments), `/api/v1/users/{following,likes}`, `/api/v1/notifications**`, `/api/v1/messages**`, `/api/v1/moderation**` — the full M1–M3 call surface | community |
| content | `/api/v1/guides**`, `/api/v1/comments**` | content-service |

Proxy semantics: path+query passthrough, hop-by-hop headers stripped, request
body buffered and forwarded with a fixed length (1 MiB cap, 413 above. The
buffering works around the go-zero upstream defect of not parsing chunked
request bodies), status code/response body passed back verbatim (anonymous 401
and the like keep their semantics).

## 4. Authentication and cross-service data flow

```
Register/login:  client → gateway → user-service (issues the JWT, HS256 shared
                 secret "tappi-user-service-jwt-secret-key-2024", each service
                 overrides it via env=JWT_SECRET)
Authed request:  client → gateway (proxy passes Bearer through verbatim / own
                 handlers pass it through) → community etc. AuthMiddleware
                 verifies the signature strictly via utils.Auth.ParseToken
                 (the JWTClaims structure is consistent across services, so no
                 second call to user-service is needed)
Cross-service:   user-service → game-catalog (recommendations);
                 gateway → community/content/game-catalog (aggregation/share card)
```

## 5. Observability and deployment

- go-zero built-in metrics are exposed by each service's Prometheus agent at
  `/metrics` (ports in §1); api-gateway logs JSON to stdout, collected by
  Promtail into Loki (deploy/docker-compose.monitoring.yaml).
- `docker compose up --build` brings everything up in one command; each business
  service has its own Dockerfile (user-service-rpc has no image, deployed on a
  separate internal network by design; api-gateway/realtime-hub are
  CGO_ENABLED=0 builds + distroless runtime, the rest CGO + debian:bookworm-slim).
- Configuration: `etc/*.yaml`, upstream addresses overridable by environment
  variables (`USER_SERVICE_URL` etc.).

## 6. Evolution roadmap (aligned with the README "next-step suggestions")

1. BFF phase 3 (on demand): more aggregation shapes (the list aggregation
   landed at `/home/feed`; the field-trimming pattern can be copied); add
   caching to aggregate endpoints when needed.
2. Device regression: the Android release package can be built with local
   gradle (`pnpm --dir apps/mobile-app build:android:release`); local push and
   the deep-link share card await device verification.
3. data-panel has landed (2026-10-09, M4 first milestone): the mobile stats tab
   + single-game detail (through the gateway's `/api/v1/stats/*` five-way
   proxy), the web `/stats` panel, and the compose/nightly/Prometheus wiring
   all landed at the same time; device verification is deferred.
4. realtime-hub: the realtime channel already tunnels through the gateway `/ws`
   (clients default to the gateway, the direct-connect variable is retained);
   matchmaking is still a planning placeholder.
5. Productionisation: TLS termination in front of the gateway (the current
   compose has no certificate), converging on HTTPS, tightening CORS.
