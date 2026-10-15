---
layout: home

hero:
  name: spawn
  text: Game community platform
  tagline: Four capabilities — game catalog, guides, community, realtime — across two clients, web and mobile; seven go-zero microservices plus a WebSocket realtime hub, in one repository.
  actions:
    - theme: brand
      text: Getting Started
      link: /guide/getting-started
    - theme: alt
      text: Architecture
      link: /architecture/overview
    - theme: alt
      text: Development Guide
      link: /development-guide
    - theme: alt
      text: 简体中文
      link: index.zh.md

features:
  - title: Game catalog & leaderboards
    details: game-catalog manages games, tags, featured and recommended lists. SQLite/MySQL dual driver; filtering, sorting and pagination all finish server-side.
  - title: Guides & content
    details: content-service manages guides and comments, with draft and published states, keyword/tag filtering, sorting and favourites. A guide's game info is pulled across services from game-catalog to fill it in.
  - title: Community interaction
    details: community has posts, topics, follows, likes, in-app notifications, direct messages and report moderation. The follow feed is time-descending with id as the tie-break inside one second; likes live in their own relation table; notifications deduplicate on (recipient, actor, type, target).
  - title: Access & aggregation
    details: api-gateway owns the login/registration proxy plus five proxy routes; JWT issuing lives in user-service. /home/feed pulls featured games, hot posts, topics and guides in one request; if one upstream fails only that group degrades, the rest return normally.
  - title: Realtime channel
    details: realtime-hub provides WebSocket long connections (JWT verified), /internal/publish fanout, presence and voice signalling passthrough, with Redis pub/sub relay across instances; the gateway /ws tunnel passes straight through.
  - title: Multi-client apps
    details: web-client on React 19 + Vite; mobile-app on Expo, with local push, deep-link share cards, a notification center and guide authoring/editing, Android builds via gradle. The mini-program and the admin console have not started.
  - title: Observability
    details: each of the seven services exposes one Prometheus /metrics port (9091–9097); logs are structured JSON into Loki, and deploy/ ships compose files for Prometheus, Loki and Grafana.
---
