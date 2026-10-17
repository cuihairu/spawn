# realtime-hub profile (approved 2026-10-11)

[English](/realtime-hub) | [简体中文](/zh/realtime-hub)

> Status: M1 kicked off. Decision record in `docs/backlog.md` to-do #1;
> architectural position in `docs/architecture/overview.md`.

## Goal

One realtime push channel: in-app notifications delivered immediately (replacing
the delay and idle spin of the web client's 30s polling), later carrying realtime
semantics such as presence / read receipts / direct messages. Mobile local push
(expo) is unaffected; a remote push bridge is a long-term item.

## Current state and constraints

- **The gateway proxy cannot carry WS**: api-gateway's upstream forwarding is a
  hand-rolled buffered implementation (read body → rebuild request →
  `http.Client` round trip, `internal/proxy/upstream.go`) with no Hijack. M1/M2
  clients connected to the hub directly; **M3 added the `/ws` raw-byte
  passthrough tunnel** (Hijack + bidirectional splice, `internal/realtime`), and
  clients now default to the gateway, with direct connection
  (`VITE_REALTIME_HUB_URL`) retained.
- Port: 8893 (hub WS/HTTP); 9091-9097 are already taken by the services'
  /metrics, so the hub's metrics face is not opened in M1.
- Auth: the same HS256 JWT as the services (shared `JWT_SECRET`, claims shape
  same as `utils.JWTClaims`). Browsers cannot send custom headers on a WS, so
  the handshake token goes in the query (local-deployment posture — mask it in
  access logs); Origin is unrestricted (the JWT is the only admission gate, and
  the token does not travel automatically across origins).
- Starts as a single-instance in-memory registry; horizontal scaling (Redis
  pub/sub) landed in M3 (`RedisAddr` config enables it, best-effort with
  loopback exclusion).

## Interface design

| Endpoint | Method | Notes |
|------|------|------|
| `/health` | GET | Liveness, 200 |
| `/ws?token=<jwt>` | GET(Upgrade) | Verifies JWT on handshake; on failure returns 401 and refuses (before the Upgrade, so no half-open connection); on success enters the registry (userId → multiple connections) |
| `/internal/publish` | POST | Service-to-service delivery. Body `{"user_ids":[..],"event":{"type":..,"payload":..}}`; header `X-Internal-Token` must match the config (unconfigured = 403 refusal), and delivery failure never affects the caller's main path |

- Frame format: JSON text frame `{"type":"notification","data":{...}}`; the
  server keeps connections alive with ping/pong (30s ping / 60s read timeout,
  pong refreshes it).
- Backpressure: each connection has a bounded send queue (16); overflow drops
  and counts the loss in a log. Publishers are never blocked.
- Only `notification` is defined as an event type in M1; later types are
  registered incrementally and remain backward compatible.

## Phases

### M1 skeleton (done, commit c8f978e)

`services/realtime-hub` as a standalone Go module (module path
`github.com/tappi/tappi/services/realtime-hub`, Go 1.26, websocket library
`github.com/gorilla/websocket`): config / main / hub registry / ws handler /
publish handler; included in `go.work` (CI discovers modules dynamically via
go.work, so -race coverage is automatic); compose service plus
`JWT_SECRET`/`REALTIME_INTERNAL_TOKEN` injection; service README.

### M2 integration (done, 2026-10-11)

- community: after a notification is created (like/comment/reply/follow — four
  trigger points), publish fire-and-forget; write failures are only logged (same
  contract as notification writes).
- web: the notification center gets realtime +1 over WS; automatic reconnect on
  disconnect (exponential backoff) with fallback to 30s polling (the existing
  polling code is kept as the safety net, so an unreachable hub does not break
  the feature).

#### M2 landing record

- community: `internal/realtime` publisher (empty Url = no-op, 3s timeout
  goroutine, failures only logx), `Realtime` config section
  (`REALTIME_HUB_URL`/`REALTIME_INTERNAL_TOKEN` env vars); the four trigger
  points send a frame only after the notification is persisted, and do not send
  on persistence failure (no ghost red dots).
- web: `src/lib/realtime.ts` (exponential-backoff reconnect 1s→30s capped, silent
  degradation); the app bell re-fetches the unread count as soon as a frame
  arrives. VITE_REALTIME_HUB_URL had been in the compose build args but was
  removed after the M3 tunnel landed (web defaults to the gateway, the direct
  variable is retained).
- End-to-end smoke test (measured on compose): register u1/u2 → u1 posts → u1
  opens a WS → u2 likes → u1 receives a `{"type":"notification",
  "data":{...like_post...}}` frame; after stopping the hub, likes still return
  200 and publish failures only produce an error log.

### M3 long-term → all landed (2026-10-12, user order to push straight through)

Gateway WS tunnel, read-receipt broadcast, Redis pub/sub horizontal scaling,
mobile remote-push bridge, voice/teaming signalling (matching the realtime-hub
vision in overview.md).

#### Remaining M3 items — landing record (2026-10-12)

- **Gateway WS tunnel** (`1d38531`): the gateway `/ws` raw-byte passthrough —
  Hijack the client connection + rewrite the request line/Host (other headers
  pass through unchanged; Connection/Upgrade/Sec-WebSocket-* are the handshake
  itself and are not stripped) + bidirectional splice; returns 502 before
  Hijack when the hub is unreachable; JWT verification stays in the hub, and the
  gateway adds X-Forwarded-For. If `Upstreams.RealtimeHub` is not configured,
  /ws is not registered (hub is an optional deployment). Tests cover the full
  go-zero middleware chain (measured Hijack delegation under the WithCode/Timeout
  wrappers) and the bare-writer path. **Also fixed a real bug in passing**: the
  hub's gorilla default CheckOrigin allowed only same-origin, so browser
  cross-origin handshakes (:4173 → :8893) always returned 403 and the M2 web
  browser path had been silently degrading to polling all along (a curl smoke
  test could not catch it); the admission gate is now only the JWT query (the
  token does not travel across origins automatically, so Origin checking adds no
  security). Web defaults to `ws://localhost:8800` through the tunnel
  (`VITE_REALTIME_HUB_URL` can switch to direct).
- **Read receipts** (`3c06d95`): after community mark-all-read succeeds, a
  `notification.read` frame is broadcast to all of the user's connections (replay
  is idempotently harmless), so badges on every device reset immediately; no
  publish on write failure.
