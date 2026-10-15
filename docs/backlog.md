# Backlog Ledger (est. 2026-10-10)

[English](backlog.md) | [简体中文](backlog.zh.md)

> Single source of truth: status and blockers of every unfinished item.
> Completed items are not listed here (see git log and each README).
> Maintenance rule: backfill this table after each landed increment; a new task
> enters the table before work starts.

## To-do (actionable, by priority)

| # | Item | Blocker / dependency | Notes |
|---|----|----------|------|
| 1 | realtime-hub realtime channel groundwork (approved to start 2026-10-11) | No external dependency | M1 skeleton + M2 integration + M3-1 presence (2026-10-11); remaining M3 items all landed on 2026-10-12 under a user order to push straight through: tunnel (gateway `/ws` passthrough) / read receipts / Redis cross-instance delivery / push-bridge webhook seam / voice signalling (same-day todo.md entries carry the commits) |
| 2 | ~~Preventive CI runner pinning~~ handled (2026-10-11: five `ubuntu-latest` pinned to `ubuntu-24.04` — ci×2 / docs×2 / nightly); ~~Ubuntu 26 migration assessment~~ done (2026-10-12; conclusion recorded in the todo.md watch item: do not follow tag rollouts; from 10-19 a nightly-only `ubuntu-26.04` canary, decide globally after wrap-up on 11-19; only image-side dependency = nightly CGO gcc13→14) | Config-action gate 2026-10-19 (user order: no config actions before 10-19) | Zero config changes during the assessment window |

### CI snapshot review (2026-10-11)

> Method: the last 30 runs via `gh run list`, plus a per-package check of the
> latest run's job logs (38112498701 @ `03c8924`).

**Verdict: zero reds, zero flakes — eight Go modules plus web/mobile all green.
Nothing to fix, nothing to rerun.**
(Package counts are the 2026-10-11 snapshot; the 2026-10-13 recheck measured
api-gateway 12 packages, community 19, realtime-hub 5. realtime-hub was already
picked up dynamically via go.work at snapshot time but the original table missed
it — row added below.)

| Module | CI verdict | Notes |
|------|--------|------|
| api-gateway | ✅ green | 11 packages ok (-race; 12 at the 2026-10-13 recheck) |
| community | ✅ green | 14 packages ok (-race; 19 at the 2026-10-13 recheck) |
| content-service | ✅ green | 11 packages ok (-race) |
| data-panel | ✅ green | 10 packages ok (-race) |
| game-catalog | ✅ green | 8 packages ok (-race) |
| user-service | ✅ green | 10 packages ok (-race) |
| user-service-rpc | ✅ green | 4 packages ok (-race) |
| realtime-hub | ✅ green | 5 packages ok (-race, picked up via go.work; row added 2026-10-13) |
| web + mobile | ✅ green | lint + typecheck job 34s |

- 8 of the last 30 runs are `cancelled`: all from `concurrency:
  cancel-in-progress` (group name `ci-` joined with `github.ref`, see ci.yml:14)
  cancelling older runs during rapid small pushes — annotation "Canceling since
  a higher priority waiting request exists". Not failures, not flakes; the same
  commit's final state is covered by the later green run. No reruns.
- The latest run is green in both jobs (gofmt+build+test 1m41s; web+mobile 34s),
  0 `FAIL` lines, all 68 packages ok.
- Watch items (no config change unless red): ① ubuntu-latest → Ubuntu 26
  migration (from 2026-10-19), recorded in to-do #2; ② Node 20 deprecation
  warning — handled (2026-10-11 major-version bumps: checkout@v7 /
  setup-node@v7 / setup-go@v7 / pnpm-action@v6, all on node24 runtimes, verified
  green in CI); the macos slot in ios.yml was upgraded in the same batch and
  manually triggered run 38146648735 measured green the same day (prebuild +
  pod install + unsigned simulator build, 14m — the three new actions verified
  on a macOS runner).

### Completed this batch (2026-10-10, kick-off order batch)

| Item | Commit |
|----|--------|
| mobile draft→publish loop (edit-save auto-publishes + draft badge on the detail page) | `2127126` |
| mobile guide-writing Markdown preview toggle (aligned with the web editor) | `5998d22` |
| mobile post comments (list / post / reply / author delete, aligned with web) | `a2144c5` |
| mobile guide comment reply/like/delete (aligned with web, tree assembled client-side) | `f92ccab` |
| mobile guide-writing "save draft, publish later" (aligned with the web dual-state save) | `93fde67` |

## Deferred (external dependency / decided not to start; does not block the main line)

