<div align="center">
  <img src="docs/public/logo.svg" alt="spawn logo" width="64" />
  <h1>spawn</h1>
  <p>Game community platform: guides, leaderboards, and community</p>
  <p><a href="README.md">English</a> | <a href="README.zh.md">简体中文</a></p>
</div>

## Download & Install

The [nightly rolling Release](https://github.com/cuihairu/spawn/releases/tag/nightly) (daily auto-build, fixed `nightly` tag, old assets replaced with new ones) provides ready-made artifacts:

| Artifact | Contents |
| --- | --- |
| `spawn-<service>-linux-amd64` ×6 | Binaries for the six backend services (CGO build with embedded SQLite driver); run directly after `chmod +x`. Config lives in each service's `etc/` directory in the repo; inject a MySQL connection string via `DATASOURCE` to switch storage |
| `spawn-web-client-linux-amd64.tar.gz` | Static build of the web site; serve it from any static server after extracting |
| `SHA256SUMS.txt` | Checksums for all assets; verify with `sha256sum -c SHA256SUMS.txt` in the same directory |

Mobile APKs and service container images are not part of the nightly delivery (Android packaging depends on a local gradle flow; build images yourself from `services/*/Dockerfile`). No stable release yet; documentation site at <https://cuihairu.github.io/spawn/>.

## Monorepo Layout

```
.
├── apps                 # End-user applications across platforms
│   ├── web-client       # Web site (guides / leaderboards / community)
│   ├── mobile-app       # iOS/Android client (incl. stats dashboard)
│   ├── mini-program     # WeChat/Alipay mini-program shells
│   └── admin-console    # Operations admin frontend
├── services             # Backend and realtime services
│   ├── api-gateway      # BFF / GraphQL / access layer
│   ├── user-service     # Accounts, permissions, friends, progression
│   ├── game-catalog     # Game library, leaderboards, tags, recommendations
│   ├── content-service  # Guides, news, CMS, comments
│   ├── community        # Posts, topics & circles, interactions, notifications
│   ├── matchmaking      # Team-up/matchmaking lobby, room management, voice signaling
│   ├── realtime-hub     # WebSocket/RTC/push gateway
│   ├── data-panel       # Match-data scraping, analytics, player profile API
│   └── crawler-jobs     # Store/esports/announcement crawlers and scheduling
├── packages             # Shared components and utilities across platforms
│   ├── ui-kit           # Design system, cross-platform component library
│   ├── data-models      # Protobuf/GraphQL schemas, type definitions
│   ├── shared-utils     # Utilities, hooks, SDK
│   └── config           # Build config, lint, env templates
├── platform             # Platform-level support
│   ├── infra            # Terraform/IaC, K8s, Helm charts
│   ├── devops           # CI/CD, release scripts, canary strategy
│   └── observability    # Logging, monitoring, alerting, SLO
├── tools                # CLIs, scripts, code generation, data migration
├── docs                 # Product, tech, API, ops docs
└── tests                # Cross-service integration and compliance tests
```

> The tree above is a roadmap. For actual status see `services/README.md` and `apps/README.md`:
> the seven services plus web-client and mobile-app have code; matchmaking/realtime-hub/crawler-jobs,
> mini-program, and admin-console have not been created yet.

## Observability

Each go-zero service exposes a `/metrics` endpoint via the `Prometheus` section of its config
(go-zero agent, listening on a separate port), with built-in request count/latency/status-code
metrics; content-service additionally has custom metrics for cross-service calls.
Ports and metric details are documented in the "Monitoring & Metrics (Prometheus)" section of
`docs/development-guide.md`:

| Service | Business port | /metrics port |
| --- | --- | --- |
| user-service | 8888 | 9091 |
| game-catalog | 8890 | 9092 |
| content-service | 8891 | 9093 |
| community | 8892 | 9094 |
| api-gateway | 8800 | 9095 |
| data-panel | 8896 | 9097 |
| user-service-rpc | 8080 (gRPC) | 9096 |

### Module Responsibilities at a Glance

- `apps/`: experience layer. web-client runs the guides/community main site; mobile-app aggregates team-up, match data, and events; mini-program focuses on lightweight browsing and mini-game multiplayer; admin-console provides content and operations configuration.
- `services/`: domain microservices. user-service manages accounts and social graph; game-catalog maintains the game library, leaderboards, and recommendations; content-service + community handle guides, posts, and interactions; matchmaking + realtime-hub cover realtime rooms, voice, and push; data-panel aggregates match and patch data; crawler-jobs periodically fetches external information.
- `packages/`: unified design system and type contracts. ui-kit ships components and themes; data-models holds protobuf/graphql schemas and TypeScript/Go SDKs; shared-utils provides cross-platform utilities; config centralizes build, lint, and env templates.
- `platform/`: infrastructure-related IaC, CI/CD, and observability config supporting automated deploys, canary releases, and alerting.
- `tools/`: CLIs, scripts, code generators, data migration jobs, and other developer-efficiency helpers.
- `docs/`: product plans, technical designs, API docs, ops manuals; paired with a docs site or knowledge base.
- `tests/`: cross-service integration tests, end-to-end scripts, compliance/security scan configs.

### Next Steps

1. Mobile M0–M3 and the M4 first milestone are landed (2026-10-09, see `docs/mobile_plan.md`): Android release packaging works (`android.package` config + `build:android:release` script, aapt/apksigner static checks passed); the data-panel stats tab consumes the gateway `/api/v1/stats/*`. Remaining: on-device install and functional regression — local push, deep-link share cards, and the notification center are still unverified on real devices (this environment has no emulator/device; CI lint+typecheck + `expo export` bundle walkthroughs are the standing substitute).
2. api-gateway BFF phase two is done (2026-10-04): the `GET /home/feed` aggregation endpoint (featured games / hot posts / topics / guides in one request, per-group field trimming + per-upstream failure degradation), consumed by both the web discover page and the mobile discover tab; web-client keeps direct service calls unaffected. More aggregation shapes to come as needed.
3. `docs/architecture/` overall architecture and data-flow docs are complete (2026-10-04): `topology.md` records the current state (service boundaries/ports/gateway reverse-proxy routes/auth data flow/evolution roadmap), `overview.md` stays at the vision level; future architecture changes update the current-state doc along with each slice.

### Go-zero Development Conventions

- Tooling: `go install github.com/zeromicro/go-zero/tools/goctl@latest`, and add `$(go env GOPATH)/bin` to `PATH`; also install `protoc-gen-go` and `protoc-gen-go-grpc`.
- Workspace: the root `go.work` includes each service module (`services/user-service`, `services/user-service-rpc`...) so cross-references don't hit remote dependencies.
- Code generation: use `goctl api new <service>`/`goctl rpc new <service>` to scaffold; API/RPC definitions live in `*.api`/`*.proto` files, with functional code generated via `goctl api go`/`goctl rpc protoc`.
- Shared config: `packages/config` provides `.env.example`, golangci-lint templates, common `make` targets, etc., which services can reference directly or via `Makefile` include.

## Services & Frontend Status

#### 1. Game Catalog Service (`services/game-catalog`)
Built with go-zero; SQLite/MySQL dual-driver storage (default local SQLite with embedded seed data), list filtering, create endpoints (JWT-protected), and recommendation/featured capabilities.
- `GET /games` - Game list (filtering and pagination)
- `GET /games/:id` - Game details
- `GET /games/featured` - Featured games
- `GET /games/recommendations` - Recommended games
- Port: 8890

#### 2. User Service (`services/user-service`)
Account, permission, friends, and progression management; the JWT issuer.
- `POST /auth/login` - User login
- `POST /auth/register` - User registration
- `GET /users/:id` - Get user info
- `GET /users/:id/recommendations` - User game recommendations (cross-service call to game-catalog)
- Global JWT middleware (whitelist for public paths like register/login)
- Port: 8888

#### 3. Content Service (`services/content-service`)
Guides, news, and comments service; go-zero implementation with SQLite/MySQL dual-driver storage.
- **Guide management**:
  - `POST /api/v1/guides` - Create a guide
  - `PUT /api/v1/guides/:id` - Update a guide
  - `GET /api/v1/guides/:id` - Guide details
  - `GET /api/v1/guides` - Guide list (filterable)
  - `POST /api/v1/guides/:id/publish` - Publish a guide
  - `POST /api/v1/guides/:id/like` - Like a guide
- **Comment system**:
  - `POST /api/v1/comments` - Post a comment
  - `GET /api/v1/comments` - Comment list
  - `DELETE /api/v1/comments/:id` - Delete a comment
  - `POST /api/v1/comments/:id/like` - Like a comment
- Supports nested comments and replies
- Port: 8891

#### 4. API Gateway (`services/api-gateway`)
Unified access layer combining BFF aggregation and a five-way reverse proxy; only one unified API surface is exposed (all mobile-app calls go through this single address).
- `POST /auth/login` - User login (proxied to user-service)
- `GET /games/featured` - Featured games
- `GET /users/:id/recommendations` - User recommendations
- `GET /home/feed` - Home feed aggregation (featured games / hot posts / topics / guides, four concurrent calls with field trimming + per-upstream failure degradation)
- `GET /s/p/:id` - Post share-card jump page (og meta + `spawn://` deep link + web entry)
- Reverse proxy: community (posts/topics/follows/likes/post comments/notifications), content (guides/comments), users (register/profile), games (list/detail), data-panel (match stats) — full route coverage
- Port: 8800

#### 5. Data Panel Service (`services/data-panel`)
Match-stats aggregation for the player dashboard; go-zero with SQLite/MySQL dual-driver storage (default local SQLite with embedded seed data). Ingest is monotonic increment accumulation (single-statement upsert `col = col + delta`, never regresses).
- `GET /api/v1/stats/users/:user_id/summary` - Cross-game summary (public)
- `GET /api/v1/stats/users/:user_id/games` - Per-game stats list (public, paginated)
- `GET /api/v1/stats/users/:user_id/games/:game_id` - Single game stats (public)
- `POST /api/v1/stats/records` - Ingest stat deltas (JWT-protected, attributed to the token user)
- Port: 8896

#### 6. Web Client (`apps/web-client`)
React 19 + Vite + React Router 7 frontend. Pages: home (login & recommendations), discover (one-request home feed), games library + game detail, guide list/detail/editor, community (post feed with followed mode), post detail, notifications (nav bell with unread badge + `/notifications` center: like/comment/reply/follow, mark-all-read), profile, match-stats dashboard (KPI cards + per-game table via the gateway). Guides support "All/Mine" filtering with draft and published save states; guide comments support nested replies and likes; posts support like/share/comments (reply with parent comment, author-only delete); the games library offers keyword search, genre filtering, and pagination, with game cards linking to detail pages that surface related guides. Browsing works without login; write operations require login. Dark theme, responsive layout.

### Technical Highlights

Cross-service calls go over HTTP: user-service calls game-catalog via `GameCatalogClient` to
fetch game data for recommendations, with failure fallback; content-service fetches game titles
from game-catalog with circuit breaking, retries, and Prometheus metrics. Game repository and
user recommendation logic have unit tests. `docker-compose.yaml` brings up all services with one
command, and each service directory has its own `Dockerfile`. All seven services' data layers run
on the SQLite/MySQL dual driver (a DSN containing `file:` uses SQLite; inject a MySQL connection
string in production), with in-process TTL+LRU read caching; concurrency safety is guaranteed by
connection-pool clamping and the cache itself.

### Quick Start

```bash
# Start all services (Docker required)
docker compose up --build

# Run a single service
go run services/game-catalog/game.go -f services/game-catalog/etc/game-api.yaml
go run services/user-service/user.go -f services/user-service/etc/user-api.yaml
go run services/content-service/content.go -f services/content-service/etc/content-api.yaml
go run services/community/community.go -f services/community/etc/community-api.yaml
go run services/data-panel/data-panel.go -f services/data-panel/etc/data-panel-api.yaml
go run services/api-gateway/gateway.go -f services/api-gateway/etc/gateway-api.yaml

# Frontend
cd apps/web-client && npm install && npm run dev
```

### Service Port Map

| Service | Port | Description | Status |
|------|------|------|------|
| user-service | 8888 | User service | Done |
| api-gateway | 8800 | API gateway (BFF aggregation + reverse proxy) | Done |
| game-catalog | 8890 | Game catalog service | Done |
| content-service | 8891 | Content service (guides + comments) | Done |
| community | 8892 | Community service (posts/topics/follows/likes) | Done |
| data-panel | 8896 | Match-stats panel service (ingest + query) | Done |
| user-service-rpc | 8080 (gRPC) | User service RPC (cluster-internal) | Done |
| web-client | 5173 | Web frontend | Done |
| mobile-app | — | React Native (Expo) client | In progress — M0–M3 + M4 milestone 1 landed (see `docs/mobile_plan.md`) |
