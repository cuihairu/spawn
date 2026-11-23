# Joint sandbox & acceptance (GP-005)

The joint sandbox couples the Spawn compose stack with the external Courier gateway (croupier, frozen repo) so one example game can be assembled end to end: partner onboarding → game creation → version submission → review → publish → server → Courier mapping → payment product → gift pack → activity → player-side smoke. The kit lives in [`deploy/sandbox/`](https://github.com/cuihairu/spawn/tree/main/deploy/sandbox): env sample, wiring README and the `seed-example-game.sh` bootstrap script. This page records the version manifest and the acceptance dependencies that cannot be satisfied inside the sandbox (real channels, real game servers, authorization, physical devices).

## Version manifest

Components pinned by the repo as of 2026-11; verify against the referenced files when versions move.

| Component | Version | Source |
| --- | --- | --- |
| Go toolchain | go 1.27.3 (modules go 1.26.0) | `go.work` |
| Service framework | go-zero v1.10.3 | `services/*/go.mod` |
| api-gateway / microservices | docker compose, images built from `services/*` Dockerfiles | `docker-compose.yaml` |
| Web client | React ^19.3.0, Vite ^8.3.2 | `apps/web-client/package.json` |
| Mobile client | Expo SDK 57 (`expo ~57.0.26`), React Native New Architecture | `apps/mobile-app/package.json` |
| Courier gateway | croupier ([cuihairu/courier](https://github.com/cuihairu/courier), external frozen repo, gRPC contract) | research doc §4 |
| Databases | SQLite file stores per service (`DATASOURCE=file:...`) | compose env |

Seed accounts produced by the bootstrap script: `sandbox-admin` (platform_admin via `ADMIN_USER_IDS`), `sandbox-partner`, `sandbox-player`, default password `Sandbox!12345678` — sandbox-only, change or drop before exposing the stack.

## What the sandbox covers

Everything that runs inside the compose stack is exercised by `deploy/sandbox/seed-example-game.sh` and is repeatable on any machine with Docker:

- admin bootstrap, partner role grant, game/ownership creation
- version submission → review approval → game + version publish (package upload to platform hosting while in draft)
- controlled download through the gateway (anonymous 401, Bearer gets the uploaded bytes, streaming route)
- game server creation and player reservation
- Courier mapping + capability declaration (adapter admin, local side)
- payment product configuration, gift pack claim (per-user and total caps), multiplier activity evaluation surfaces
- player-side smoke: activity listing, server reservation, gift pack claim

Courier-dependent endpoints (session tickets, agent login, order placement through teller) answer 502 until a croupier instance joins the compose network — the interface contract is what the sandbox pins, not a live instance.

## Acceptance dependencies outside the sandbox

These acceptance steps require assets no repo config can mint. They are recorded here per GP-005 and stay gated on their own backlog items.

| Dependency | Blocks | What is missing |
| --- | --- | --- |
| Real channel credentials | GP-202 real-channel payments | Channel merchant IDs/keys, callback secrets; sandbox HMAC secret must not be used |
| Real game server | GP-206 end-to-end play | A croupier-connected game session server reachable from the sandbox network |
| Authorization artifacts | GP-202/203 production wiring | Signed channel agreements / production gateway credentials for teller Deliver |
| Physical devices | GP-105 Android install flow | Android device or emulator for APK download → install → launch; SHA-256 package verification needs a native file-hash module (expo-crypto has no streaming digest, SDK 57) |
| Release sign-off | versioned releases | Per repo policy, releases are cut only on explicit user instruction after acceptance passes |

## Acceptance checklist

Run on a fresh sandbox after `seed-example-game.sh` succeeds. Items 1/2 are asserted by the seed itself (upload in draft, then gateway visibility, anonymous 401 and Bearer download match); 1/2/3/5 were verified live on 2026-11-21 (caps: per-user 3rd claim 409, total-exhausted 409, idempotent replay returns the same claim id; session endpoints answer 502 without croupier); items 4/6 were verified live on 2026-11-23 through courier-gateway-integrator (order creation goes through Courier `POST /v1/payments/orders`, the callback is forwarded from spawn `/callback/sandbox` after HMAC verification).

- [x] Example game visible to players (`GET /games/:id` via gateway 8800)
- [x] Published version downloadable through the gateway streaming route (Bearer required)
- [x] Gift pack claim honors per-user and total caps (409 on exhaustion, idempotent replay wins)
- [x] Activity reward evaluation fires on paid orders (payments → promotions S2S) — verified 2026-11-23: after PAID the facts land in `promotion_evaluations` (done), `POST /internal/reward-evaluations` answers 200, and `reward_grants` writes `granted` (rate_bp 15000 multiplier, extra 50)
- [x] Courier session endpoints answer 502 (no croupier); ticket flow with croupier belongs to GP-206
- [x] Sandbox payment callback (`POST /callback/sandbox/...`) flips an order to PAID with valid HMAC — verified 2026-11-23: after signature check the forward resolves the scope from the order projection (order_01a2064c… CREATED → PAID → delivery ledger DELIVERED)

Items needing the external dependencies above belong to GP-105/GP-202/GP-206 acceptance and are tracked there, not in this page.
