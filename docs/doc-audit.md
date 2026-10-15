# Documentation vs source-code consistency audit (2026-10-13)

[English](doc-audit.md) | [简体中文](审计-文档一致性.zh.md)

Run against `~/workspaces/standards/文档与源码一致性审计规范.md`: enumerate the
documentation surface → collect evidence per item (file:line) → sort into three
buckets → record in a table → fix (the source code is the truth; never change
code to flatter the docs) → re-check the gates.

- Surface audited: root README.md / README.zh.md, apps/**, services/** READMEs,
  docs/** site pages, and the gh repository facade (description / topics).
  Negative claims ("not created / not implemented") all had their search paths
  verified first.
- Bucket counts: **A docs ahead of code 19 · B code ahead of docs 32 ·
  C description mismatch 31** (82 items, all fixed).
- P0: 1 item (the facade's mini-program). P1: 18 items. The rest P2.
- Fix principle: the code is always authoritative; docs either drop the
  ahead-of-code claim or mark it "planned", fill in missing surfaces, and
  correct mismatched wording. Zero code changes this round.

## P0 / P1 list

| ID | Severity | Summary |
| --- | --- | --- |
| D-01 | P0 | The gh facade description/topics claim a mini-program, which was decided not to start |
| D-02 | P1 | Root README (EN/ZH) calls realtime-hub "not yet created" while the service is fully landed |
| D-06 | P1 | Root README has no current-state section for community/realtime-hub/user-service-rpc |
| D-12 | P1 | Root README says "seven services with a dual-driver data layer" and "every service has its own Dockerfile"; only 5 services are dual-driver and rpc has no image |
| D-13 | P1 | apps/README claims "design system / routing conventions / i18n are all supported through packages/", but packages/ holds only config |
| D-14 | P1 | services/README attributes "friends/achievements/points/notification preferences" to user-service |
| D-18 | P1 | services/README calls realtime-hub an "M1 skeleton" while voice signalling / push / tunnel are all landed |
| D-20 | P1 | Gateway README says "if BaseURL is not configured the routes are not registered (optional deployment)", but startup panics for everything except RealtimeHub |
| D-32 | P1 | content-service README metric prefix `content_service_gamecatalog_client_*` is entirely wrong |
| D-40 | P1 | docs/index.md says "api-gateway manages login issuing"; login is a proxy and issuing lives in user-service |
| D-44 | P1 | docs/clients.md calls realtime-hub "not yet created", contradicting its own 2026-10-11 record |
| D-48 | P1 | The development guide's service tree says "all seven landed" and has no realtime-hub |
| D-49 | P1 | The development guide's Dockerfile excerpt shows golang:1.22, which is actually 1.26 |
| D-51 | P1 | The development guide's cross-service metric names disagree with the implementation |
| D-57 | P1 | The development guide's demo-account login example — the service has no seed users, so it always 401s |
| D-64 | P1 | mobile_plan calls realtime-hub "not landed" |
| D-65 | P1 | mobile_plan calls image upload "a separate backend task" while gateway /upload is landed and wired on both clients |
| D-67 | P1 | The backlog deferred table still lists realtime-hub as "decided not to start", contradicting line 10 of the same document |
| D-83 | P1 | apps/mobile-app has no README (its only doc is the generic AGENTS.md) |

## Audit table

Legend: A = docs ahead of code / B = code ahead of docs / C = description
mismatch. Every item was fixed this round.

| ID | Page | Claim | Source evidence (file:line) | Category | Severity | Fix |
| --- | --- | --- | --- | --- | --- | --- |
| D-01 | gh facade | description/topics claim a mini-program client | apps/mini-program does not exist; docs/backlog.md deferred table says "decided not to start" | A | P0 | remove mini-program from the description and delete the topic |
| D-02 | README.md:53-55 / README.zh.md:53-54 | Seven services have code; realtime-hub "not yet created" | services/realtime-hub/realtime-hub.go:1, etc/realtime-hub.yaml:2, Dockerfile | B | P1 | say eight services and move realtime-hub out of the unbuilt list |
| D-03 | README.md:14 / README.zh.md:14 | nightly artifacts ×6 (six backend services) | .github/workflows/nightly.yml:47-53 has seven build_service | C | P2 | ×7 (seven services) |
| D-04 | README.md:77 / README.zh.md:74 | mobile-app aggregates teaming/stats/events | apps/mobile-app/src/app/(tabs)/_layout.tsx has five tabs with no teaming/events | A | P2 | say stats/guides/community/notifications |
| D-05 | README.md:87 / README.zh.md:84 | "M0–M3 landed" stated absolutely; "CI lint+typecheck + expo export walkthrough" implies CI runs export | docs/mobile_plan.md (device regression unfinished); .github/workflows/ci.yml:87-91 runs lint+typecheck only | C | P2 | say "code slices landed + device regression pending" and "CI runs lint+typecheck only; export walkthroughs are recorded" |
| D-06 | README.md service-status section / README.zh.md | No sections for community/realtime-hub/rpc | services/community/internal/handler/routes.go:28-258; services/realtime-hub/internal/handler/routes.go:63-66 | B | P1 | add sections 6/7/8 and renumber web to 9 |
| D-07 | README.md port table / README.zh.md | No realtime-hub row | services/realtime-hub/etc/realtime-hub.yaml:2 (:8893) | B | P2 | add the row |
| D-08 | README.md quick start / README.zh.md | The startup checklist omits realtime-hub | services/realtime-hub/realtime-hub.go:21 | B | P2 | add a go run line |
| D-10 | README.md:153 / README.zh.md:150 | web community has only a "following mode" | apps/web-client/src/pages/CommunityPage.tsx:31 (latest/hot/mine/followed/search) | B | P2 | add the four modes plus keyword search |
| D-11 | README.md:141 / README.zh.md:138 | The proxy list misses DMs/moderation (the Chinese version also misses notifications) | services/api-gateway/internal/community/routes.go:58-67 | B | P2 | add messages/moderation/notifications to the list |
| D-12 | README.md technical highlights / README.zh.md | "Seven services with a dual-driver data layer", "every service directory has its own Dockerfile" | Only 5 services are dual-driver; services/user-service-rpc/ has no Dockerfile | C | P1 | limit it to the five SQLite services; note that rpc has no image and the hub is stateless |
| D-13 | apps/README.md:9 | Design system / routing conventions / i18n are all supported through packages/ | packages/ holds only config (env/lint/make) | A | P1 | describe the config reality and mark the rest as planned |
| D-14 | services/README.md:6 | user-service includes friends/achievements/points/notification preferences | services/user-service/model/usermodel.go:246 (users table only) | A | P1 | trim to accounts/authz/profile/recommendations and mark the rest as planned |
| D-15 | services/README.md:7 | game-catalog includes version metadata | services/game-catalog/model/gamemodel.go:81 has no version field | A | P2 | delete it |
| D-16 | services/README.md:8 | content-service includes moderation | services/content-service/internal/handler/routes.go:15-89 has no moderation endpoints | A | P2 | say "publish-state management" |
| D-17 | services/README.md:10 | data-panel includes a leaderboard API | services/data-panel/internal/handler/routes.go has only 4 routes | A | P2 | delete it |
| D-18 | services/README.md:13 | realtime-hub is an "M1 skeleton", with voice signalling / push fanout planned for M3 | services/realtime-hub/internal/hub/signal.go:16-34, push.go:39-53, README.md:41-46 all M3 items checked | B | P1 | change the status to "landed" and list the full responsibilities |
| D-19 | services/README.md:9 | community's responsibilities miss DMs/moderation | services/community/internal/handler/routes.go:163-207 | B | P2 | add direct messages and report moderation |
| D-20 | services/api-gateway/README.md:17,28 | An unconfigured upstream means that group is not registered (optional deployment) | internal/proxy/upstream.go:20-22 panics; only internal/realtime/routes.go:17-19 skips | C | P1 | say "startup fails for everything except RealtimeHub" |
| D-21 | services/api-gateway/README.md:14 | The community proxy list misses messages/moderation/comments | internal/community/routes.go:58-67 | B | P2 | complete the list |
| D-22 | services/api-gateway/README.md:7-15 | The route table has no user-proxy row | internal/users/routes.go:25-27 | B | P2 | add the row |
| D-23 | services/api-gateway/README.md:45 | Test file path `internal/*/routes_integration_test.go` | Actually internal/handler/routes_integration_test.go plus each group's internal/*/routes_test.go | C | P2 | correct the paths |
| D-24 | services/community/README.md:69 | DMs: "each side stores a row view" | internal/model/directmessagemodel.go:66-79 stores a single row | C | P2 | say "one shared row; is_read flips only for the recipient" |
| D-25 | services/community/README.md:154 | "No outbound calls" | internal/realtime/publisher.go:27-36 POSTs to the hub's /internal/publish | C | P2 | limit to "no outbound calls to user-service" and add the hub publish |
| D-26 | services/community/README.md:237 | Dockerfile golang:1.22 | services/community/Dockerfile:3 golang:1.26 | C | P2 | change to 1.26 |
| D-27 | services/community/README.md:53-55 | Notification endpoints miss :id/read | internal/handler/routes.go:158 | B | P2 | add the endpoint |
| D-28 | services/community/README.md:27-36 | Post endpoints miss the three comment routes | internal/handler/routes.go:87-104 | B | P2 | add the endpoints |
| D-29 | services/community/README.md:172-191 | The config block misses Realtime/Prometheus | etc/community-api.yaml has a Realtime section plus Prometheus 9094 | B | P2 | add the config sections |
| D-30 | services/community/README.md:305 | "Current version v1.2" | No version marker in the code | A | P2 | delete the version line |
| D-31 | services/community/README.md:164-167 | The gateway entry list misses messages | services/api-gateway/internal/community/routes.go:58-62 | B | P2 | add `/api/v1/messages**` |
| D-32 | services/content-service/README.md:29 | Metric prefix content_service_gamecatalog_client_* | internal/metrics/metrics.go:14,24,35,45 real names content_game_catalog_client_* | C | P1 | correct the metric names |
| D-33 | services/content-service/README.md:29 | "retries" is a separate metric | There is no retries counter, only a retry_exhausted status label | A | P2 | change the wording |
| D-34 | services/content-service/README.md:71-72 | guiderepository.go/commentrepository.go | model/guidemodel.go, commentmodel.go | C | P2 | correct the file names |
| D-35 | services/content-service/README.md:33-73 | The directory tree misses middleware/utils/client/cache/metrics/httperr/favourite files | All the directories exist on disk | B | P2 | complete the tree |
| D-36 | services/content-service/README.md:7-14 | The feature list misses favourites | routes.go:53-65 has four favourite endpoints | B | P2 | add "favourite a guide (idempotent)" |
| D-37 | services/content-service/README.md:217-224 | The config block misses Auth/Services.GameCatalog/Prometheus | All three sections are in etc/content-api.yaml | B | P2 | add the config sections |
| D-38 | services/realtime-hub/README.md:3 | "Direct-connect mode… tunnelisation is an M3 item" | The gateway's internal/realtime/routes.go:26 tunnel is landed (M3-2 [x]); the Redis relay is landed | B | P2 | update the intro to landed |
| D-39 | docs/index.md:7 | The homepage hero omits the realtime channel and says only "seven go-zero microservices" | go.work has eight modules (7 go-zero + realtime-hub) | B | P2 | add the realtime hub to the hero plus a new "realtime channel" feature |
| D-40 | docs/index.md:27 | "api-gateway manages login issuing" | internal/users/routes.go:26 login is a proxy; issuing is in user-service | C | P1 | say "login/registration proxy" |
| D-41 | docs/index.md:31 | "Six services each open /metrics" | etc/*.yaml has seven Prometheus ports 9091–9097 | C | P2 | seven (9091–9097) |
| D-42 | docs/index.md:25 | The community feature misses DMs/moderation | services/community/internal/handler/routes.go:163-207 | B | P2 | complete the feature |
| D-43 | docs/README.md:5 | "Six-service development guide" | The guide's service tree now has eight modules | C | P2 | say eight services |
| D-44 | docs/clients.md:52 | realtime-hub "not yet created" | services/realtime-hub/ exists; earlier in the same document it landed 2026-10-11 | A | P1 | move it out of the unbuilt list |
| D-45 | docs/clients.md:38-48 | The service table has no realtime-hub row | etc/realtime-hub.yaml:2 | B | P2 | add the row (8893, no /metrics) |
| D-46 | docs/clients.md:46 | community's responsibilities miss DMs/moderation | routes.go:163-207 | B | P2 | complete it |
| D-47 | docs/development-guide.md:13 | Go recommended 1.25+ | go.work:1 go 1.26.0 | C | P2 | change to 1.26+ |
| D-48 | docs/development-guide.md:48-55 | The service tree says "all seven landed" with no realtime-hub | go.work use lists eight modules | B | P1 | add a realtime-hub row |
| D-49 | docs/development-guide.md:1070 | Excerpt says golang:1.22 | services/community/Dockerfile:3 golang:1.26 | C | P1 | change to 1.26 |
| D-50 | docs/development-guide.md:1213-1220 | The /metrics table misses data-panel | services/data-panel/etc/data-panel-api.yaml 9097 | B | P2 | add the row |
| D-51 | docs/development-guide.md:1242-1246 | Cross-service metric names/labels disagree with the implementation | internal/metrics/metrics.go:14,24,35,45 | C | P1 | rewrite using the real names (calls_total/duration/breaker_state/transitions) |
| D-52 | docs/development-guide.md:1275-1277 | "Six targets 9091–9096", "six-service Log block" | deploy/prometheus/prometheus.yml has seven targets; realtime-hub has no Log block | C | P2 | seven targets 9091–9097; note the hub exception in the Log block |
| D-53 | docs/development-guide.md:1299-1300 | "Five services added /metrics exposure (9091–9095)" | docker-compose.yaml publishes 9091–9095 + 9097 | C | P2 | six services (9091–9095, 9097) |
| D-54 | docs/development-guide.md:1312 | "Six targets up" | Same as D-52, seven targets | C | P2 | change to seven |
| D-55 | docs/development-guide.md:1399 | `curl /health` liveness for user-service | services/user-service/internal/handler/routes.go does not register /health | C | P2 | use the registered /from/:name route |
| D-56 | docs/development-guide.md:1415 | setup-go@v5 | .github/workflows/ci.yml:14 @v7 | C | P2 | change to @v7 |
| D-57 | docs/development-guide.md:1352 | A demo account logs in directly | model/usermodel.go has no seed users | A | P1 | add a register step before login |
| D-58 | docs/development-guide.md:1331-1338 | The auth overview says "6 services" with no realtime-hub | services/realtime-hub/internal/auth/auth.go HS256 | B | P2 | say 7 services and add a hub row |
| D-59 | docs/development-guide.md:1449 | "content-service guides have no image field" | model/guidemodel.go CoverImage | A | P2 | say "guides have CoverImage; users have no avatar" |
| D-60 | docs/development-guide.md:1584 | The gateway's own-handler table misses /upload, /uploads/:file | services/api-gateway/internal/handler/routes.go:37-43 | B | P2 | add both |
| D-61 | docs/development-guide.md:1588 | "community has no outbound calls" | services/community/internal/svc/service_context.go Realtime Publisher | C | P2 | say "no outbound calls to user-service; publishes to the hub" |
| D-62 | docs/development-guide.md:1572 | "All 6 services have a JWT system" | The auth overview now covers 7 services | C | P2 | change to 7 |
| D-63 | docs/development-guide.md:1065 | "Every service has a Dockerfile with the same pattern" | user-service-rpc has none; api-gateway/realtime-hub are distroless | C | P2 | add the qualification |
| D-64 | docs/mobile_plan.md:49 | "realtime-hub not landed" | services/realtime-hub/ is fully landed (including voice signalling hub/signal.go) | A | P1 | say landed |
| D-65 | docs/mobile_plan.md:90 | "Upload capability is a separate backend task" | gateway upload.go is landed (6642132) and wired on both clients | A | P1 | say cleared |
| D-66 | docs/realtime-hub.md:32 | "JWT verified on handshake; disconnect on failure" | internal/handler/routes.go:76-81 returns 401 before the Upgrade | C | P2 | say "refuse with 401 on failure (before upgrade)" |
| D-67 | docs/backlog.md:52 | The deferred table includes realtime-hub as "decided not to start" | Line 10 of the same document says it landed 2026-10-11 | C | P1 | remove realtime-hub from the deferred table |
| D-68 | docs/backlog.md:19-28 | The CI snapshot misses a realtime-hub row; package counts are stale | realtime-hub has 5 test packages; api-gateway 12 / community 19 packages (measured 2026-10-13) | B | P2 | add the row plus a snapshot-posture note |
| D-69 | docs/backlog.md:50-51 | community README:271/272 line numbers drifted | Actually :285/:286 | C | P2 | correct the line numbers |
| D-70 | docs/architecture/overview.md:28 | "Persist by data model (e.g. Postgres/ElasticSearch)" + a Kafka/PubSub message bus | No Postgres/ES/Kafka anywhere in the repo; storage is SQLite/MySQL | A | P2 | describe the dual-driver reality and mark the candidate technologies "candidate" |
| D-71 | docs/architecture/overview.md:30 | "Report through the shared-utils SDK to a data pipeline" | packages/ holds only config; there is no data pipeline | A | P2 | say stats write straight to data-panel and mark the SDK as planned |
| D-72 | docs/architecture/overview.md:49 | "Every service provides a docs/ subdirectory" | No services/*/docs exists | A | P2 | say "each service's README.md" |
| D-73 | docs/architecture/overview.md:50 | "Contract tests under tests/" | tests/ holds only a README that describes itself as planned | A | P2 | say "per-service test coverage; cross-service tests/ is planned" |
| D-74 | docs/architecture/overview.md:51 | "A CI/CD pipeline provided by the platform layer" | CI lives in .github/workflows; platform/ is planned | C | P2 | use the workflows wording |
| D-75 | docs/architecture/topology.md:10 | user-service's responsibilities include a "growth system" | No growth/level code exists (grep has no hits) | A | P2 | delete it |
| D-76 | docs/architecture/topology.md:13 | community's responsibilities miss DMs/moderation/search | routes.go:130,163-207 | B | P2 | complete the responsibilities |
| D-77 | docs/architecture/topology.md:59 | The community proxy list misses search/comments/messages/moderation | services/api-gateway/internal/community/routes.go:32,42-44,58-67 | B | P2 | complete the list |
| D-78 | docs/architecture/topology.md:79 | "Every service directory has its own Dockerfile" | user-service-rpc has no Dockerfile | C | P2 | add the qualification (rpc has no image; two services are distroless) |
| D-79 | apps/web-client/README.md:25-26 | The route list misses 8 implemented routes | apps/web-client/src/App.tsx:156-264 | B | P2 | complete all 15 |
| D-80 | apps/web-client/GUIDE.md:9-19 | The route table misses 6 implemented routes | Same as above (/discover /games /games/:id /stats /profile /users/:id) | B | P2 | add the 6 rows (including the permission column) |
| D-81 | apps/web-client/GUIDE.md:35 | "The create-guide button on the homepage or the guide list page" | Only GuidesPage.tsx:155 has that button | C | P2 | say "the guide list page" |
| D-82 | apps/web-client/GUIDE.md:118 | The gateway variable says "login/recommendations/featured prefer the gateway" | Once configured, all calls go through the gateway (every fetcher in client.ts) | C | P2 | say "all service calls prefer the gateway" |
| D-83 | apps/mobile-app | No README.md (root README:26 and apps/README.md:4 both reference this client) | A find over the whole directory shows only AGENTS.md | B | P1 | create a README (routes/config/scripts/status) |

## Verification of negative claims (search paths verified before concluding)

The following "not landed" claims were confirmed true by a repo-wide grep
(services/ apps/ packages/ platform/ deploy/ tools/ docker-compose.yaml
go.work, excluding .md/.out), so the original verdict stands:

- matchmaking, crawler-jobs, mini-program, admin-console: directories do not exist.
- Elasticsearch / ML recommendations / video content / GraphQL: no implementation.
- Postgres / Kafka: no implementation (the overview.md text now says
  "candidate").

## Fix status and gates

- All 82 items were fixed this round (the facade's 1 item via `gh repo edit`;
  the rest in repository commits).
- Zero code changes this round; Go `-race` tests across all modules measured
  green 2026-10-13 (community 353 / gateway 124 / realtime-hub 45 /
  user-service 116 / game-catalog 100 / content-service 301 / data-panel 96 /
  user-service-rpc 5).
- Docs site build gate: docs/ VitePress `npm run build` passes (this page ships
  with the site).
