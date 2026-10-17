# spawn mobile plan (mobile-app)


> Status: **approved, in implementation** (finalized 2026-10-02, M0 first batch
> kicked off).
> Implementation progress is noted inline on each milestone row; decisions are
> appended to the decision log table.
>
> Related: the overall directory plan is in the root `README.md` (the
> `apps/mobile-app` placeholder); backend capabilities and deployment shape are
> in `docs/development-guide.md`.

## 0. Current-state survey (2026-10-02)

| Item | Status |
|----|------|
| Mobile planning | Only a one-line placeholder in the root `README.md` tree: `apps/mobile-app # iOS/Android client (with a 小黑盒-style stats panel)` — no standalone plan |
| Mobile code | None (`apps/` only holds `web-client`) |
| Reusable backend | Six Go services, all REST + JWT: user-service (register/login/recommendations), game-catalog (game catalog/leaderboards/recommendations), content-service (guides/comments), community (posts/topics/follows/likes), api-gateway (aggregation + proxy), user-service-rpc (internal gRPC, mobile never calls it directly) |
| Frontend stack | web-client is React 19 + TypeScript + Vite; no React Native / Flutter dependency at all |
| Backend not landed | matchmaking / realtime-hub (teaming/realtime) from the README plan not implemented; data-panel (stats) landed its first milestone 2026-10-09 (service + gateway proxy + minimal web panel face) — the mobile panel can consume gateway `/api/v1/stats/*` directly |
| Access pattern status | web-client calls the four services directly (`apps/web-client/src/api/client.ts` hardcodes localhost:88xx); api-gateway(8800) had a narrow endpoint surface (login / games/featured / recommendations + community/content proxies) — now outdated: after BFF phases 1 and 2 (2026-10-04) the gateway gained users/games proxies, the `/home/feed` aggregate and share cards. See the "network" row in section 3 |

## 1. Target platforms

| Platform | Priority | Notes |
|------|--------|------|
| **Android** | P0 (first release) | Main battlefield of the Chinese game community; debug over both emulator and device, release to domestic app stores |
| **iOS** | P1 (immediately after) | Same codebase build; differences only in push certificates and store release flow; before an Apple developer account exists, verify via TestFlight/sideloading |
| HarmonyOS NEXT | Not yet | RN ecosystem support for pure HarmonyOS is not stable; reassess once community solutions mature, out of the first-wave schedule |

## 2. Feature-block split

### 2.1 On mobile (ordered by backend readiness)

| Feature block | Backend dependency | Milestone |
|--------|---------|--------|
| Register / login / secure JWT storage (SecureStore) | user-service ready | M1 |
| Game catalog browsing / search / detail | game-catalog ready | M1 |
| Leaderboards (featured / recommended) | game-catalog ready | M1 |
| Guide reading + comments (browse / post) | content-service ready | M2 |
| Community feed / topic circles / posting / likes / share count | community ready | M2 |
| Follow relations (topics/users) + follow feed | community ready | M3 |
| Profile center (profile / my posts / my likes) | user-service + community ready | M3 |
| Local push (new post / hot list reminders, Expo Notifications) | No backend dependency | M3 |
| Deep links and share cards (app links / universal links) | Gateway-side routing | M3 |

### 2.2 Not on mobile in the first wave (and why)

| Feature | Reason |
|------|------|
| 小黑盒-style stats panel | data-panel landed its first milestone 2026-10-09 (backend + web panel); the mobile panel is an M4 increment consuming gateway `/api/v1/stats/*` |
| Teaming rooms / voice / realtime matching | realtime-hub landed (2026-10-11, including voice signalling passthrough); matchmaking not landed |
| Admin-console features | admin-console domain, out of mobile scope |
| Payment / membership | No such backend domain |
| Mini-program shell | Separate `apps/mini-program`, planned independently |

### 2.3 Mobile-only interaction conventions