- **Redis scaling** (`83d2fa5`): empty `RedisAddr` = single instance (default,
  behavior unchanged); configuring it enables the pub/sub relay (channel
  `realtime-hub:events`, envelope carries the node id for loopback exclusion;
  publish/subscribe disconnects are best-effort and only logged; a bad address
  fails fast at startup). Two non-obvious traps plugged: go-redis v9 subscribes
  lazily (Receive waits for the confirmation, otherwise early messages are
  missed); the node id carries a random suffix (multiple relays in one test
  process do not recognize each other's loopback). Presence/watch state along an
  event is still instance-local truth (not synced across instances).
- **Push bridge** (`a4b7ca9`): empty `PushWebhook` = no push; in single-instance
  mode, users with zero local deliveries get a POST to the webhook
  (`X-Push-Token`, user_ids deduplicated and merged into one request,
  fire-and-forget with a 3s timeout); vendor protocols (APNs/FCM) are handled by
  the bridging service — credentials are an external dependency, so the seam
  comes first. Under multiple instances, zero local deliveries ≠ offline, so no
  push (distributed push decisions belong to deployment). Tightened in passing:
  duplicate user_ids in publish are delivered only once (previously fanned out
  per occurrence).
- **Direct-message consumption** (`f08e19d`/`960541c`, 2026-10-12 inspection
  ignition batch): after community persists a message it calls
  `Publish("message", [recipient], {message_id,from_id,from_name,content,created_at})`,
  and a conversation read receipt calls
  `Publish("message.read", [self], {unread_count,peer_id})` — zero changes to the
  hub and gateway (the frame channel, tunnel and push bridge already exist); the
  web message page and the envelope badge refresh in realtime.
- **Voice signalling** (`acdc8ac`): upstream
  `{"action":"signal","to":..,"room":..,"data":{..}}` is forwarded statelessly
  (offer/answer/ICE pass through; the hub neither parses nor stores room state);
  all of the target's connections receive it, excluding the sender's own
  connection (no loopback for your own signal across devices); Relay mode
  broadcasts across instances as a fallback; silent drop when the target is
  offline (retries belong to the endpoint negotiation layer); the size limit is
  the 4096 upstream read limit.

### M3-1 Presence (done, approved 2026-10-11)

Single-instance in-memory state; does not depend on the gateway tunnel or Redis.

Design posture:

- Online is defined as userId with ≥1 live connection; events fire only on the
  transition edge (first connect / last disconnect), so multi-device
  online/offline does not spam duplicate frames.
- Three faces: REST service-to-service query
  `GET /internal/presence?user_ids=1,2,3` (X-Internal-Token, fail closed like
  publish, 400 for missing/invalid ids); WS watch push (upstream
  `{"action":"watch|unwatch","user_ids":[..]}`, transition edge → watchers
  receive `{"type":"presence","data":{user_id,online,at}}`); WS query snapshot
  (upstream `{"action":"query","user_ids":[..]}`, replying only to the requesting
  connection — endpoints cannot get the internal token, so pulls go through the
  existing WS JWT gate).
- Bounded memory: 200 watches per client (silently truncated above the limit) and
  200 ids per call; malformed upstream frames are silently ignored without
  disconnecting; the hub stores no social relations (who watches whom is declared
  by the client, and privacy filtering belongs to the product layer).
- No new config, no new dependencies; also fixed a potential race in SendToUsers
  where drops++ happened under an RLock (now atomic).

Landing record: `hub` gains since/watchers tables and Watch/Unwatch/Presence/
replyPresence (transition-edge events delivered outside the lock); handler gains
the presence endpoint and routing; 6 new hub tests + 2 handler tests (including
A watching 7 → 7 connections receive online, REST snapshot agreement, surviving
malformed upstream, and end-to-end 7 disconnects → offline); also fixed a
registration-timing race in an existing fanout test (Dial returning ≠ the
server-side Join has run, so the connection count is polled before publish).

## Acceptance criteria

- **M1**: `go test ./... -race` green; integration — two clients connected at the
  same time each receive one frame after publish, invalid JWT is refused, missing
  or wrong publish token returns 403; after `docker compose build realtime-hub &&
  up`, `/health` returns 200; CI all green.
- **M2**: the web bell receives a realtime +1 (manual end-to-end posture plus
  integration test); stopping the hub lets web degrade to polling without losing
  the feature; the web triple gate green.
- **M3**: decided per item at approval time.

## Standing rules carried over

Non-interactive; no tags/releases and no force push; fetch+rebase origin/main
before pushing; tests all green before committing and pushing each increment;
short English commit messages; add only files you changed; the Go module path
`github.com/tappi/tappi` never changes; push main only.