| Item | Blocker | Basis |
|----|------|------|
| ~~Realtime push (WebSocket)~~ | Cleared (realtime-hub M1/M2 landed, 2026-10-11) | docs/realtime-hub.zh.md |
| Full-text search upgrade (Elasticsearch) | No external middleware introduced; LIKE-level keyword search landed 2026-10-12 (`GET /api/v1/posts/search`, app-side filtering — see the community README) | community README long-term item |
| Content recommendation (ML) | No external model service introduced | community README:285 |
| Video content | No object-storage/transcoding infra initiated | community README:286 |
| matchmaking / crawler-jobs / admin-console / mini-program | Decided not to start (2026-10-09) | mobile_plan section 4 (realtime-hub cleared and landed 2026-10-11) |
| On-device regression (local push / deep-link share cards / notification center) | No device or emulator in this environment; substitute = CI gates + `expo export --platform android` bundle walkthrough | mobile_plan risks section |
| iOS backfill (TestFlight sideloading / Universal Links / APNs) | External credentials (Apple team / domain AASA / push certificates) | mobile_plan decision log 2026-10-09 |

## Walkthrough log (2026-10-10 web batch)

> Method: no browser automation on this machine. Substitute = full-stack
> bring-up (six services + vite dev, `VITE_API_GATEWAY_URL` pointed at the
> gateway, the compose JWT_SECRET) + walking every function in
> `src/api/client.ts` through its API contract (curl, same paths and payloads)
> + SPA deep-link HTML probes + the web triple gate (tsc/eslint/build).
> Verdict for the four areas: notification center / favourites / image upload /
> reports — main paths all pass (dedup, idempotency, cross-author 403,
> size/type gates, static file serving, read-all reset to zero).
> 2026-10-11 follow-up batch: 2 technical items fixed (commits below); 3 more
> items were decided under user authorization and landed (see the "pending
> decisions" section for the outcomes).

### Fixed (2026-10-11)

| Orig # | Item | Commit |
|---|----|--------|
| 3 | `fetchMyFavorites` gains a `code !== 200` check: HTTP 200 with envelope code 401 no longer silently renders empty favourites — ProfilePage shows a "partial load failure: my favourites" banner instead (consistent with the rest of the content-domain functions) | `c600193` |
| 4 | Local bare-run JWT secret trap: the etc yaml defaults of content-service / community now align `Auth.JWTSecret` with user-service (`tappi-user-service-jwt-secret-key-2024`), README/ENHANCEMENT config snapshots synced. Previously a bare run without JWT_SECRET let a login token reach content/community endpoints directly (verified by bare-running the three services + cross-service curl with a login token; compose already passes JWT_SECRET explicitly — container behavior unchanged) | `100291c` |

### Pending decisions (contract/product calls — not decided on one's own)

> On 2026-10-11 the user authorized deciding these per the repo's decision
> records (recommendation #2). All three were decided and landed, recorded
> below.

| Orig # | Decision | Basis & method | Commit |
|---|---------|-----------|--------|
| 1 | **Build the web registration entry** (register/login toggle inside the login card; auto-login on successful registration; no email verification / invite system — no mail service in a local deployment, a separate later item) | The web GUIDE already lists registration as a visitor-facing feature; the web register API surface exists (gateway proxy /auth/register) — UI was the only missing piece | `0bd03e5` |
| 2 | **Unify: auth/permission failures return real HTTP statuses (401/403) with an envelope body; business codes (400/404/409/500) stay HTTP 200 + envelope code** | Returning 200 on auth failure bypasses the clients' unified 401 handling (mobile logout-to-login, web error banners); gateway /upload and moderation already work this way — align with the majority. 8 content-service logics migrated to httperr (optional-auth GET no longer silently returns 200 + code 401 when anonymous) | `d2b7c93` |
| 5 | **login uses the same business envelope as register** `{code,message,data:{token,user_info}}`; failures go through httperr → real 401/400 envelopes. Gateway /auth/login changed from an aggregate handler to the same proxy style as /auth/register (envelope passed through verbatim, shape can never drift); web/mobile parse the data field on login | Removes the bare `{token,user_info}` vs envelope double track; a wrong password now yields a readable 401 envelope instead of plain "HTTP 400 login failed". user-service/gateway/web layers plus mobile landed in one batch (pushed together after all gates green) | `beb726f` `ec1509f` `548cb49` |

## Acceptance polishing track

| Item | Status |
|----|------|
| Doc gap fill (notifications / data-panel / favourites / seven-service drift) | ✅ 2026-10-10 (`1e092fa`) |
| CI fully green | ✅ Verified after every push (see batch reports) |
| Walkthroughs | Web four areas ✅ 2026-10-10 (substitute method above; 5 findings logged); mobile ⏸ no device, substitute above |
| mobile M0–M3 + M4 milestone 1 | ✅ All landed by 2026-10-09 |