- Every list uses pull-to-refresh + bottom-of-list pagination (matching each
  service's List endpoint limit/offset);
- Post/comment text boxes grow to multiple lines; image picking goes through the
  system photo library (single image first, multiple images follow
  content-service image capabilities);
- JWT expiry silently redirects to login (same 401 semantics as web-client).

## 3. Technology choices (relation to the main repo stack)

| Decision | Choice | Rationale |
|--------|------|------|
| Framework | **React Native (Expo managed workflow)** | ① Same React + TypeScript as web-client: types, utilities and team mental model are reused directly, and the README-planned `packages/shared-utils` / `packages/data-models` (TS type contracts) are naturally shared by both clients; ② the backend is entirely REST + JWT with no custom native protocol, so RN fully covers it; ③ Expo's managed native build (EAS) means shipping both platforms without maintaining separate Android/iOS native projects locally |
| Rejected alternative | Flutter | Technically excellent, but it brings in Dart as a second language system with zero reuse against this repo's React frontend; dual-stack maintenance is disproportionate at a single-developer or small-team scale (if a later decision prefers Flutter, the feature blocks and milestones stay unchanged — only this section is replaced) |
| Language | TypeScript (strict) | Same as web-client |
| State / data | Lightweight hand-rolled fetch wrapper + React Query (TanStack Query) | Backend APIs are regular (unified code/message envelope), so no heavy client is needed; React Query covers caching / retry / pagination |
| Network | **Single gateway entry via api-gateway (BFF phase 1, switched 2026-10-04)**: `EXPO_PUBLIC_API_URL` overrides the gateway domain, dev default `host:8800` (Android emulator 10.0.2.2). Gateway-side own aggregate handlers (featured/recommendations/share card/homepage aggregate) + proxies (community/content/users/games; login and register also go through the users proxy from 2026-10-11) cover the app's entire call surface. The earlier concern that pre-widening the gateway for mobile would lengthen the critical path was resolved once BFF phase 1 landed |
| Auth | JWT Bearer + expo-secure-store | Drops into the six services' existing middleware with no changes |
| Directory | `apps/mobile-app` (already placeholdered in the README), wired into the pnpm workspace | Consistent with the monorepo plan |
| Type contracts | `packages/data-models`: hand-written TS types first, plus one `make types` verification script checked against the `.api` files; goctl has no TS generator, so no codegen chain is introduced for it | Avoids building codegen infrastructure just for type sync |

## 4. Milestones

> Pace estimated for "1 full-time frontend developer + backend on demand"; each
> milestone includes integration and on-device verification, with acceptance
> criteria written per milestone. The calendar counts from T0 = the approval
> date.

| Phase | Content | Acceptance criteria | Duration | Progress |
|----|------|---------|----|----|
| **M0 Project skeleton** | Expo scaffold into `apps/mobile-app`, pnpm workspace wiring, TS strict, ESLint aligned with web-client, navigation frame (bottom tabs: Games / Community / Me), CI wired alongside the existing gofmt/build gates (lint + tsc) | Shell app runs on an Android emulator, three tabs switch, CI green | 1 week | Done 2026-10-02, first batch landed (no emulator in this environment; `tsc` + `eslint` + `expo export --platform android` bundle walkthrough substituted for device acceptance — device regression folded into M1) |
| **M1 Browsing + accounts** | Login/register (including JWT SecureStore and 401 redirect), game catalog list/search/detail, leaderboards | Device walk of the full chain login → browse → search → detail; token survives a restart | 2 weeks | Done 2026-10-03, features landed (no emulator or device here: backend contracts walked with two local services + curl end-to-end; the app side substituted with `tsc` + `eslint` + `expo export --platform android` bundle walkthrough; token-survives-restart depends on SecureStore device behavior, so M0+M1 device regression is deferred to the next batch with a device) |
| **M2 Content + community** | Guide reading + comments, feed/topics/posting/likes; pull-to-refresh + bottom-of-list pagination | Device walk of "read guide → comment" and "read post → like → post" | 2–3 weeks | Done 2026-10-03, features landed (no emulator or device: backend contracts walked with local content/community/user services + curl end-to-end — guide list/detail/comment posting, topics/feed/posting/likes/share-count increments; the app side substituted with tsc + eslint + expo export --platform android bundle walkthrough; the "read guide → comment" and "read post → like → post" device walks are folded into M0+M1 and deferred to the next batch with a device) |
| **M3 Relationships + wrap-up** | Follows/follow feed, profile center, local push, deep-link share cards; Android release build (EAS or local gradle) | An installable Android build walks every landed feature; iOS TestFlight sideload verification | 2 weeks | In progress — slice 1 landed 2026-10-03: follow relations (topics/users) + follow feed (the handoff called the community backend "ready", but only follow/unfollow and /topics/following actually existed — the follow-feed endpoints GET /posts/followed and GET /users/following were added by this slice); app side: the community tab's Following mode (including not-signed-in / empty / pagination states), a circle management page, and follow-author on the post detail page. No emulator or device: backend curl end-to-end + `tsc`/`eslint` + `expo export --platform android` bundle walkthrough substituted. Slice 2 landed: profile center "My posts" (community GET /posts?author_id=) + "My guides" (content-service GET /guides?author_id=, with the optional-auth middleware verified — the author sees their own drafts with a Bearer token, anonymous sees only published, draft details are author-only, verified by curling all three services). Slice 3 landed 2026-10-04: "My likes" — community gains a post_likes relation table (created at startup, SQLite/MySQL dual format) + GET /users/likes paginated by like time descending (Like idempotently records the relation, counters keep the monotonically-increasing contract); the app's profile center third block, verified by curling both services (anonymous 401 / empty state / descending order / idempotency). The README's next-step suggestions are long-range architecture-doc and toolchain items and were not converted into deliveries in this slice. Slice 4 landed 2026-10-04: "local push + deep-link share cards" — app side expo-notifications (wired via the SDK 57 plugin; on Android 13 the notification channel is created before the permission prompt; a daily 20:05 local scheduled reminder with a profile-center toggle, degrading on reboot or permission denial; tapping the notification deep-links to the community tab; cold start covered via useLastNotificationResponse); post/feed sharing now opens the system share sheet carrying a `spawn://` deep link (Linking.createURL — counted only on a real share, not on cancel; guests may share but are not counted); the gateway adds a GET /s/p/:id share-card jump page (html/template og meta + spawn deep-link button + web entry button, falling back to a generic card when the post detail cannot be fetched so the deep link never dies). Two walkthrough findings logged and fixed: the gateway community proxy was missing three M3 routes (GET /posts/followed, /users/following, /users/likes — the app already called them in slice 3 and would 404 through the gateway; added in this slice and verified by curl that anonymous 401 passes through); the share-card CSS misused a Sprintf-style `88%%` (html/template emits it verbatim as invalid CSS — changed to `88%` with a regression assertion). expo export bundle walkthrough: reminder text / toggle text / `/community` landing path / POST_NOTIFICATIONS are all in the package (`spawn:///post` is assembled at runtime by Linking, so the literal is absent from the bundle by design). Slice 5 landed 2026-10-04: Android release packaging via local gradle — `app.json` gains `android.package` (com.tappi.spawn) + `prebuild:android`/`build:android:release` scripts; `expo prebuild` generates android/ (CNG, gitignored) then `gradlew assembleRelease` runs through (the toolchain auto-downloads JDK 17; the Expo template's release build uses the debug signingConfig so it installs locally); aapt/apksigner static walkthrough passed (package name / permissions including POST_NOTIFICATIONS / deep-link scheme / signature). Device install and full-feature regression remain deferred (no device in this environment) |
| **M4 (increment, separate item)** | The data-panel stats panel after data-panel lands; switch to the unified gateway after api-gateway becomes a BFF | — | In progress: BFF phase 1 completed ahead of M4 (2026-10-04) — the gateway gains users/games proxies (POST /auth/register, GET /users/:id, GET /games, GET /games/:id), coexisting with the existing community/content proxies and the gateway's own handlers (the integration test registers every route the way production does, which exposed a route conflict), and the mobile-app base URL converges on the single gateway address (overridable via `EXPO_PUBLIC_API_URL`). A cross-cutting defect logged and fixed during the walkthrough: proxying r.Body directly with unknown length goes chunked, and go-zero upstreams fail to parse a chunked request body (reproduced against user-service with a direct chunked curl returning `field is not set`) — the proxy now buffers and forwards with a fixed length (1 MiB cap, 413 above that, with a contract test); five services walked end-to-end via curl (register/login/profile, game list/detail/featured, guides/comments, topics/posts/likes/shares, follow feed/my likes, anonymous 401 pass-through, share card) and the app's entire call surface goes only through :8800; the web-client direct topology and the gateway's existing routes are unchanged. Phase 2 landed the same day: the gateway `GET /home/feed` list aggregate (featured games / hot posts / topics / guides pulled concurrently, per-group field trimming — large fields such as body and images are not sent, the body is truncated to a 60-character summary; a single upstream failure degrades only that group to empty and marks `degraded`, no wholesale 5xx), with the app consumption point landing the same day: a new Discover tab (`(tabs)/discover.tsx`) rendering all four blocks from one request, pull-to-refresh, a degraded banner, empty/error retry — the bundle walkthrough confirms the text and routes are in the package; web-client unaffected. The data-panel panel is still a separate M4 item. **M4 milestone 1 landed (approved 2026-10-09: data-panel kicks off; matchmaking / realtime-hub / admin-console / mini-program stay off; device regression stays unscheduled without a device):** new `services/data-panel` (port 8896 / metrics 9097) — SQLite/MySQL dual-driver `player_stats` table (UNIQUE(user_id, game_id)); stats ingest is a monotonic increment accumulation (JWT-protected `POST /api/v1/stats/records`, single-statement upsert `col=col+delta`, negative delta 400, game_title overwritten only when non-empty, last_played_at advances to the max, ownership always taken from the token's user_id); queries are public (`GET /api/v1/stats/users/:id/summary`, `.../games?limit&offset`, `.../games/:game_id`, win_rate/kd read-side 4 decimal places); an empty table ships with embedded seed data; TTL+LRU caching. api-gateway adds five `/api/v1/stats` proxy routes (the integration test gains a data-panel stub and full assembly); web-client adds a `/stats` page (KPI cards + per-game detail table, gateway address only); compose/nightly/Prometheus wired in step (seven services). The web minimum face is live. The mobile consumption point landed the same day (M4 increment): client.ts gains stats fetchers (fetchStatsSummary / fetchUserGameStats / fetchUserGameStat, via the single gateway address), a new bottom tab Stats (KPI summary card + per-game detail cards, a login entry when not signed in, pull-to-refresh / error retry / empty state) and a Stack dynamic route `stats/[gameId]` (single-game stats detail: match/combat sections + tier-score badge); no emulator here, so tsc + eslint + expo export --platform android bundle walkthrough substituted (the stats tab and detail routes plus the KPI/detail/login-prompt text are all in the bundle); device regression is folded into the next batch with a device |

