# Current topology and service boundaries (updated 2026-10-10)


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
| courier-adapter | Go / go-zero | 8897 | 9098 | GP-201 platform-account ↔ Courier adapter: `courier_game_mapping` (GP-002 mapping/capabilities), `platform_courier_binding` (GP-001 one-to-one binding, delegated-login random password stored as AES-GCM ciphertext), short-lived one-time game-launch tickets (never persisted), deregistration sync; admin roles authorised via game-catalog `/me`. GP-307 after-sales tickets (Courier SupportProvider proxy, contract Frozen v1.1 read-only): ticket create/my-list, `:ticketId` detail (handling record = Courier message stream PLAYER/AGENT ascending), append message, `/support/faq`; attribution = platform user ↔ Courier bound account (reuses GP-201 binding/delegated login); context facts built server-side (spawn_user_id/courier_account_id/game_id, serverId verified against game-catalog public servers, fail-open; orderId ownership verified against payments S2S order facts — self+game only, 400 on mismatch/unknown, fail-open when payments unreachable or token unset). GP-303 returning-player source: a successful game-session issuance records an account×game activity fact, persisted to a local outbox (`player_activity_outbox`, idempotent enqueue) then delivered to promotions `POST /internal/player-activity` (X-Internal-Token, exponential-backoff redelivery; no-op when PROMOTIONS_URL unset, report success/failure never blocks the session flow) | SQLite (default `file:data/adapter.db`) or MySQL; binding/mapping/audit + `adapter_audit_logs` + `player_activity_outbox` |
| payments | Go / go-zero | 8898 | 9099 | GP-202 payment access: product configuration (server-side pricing + merchant ownership), player order create/query (Courier `/v1/payments/*` read-only forward + amount/currency verification), channel S2S callback signature seam; the order source of truth stays in the Courier teller, no state machine copied. GP-203 delivery seam (order → server/role/product links, signed webhook landing `POST /internal/deliveries`, per-game endpoint+secret config, idempotent ledger, redelivery with backoff). GP-204 operations face (read-only `order_projection` copy, audit, refund/reversal registration plus a refund executor that drives PENDING→EXECUTED/REJECTED through the Courier v1.2 admin endpoint `POST /v1/payments/admin/orders/:orderId/refund` — deterministic rejects recorded, transient failures stay PENDING for background retry, an unconfigured admin token stays PENDING and never fabricates success). GP-205 vendor statements (aggregate actual paid / discount borne / refunds, split base, manual confirm, CSV export). GP-302 reward evaluation bridge, GP-303 first-charge/cumulative facts, GP-305 coupons (definition/holding/redemption + server-side quoting). GP-307 S2S order facts (GET /internal/orders/:orderId, X-Internal-Token fail-closed) feeding courier-adapter support-ticket order-ownership validation | SQLite (default `file:data/payments.db`) or MySQL; product/delivery/projection/audit/refund/statement/evaluation/coupon tables |
| promotions | Go / go-zero | 8902 | 9103 | GP-301 activities: state machine draft→published (publish creates an immutable `activity_versions.snapshot`, runtime references `current_version`)→disabled→revise; windows/timezone, game/server/channel/audience, trigger conditions, limits/budget, stacking groups, reward config (server-side config domain). GP-302 multiplier runtime (`POST /internal/reward-evaluations`, HMAC `X-Reward-Signature` verified before parsing, fail-closed 403 when no secret; explicit `mode=total`/`mode=extra` semantics from the server-side `base_quantity`), stack arbitration, limit consumption, `reward_grants` ledger idempotent on (order_id, activity_id) with the order+reward snapshot frozen in the row. GP-303 first-charge/cumulative/returning rules (returning-player activity store: `POST /internal/player-activity` from courier-adapter sessions, conditional-advance last/previous_active_at, per-activity absence threshold `audience.returning_threshold_days` default 30); GP-304 gift packs (config + code inventory + claims, player claim with platform JWT, S2S redemption fact); GP-306 activity messages on publish | SQLite (default `file:data/promotions.db`) or MySQL; activities + activity_versions + activity_audit_logs + reward_grants + gift_packs + gift_pack_codes + gift_pack_claims + activity_player_activity |
| api-gateway | Go / go-zero | 8800 | 9095 | BFF: own aggregate handlers + nine-way proxy (see §3) + unified image upload entry (see §3.1) | uploads volume (images on local disk, option A) |
| user-service-rpc | Go / gRPC | 8080 (gRPC) | 9096 | Internal gRPC channel (neither mobile nor web connects directly; the protoc code-generation gap is kept as-is) | — |
| realtime-hub | Go (not go-zero) | 8893 | — (no /metrics) | WebSocket hub: JWT-verified `/ws` upgrade, `/internal/publish` fanout (single instance or Redis pub/sub relay), `/internal/presence`, voice signalling passthrough | in-memory registry (optional Redis pub/sub) |

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

### 3.2 Proxy routes (internal/{users,games,community,content,datapanel,adapter,payments,promotions,realtime}; users/games/community/content/data-panel/adapter/payments/promotions are proxy.NewUpstream passthrough, realtime is a /ws tunnel)

`**` below marks a whole route family rather than a literal character.

