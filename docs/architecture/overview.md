# Architecture overview

[English](overview.md) | [简体中文](overview.zh.md)

> This document is the vision and direction; **the currently landed service
> boundaries, ports, gateway proxy topology and data flow** are in
> `topology.md` in the same directory (the as-built document — when the two
> conflict, the as-built document wins).

## Goal

Turn the game catalog, guides, community, stats and teaming into one platform
covering Web/App/mini-program. Backend and automation follow in later phases.

## System layers

1. **Experience layer (`apps/`)**: Web, mobile, backend-admin and mini-program
   clients share one design system and state management, reach backend services
   through BFF/REST (GraphQL is not implemented), and interact with the realtime
   channel (WebSocket/RTC).
2. **Business service layer (`services/`)**: microservices split by domain,
   exposed through the gateway, sharing data models and configuration.
3. **Data and realtime layer**: `data-panel`, `crawler-jobs` and the realtime
   push service handle scraping, computation and realtime sync.
4. **Platform support layer (`platform/`)**: infrastructure, CI/CD and
   observability configuration all live here.

## Core modules

- **User & Social** (`user-service`, `community`, `matchmaking`): accounts,
  friends, activity feed, teaming, voice/text communication.
- **Content & CMS** (`content-service`, `admin-console`): guide/news authoring,
  moderation, campaign configuration.
- **Game Catalog & Discovery** (`game-catalog`, `web-client`): game catalog,
  leaderboards, tags, recommendation feed.
- **Data Panel** (`data-panel`, `mobile-app`): stats ingestion,
  character/equipment statistics, player profiles.
- **Automation** (`crawler-jobs`, `tools/` scripts): auto-sync of external
  store data, event info and announcements.

## Data flow and interaction

1. **Input**: `crawler-jobs` scrape official stores/events/announcements into a
   data warehouse or cache; `data-panel` pulls stats data through official APIs
   or partner interfaces.
2. **Processing**: game-catalog/content-service/community persist by data model
   (SQLite/MySQL dual-driver; Postgres/ElasticSearch are candidates);
   leaderboards and recommendations come from in-service queries and
   cross-service calls (Kafka/PubSub message buses are candidates).
3. **Output**: api-gateway sends the aggregated data to each client;
   realtime-hub handles realtime WebSocket push (in-app notifications / presence
   / read receipts / WebRTC signalling, through the gateway `/ws` tunnel,
   multi-instance Redis pub/sub, remote push via webhook bridge).
4. **Feedback**: stats uploads from apps go through gateway
   `POST /api/v1/stats/records` straight into data-panel (shared-utils SDK and a
   standalone data pipeline are planned); recommendations and leaderboards
   consume the query APIs of data-panel and game-catalog.

## Tech stack: as-built and candidates

As-built (see `topology.md`): backend go-zero REST plus a little gRPC, frontend
React/Vite and React Native (Expo), storage SQLite/MySQL dual-driver,
observability Prometheus + Promtail/Loki/Grafana, deployment docker compose.

Candidates (unselected, decided with the evolution phases): mini-program
Taro/UniApp, PostgreSQL as primary store, ElasticSearch search, Kafka message
bus, Terraform + Kubernetes. The rest of this document is vision-level too —
where it conflicts with `topology.md`, `topology.md` wins.

## Evolution roadmap

1. MVP: land the game catalog, guides and community basics first, wire up the
   user system and content publishing.
2. Phase 2: stats data, player profiles, mini-game teaming, realtime push.
3. Phase 3: crawlers, data analysis, event center; connect backend operations
   and the data dashboards.

## Development conventions

- All services/apps share the toolchain in `packages/config` (make targets /
  lint config / env templates); code is managed through the pnpm workspace.
- Each service records its API and runbook in a `README.md` in its directory
  (a `docs/` subdirectory is planned).
- Inter-service interface contracts are covered by per-service unit/integration
  tests; cross-service `tests/` contract tests are planned (see
  `tests/README.md`).
- CI/CD automates test runs, the docs build and nightly builds through
  `.github/workflows` (`platform/` is planned).
