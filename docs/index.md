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
      link: /zh/

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

<script setup lang="ts">
// 首次访问按浏览器语言选择文档语言（vitepress 无此能力，GitHub Pages 不支持 _redirects）
// 优先级：localStorage 手动选择 > navigator.language > 保持当前 locale
import { onBeforeMount } from 'vue'
const KEY = 'spawn-lang-auto-picked'
const BASE = '/spawn/'
const ROOT_PATH = BASE
const ZH_PATH = BASE + 'zh/'
onBeforeMount(() => {{
  if (typeof window === 'undefined') return
  const p = window.location.pathname
  if (p !== ROOT_PATH && p !== ROOT_PATH.slice(0, -1)) return
  const picked = localStorage.getItem(KEY)
  if (picked) {{ if (picked !== p) window.location.replace(picked); return }}
  const langs = (navigator.languages?.length ? navigator.languages : [navigator.language]).map(l => l.toLowerCase())
  if (langs.some(l => l.startsWith('zh'))) window.location.replace(ZH_PATH)
}})
if (typeof window !== 'undefined') {{
  window.addEventListener('click', (e) => {{
    const a = (e.target as HTMLElement)?.closest?.('a') as HTMLAnchorElement | null
    const href = a?.getAttribute('href') || ''
    if (href.startsWith(ZH_PATH) || href.startsWith('zh/')) localStorage.setItem(KEY, ZH_PATH)
    else if (href === ROOT_PATH || href === ROOT_PATH.slice(0, -1)) localStorage.setItem(KEY, ROOT_PATH)
  }}, true)
}}
</script>