| Prefix | Routes | Upstream |
|---|---|---|
| users | POST /auth/register, POST /auth/login, GET /users/:id (from 2026-10-11 login also goes through the proxy, envelope passed through verbatim) | user-service |
| games | GET /games, GET /games/:id (public), PATCH /games/:id/tags, GET /me, `admin/**` (roles/partners/reviews/ownership/publish/unpublish/versions/audit), `partner/**` (games/submit/versions/servers), GET /games/:id/servers, POST/DELETE /games/:id/servers/:serverId/reserve, GET /games/:id/reservations/me (GP-306 reservations; the S2S `/internal/games/:id/reservation-users` stays off the gateway) | game-catalog |
| community | `/api/v1/topics**`, `/api/v1/posts**` (including /posts/search, :id/comments and POST `:id/pin`), `GET /api/v1/announcements` (GP-307), `/api/v1/users/{following,likes}`, `/api/v1/users/:user_id/follow`, `/api/v1/notifications**`, `/api/v1/messages**`, `/api/v1/moderation**`, `/api/v1/design/**` (categories, breakdowns, breakdown detail, breakdown comments; DESIGN-001) — the full M1–M3 call surface | community |
| content | `/api/v1/guides**`, `/api/v1/comments**` | content-service |
| data-panel | `/api/v1/stats/users/:user_id/summary`, `/api/v1/stats/users/:user_id/games`, `/api/v1/stats/users/:user_id/games/:game_id`, POST `/api/v1/stats/records` | data-panel |
| adapter | POST /games/:id/session, POST /session/redeem, POST /games/:id/account/delete (+ /cancel), `/games/:id/support/**` (GP-307 tickets/FAQ; login and capability semantics live in courier-adapter), GET/PUT /admin/games/:id/mapping, GET /admin/courier/audit (domain-scoped gateway path, forwards to the upstream /admin/audit; GP-201; not registered when CourierAdapter.BaseURL is empty) | courier-adapter |
| payments | `/games/:gameId/orders**` + `/games/:gameId/skus` (order create/detail/list, SKU list), POST `/admin/products/:id` + PATCH `/admin/products/:id/enabled`, GET `/admin/games/:gameId/products`, POST `/admin/games/:gameId/products/sync` (GP-202 SKU sync: server-side pricing registered/repriced into teller via Courier v1.2 admin face, 503 when unwired), PUT/GET `/admin/games/:gameId/delivery`, GET `/admin/deliveries` (+ POST `:orderId/redeliver`, PATCH `:orderId/target`), GET `/admin/orders` (+ `/admin/orders/:orderId`, POST `/admin/orders/:orderId/refund`), GET `/admin/refunds`, GET `/admin/payments/audit` (domain-scoped gateway path, forwards to the upstream /admin/audit), POST/GET `/admin/statements` + `/admin/statements/:id` (+ PUT `:id/settlement`, POST `:id/confirm`/`:id/resolve`, GET `:id/export`), `/partner/statements**` (self-service list/differences/detail/export/confirm/dispute), `/coupons**` (POST `/coupons/:id/claim`, GET `/coupons/my`, admin create/update/`{publish,disable,revise}`/list/detail), POST `/callback/:channel` (GP-202/203/204/205/305/404; not registered when Payments.BaseURL is empty; `/internal/deliveries` and `/internal/orders/:orderId` (GP-307) are S2S-only and stay off the gateway) | payments |
| promotions | `/admin/activities**` (POST create, PUT `:id`, POST `:id/{publish,disable,revise}`, GET list/`:id`/`:id/versions`), GET `/admin/activity-audit`, GET `/admin/reward-grants` (GP-301/302); `/admin/gift-packs**` (POST create, PUT `:id`, POST `:id/{publish,disable,revise}`, GET list/`:id`, POST/GET `:id/codes`, GET `:id/claims`), GET `/admin/gift-claims` (GP-304 admin); player side (platform JWT): GET `/gift-packs`, GET `/gift-packs/:id`, POST `/gift-packs/:id/claim`, GET `/gift-claims/me` (GP-304; not registered when Promotions.BaseURL is empty; `/internal/reward-evaluations`, `/internal/player-activity` and `/internal/gift-pack-redemptions` are S2S-only and stay off the gateway) | promotions |
| realtime | GET /ws (WebSocket tunnel: Hijack + bidirectional splice, 502 pre-Hijack; not registered when RealtimeHub.BaseURL is empty) | realtime-hub |

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
  CGO_ENABLED=0 builds — api-gateway on distroless, realtime-hub on
  debian:bookworm-slim — the rest CGO + debian:bookworm-slim).
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
   + single-game detail (through the gateway's `/api/v1/stats/*` four-route
   proxy), the web `/stats` panel, and the compose/nightly/Prometheus wiring
   all landed at the same time; device verification is deferred.
4. realtime-hub: the realtime channel already tunnels through the gateway `/ws`
   (clients default to the gateway, the direct-connect variable is retained);
   matchmaking is still a planning placeholder.
5. Productionisation: TLS termination in front of the gateway (the current
   compose has no certificate), converging on HTTPS, tightening CORS.

## 7. Source layer analysis

The 2026-11-07 inventory of layers, modules, state, call channels, configuration, and lifecycle lives in [layer-analysis.md](layer-analysis.md); term definitions live in [glossary.md](glossary.md).
