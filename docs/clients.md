# Clients & Services


Current state of spawn's clients and services. Planned surfaces only list
their status; unimplemented features are not described.

## Clients

| Client | Directory | Stack | Status |
| --- | --- | --- | --- |
| Web | `apps/web-client` | React 19 · Vite 8 · React Router 7 | Landed |
| Mobile | `apps/mobile-app` | Expo 57 · React Native 0.86 · expo-router | Landed (Android release packaging flow ready) |
| Mini-program shell | `apps/mini-program` | — | Planned, not created |
| Admin console | `apps/admin-console` | — | Planned, not created |

## Web client (web-client)

- Main entry for guides, leaderboards and community; routes are organized with
  React Router.
- During development it calls the business services directly; the gateway
  aggregate endpoint (`/home/feed`) returns all four homepage groups in one
  call.
- The Discover page (`/discover`) consumes the `GET /home/feed` aggregate's
  four blocks (featured games / hot posts / topics / guides); the gateway marks
  a degraded group with a banner at its top. Parity with the mobile Discover
  tab (landed 2026-10-09).
- M3 relationship features on web (landed 2026-10-09): the community feed gains
  a Following mode (`GET /api/v1/posts/followed`, falling back to latest when
  not signed in); the profile page (`/profile`, reachable from the nav after
  login) aggregates my posts (author_id filter) / my guides (author_id filter,
  drafts visible with a Bearer token for one's own) / my likes
  (`GET /api/v1/users/likes`).
- In-app notifications (landed 2026-10-09): nav bell with unread badge (30s
  polling) plus the `/notifications` center (like / comment / reply / follow —
  four types, unread dots, mark all read; like/comment/reply open the original
  post).
- Common scripts: `pnpm --dir apps/web-client run dev` (dev), `build`
  (tsc + vite bundle), `lint`.

## Mobile client (mobile-app)

- File-based routing on Expo and expo-router, covering game catalog / guides /
  community / stats scenarios.
- Integrations: expo-notifications (push), expo-linking (deep links and share
  cards), expo-secure-store (secure credential storage).
- Homepage data comes from the gateway `GET /home/feed` aggregate consumption
  point (Discover tab).
- In-app notifications (landed 2026-10-10): community-tab bell unread badge
  plus the `/notifications` center (like/comment/reply open the original post,
  mark all read).
- Guide authoring and editing (landed 2026-10-10): `/guide-compose` writes a
  guide (plain text / Markdown, publishing on create); the detail page allows
  the author to edit (PUT partial update, game cannot be changed).
- Android packaging: `pnpm --dir apps/mobile-app run prebuild:android` generates
  the native project, `build:android:release` runs gradle assembleRelease.
- On-device regression items (local push, deep-link share cards, notification
  center) still await device verification; progress in the
  [mobile plan](/mobile_plan).

## Services (seven go-zero microservices + one realtime hub)

| Service | Directory | Responsibility | Storage |
| --- | --- | --- | --- |
| api-gateway | `services/api-gateway` | Access layer: JWT auth, route proxying, BFF aggregation | — |
| user-service | `services/user-service` | Accounts, login/registration, user profile and recommendations | SQLite / MySQL dual driver |
| game-catalog | `services/game-catalog` | Game catalog, tags, featured, recommendations | SQLite / MySQL dual driver + in-process read cache |
| content-service | `services/content-service` | Guides, comments, publish flow, favourites | SQLite / MySQL dual driver + in-process read cache |
| community | `services/community` | Posts, topics, follows, likes, follow feed, in-app notifications, direct messages, report moderation | SQLite / MySQL dual driver |
| data-panel | `services/data-panel` | Stats ingest (monotonic increments) and player stats queries | SQLite / MySQL dual driver + TTL/LRU cache |
| user-service-rpc | `services/user-service-rpc` | Internal gRPC API for the user domain (zrpc + etcd) | Reuses user-service storage |
| realtime-hub | `services/realtime-hub` | WebSocket push / presence / voice signalling (8893, no /metrics) | — (in-memory registry, optional Redis pub/sub relay) |

The port table is in [Getting Started](/guide/getting-started); the current
record of service boundaries and auth data flow is in
[Current Topology](/architecture/topology).

Planned services: matchmaking (team matching) and crawler-jobs (external data
scraping) are not created yet (realtime-hub landed 2026-10-11, see the table
above) — see the directory plan in the repository README.