Risks and dependencies:

1. **Backend CORS / cleartext HTTP**: real devices block cleartext http by
   default (Android `usesCleartextTraffic`); during development use a LAN IP plus
   the allowance config, production requires HTTPS — a public domain and
   certificate are needed before release (the current compose has no TLS
   termination; this is a new item in the deploy domain);
2. **iOS developer account**: without one, iOS is verified only on a
   simulator/sideloading, which does not affect the Android first-release pace;
3. **Image upload**: cleared — gateway `POST /upload` local-disk storage landed
   2026-10-10 (`6642132`) and is wired on both web and mobile; sending uploads
   to an external CDN in production is a new decision item.

## 5. Decision log

| Date | Decision | Notes |
|------|------|------|
| 2026-10-02 | Finalized per recommendation: Android P0 / iOS P1; feature split per section 2; React Native (Expo) + TypeScript; milestones M0–M3 at about 7–8 weeks total | User authorized deciding without review; M0 kicked off the same day. The Flutter rejection rationale remains valid |
| 2026-10-05 | iOS and HarmonyOS repositioned as **low-priority backfill**: write code only; iOS verification runs on a GitHub macOS runner (no local time, does not block progress); HarmonyOS has no toolchain so writing code that compiles is enough — do it when free, continue other tasks otherwise, never block the main progress and never treat it as the main line | User correction order, overriding the 10-02 "iOS P1 immediately after" pace; M3's iOS TestFlight sideload verification becomes a backfill item under this policy |
| 2026-10-07 | (Executed autonomously under the 10-05 backfill policy, not a new decision) iOS gains `ios.bundleIdentifier = com.tappi.spawn` to match Android: iOS previously had no bundle id, so each prebuild relied on Expo deriving a default and the identity was unstable, affecting `spawn://` deep-link registration and any future sideload/push certificates; verified by the iOS workflow (497d4cd) build | Backfill item after the main queue cleared; HarmonyOS stays "not yet" per section 2 (RN's pure-HarmonyOS ecosystem is not stable) |
| 2026-10-09 | (Executed autonomously under the 10-05 backfill policy, not a new decision) iOS permissions and store keys completed (6f30ad2): `app.json` wires the `expo-image-picker` plugin and declares the photo-library usage text (`photosPermission` — post image picking uses `launchImageLibraryAsync`, and iOS crashes at runtime without `NSPhotoLibraryUsageDescription` and would be rejected at review; only the photo-library read permission actually used is declared, not camera or microphone), and `ios.infoPlist` gains `ITSAppUsesNonExemptEncryption=false` (the app uses only standard HTTPS, saving the per-release encryption questionnaire in TestFlight). Verification method: local `expo config --type prebuild` expansion + `expo prebuild -p ios --no-install` static check of the Info.plist (photo-library text / encryption key / `CFBundleURLSchemes=[spawn, com.tappi.spawn]` all present); the macOS runner (ios.yml manual trigger) ran prebuild + pod install + unsigned simulator build all green (15m42s). Deferred ledger: real device install and permission-prompt testing, Universal Links (needs an Apple team plus domain AASA deployment — no code written without credentials), APNs remote push certificates (the local-notification code is in place on both platforms, see M3 slice 4); device regression folded into the next batch with a device | Backfill item after M4's main queue cleared; at this point no substantive writable iOS config remains — everything left depends on external credentials or devices, and stays backfill until those arrive |
