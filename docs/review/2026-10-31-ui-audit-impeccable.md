# spawn UI audit — web-client

**Date:** 2026-10-31 · **Surface:** apps/web-client (React 19 + Vite + React Router 7) · **Method:** rendered audit (Playwright 1.64.1, Chromium 1243) at 1440x900 and 390x844, computed-style contrast measurement, plus `impeccable detect`.

**Note on environment:** the api-gateway (localhost:8800) and the user/game/content/community services were partially reachable. The web client is configured without `VITE_API_GATEWAY_URL` (no `.env`), so pages that call services directly (home, games, guides, community, guide detail) get CORS-blocked and exercise their error/empty states; pages that use the gateway BFF (`/home/feed`, stats) render real data. Data-rich states for the service-direct pages could not be fully verified locally. This is noted per-issue below.

## Overall critique

Tappi's web client is a competent, coherent dark dashboard that gets the big things right — a single visual language (dark navy + indigo/purple accent), a consistent card system, and genuinely thoughtful empty/loading/error states on every page (discover, games, community, notifications, messages, profile all render a graceful "no data" or a humanized failure banner with a retry affordance). It is also, however, a classic example of a UI that was designed and tested almost exclusively at desktop width: at 390px the global nav breaks hard (links stack into a cramped vertical column and overflow the viewport, pushing the 攻略 item fully off-screen and causing a horizontal page scroll), and it leans on a single saturated purple for every primary action that fails WCAG AA for white button text (3.29:1 on #7083ff, 2.78:1 on green #4caf50). Error copy is the weakest point: several pages surface the raw browser string "Failed to fetch" to Chinese users. There is no DESIGN.md, so the "system" is implicit in CSS — workable, but nothing documents intent. The layout, typography, and spacing are otherwise disciplined; the gaps are responsive behavior and a11y contrast, not taste.

## Issues (P0 → P3)

### [P1] Global nav breaks on narrow screens — horizontal overflow, off-screen item

- **Screenshot (before):** `docs/review/2026-10-31-ui-audit-impeccable/web-nav-overflow-before-home-narrow.png`, `web-nav-overflow-before-discover-narrow.png`
- **Location:** `apps/web-client/src/App.css:24` (`.nav-container`), `:45` (`.nav-links`) — no media query existed anywhere in App.css
- **Why it matters:** At 390px, measured `document.documentElement.scrollWidth = 472` vs `clientWidth = 390` → the whole page scrolls horizontally on mobile. The 5 nav links (`首页/发现/游戏库/社区/攻略`) are `display:flex; flex-wrap:nowrap`, so they render as a single overflowing row (首页 x=104 … 攻略 x=424–472, fully outside the viewport). The 攻略 link is unreachable without sideways scrolling, and the nav occupies ~100px as a cramped vertical-looking column.
- **Fix (done):** Added `@media (max-width: 768px)` in `App.css` that sets `flex-wrap: wrap` + reduced gap/padding on `.nav-container` and `.nav-links`. Verified `scrollWidth = 390 == clientWidth`, logged-out and logged-in (auth nav has 8 extra items).
- **Commit:** `ed79698 fix(ui): stop nav horizontal overflow on narrow screens`

### [P1] Primary-action button text fails WCAG AA contrast (white on saturated purple/green)

- **Screenshot:** `docs/review/2026-10-31-ui-audit-impeccable/web-guides-desktop.png`, `web-games-desktop.png`
- **Location:** `apps/web-client/src/App.css:208` (`.auth-card button` gradient), `pages/guides.css:74` (`.guide-search-submit`), `:108` (`.filter-tab.active`), `:115` (`.create-guide-btn`), `:377` (`.action-btn`), `:506` (`.publish-btn`), `:536` (`.error-page`), `:805` (`.guide-tag-chip.active`), `pages/games.css:70` (`.genre-chip--active`), `pages/community.css:198` (`.primary-btn`), `components/comments/comments.css:77` (`.comment-submit-btn`)
- **Why it matters:** All primary CTAs use white text on a saturated fill. Measured (WCAG 2.1, vs the actual composite background): white on `#7083ff` = **3.29:1**, `#9f6bff` = **3.49:1**, `#7c6cf0` = **3.99:1**, green `#4caf50` = **2.78:1**. All fall below the 4.5:1 AA requirement for normal-size (16px / 400–600 weight) button labels. The green "创建攻略 / 保存并发布" buttons are the worst at ~2.8:1 — visibly low contrast, and a real readability problem on the editor's primary action.
- **Fix (done):** Hue-preserving lightness adjustment only (brand hue identity unchanged): `#7083ff→#5a68e8` (4.59:1), `#9f6bff→#8650e6` (4.88:1), `#7c6cf0→#5f52d6` (5.73:1), `#4caf50→#2e7d32` (5.13:1), plus hover shades `#5f6fe6→#4f56d4`, `#45a049→#286e2c`. Re-ran the live contrast probe: **0 failures** on `/`, `/guides`, `/games`. Non-text uses of these colors (focus rings `border-color:#7083ff`, blockquote `border-left`, accent text) were left untouched — they already pass 3:1 non-text.
- **Commit:** `05cfe57 fix(ui): raise button text contrast to WCAG AA`

### [P2] Raw English "Failed to fetch" surfaced in error banners

- **Screenshot (before):** `docs/review/2026-10-31-ui-audit-impeccable/web-error-before-games-desktop.png`, `web-error-before-community-desktop.png`
- **Location:** `apps/web-client/src/pages/HomePage.tsx:36,61`, `GamesPage.tsx`, `GuidesPage.tsx`, `CommunityPage.tsx` — each catch handler does `setBanner((err as Error).message ?? '…')`, and a failed `fetch` throws `TypeError: Failed to fetch`
- **Why it matters:** A Chinese-language app shows the browser's raw English string "Failed to fetch" as the headline error on home, games, guides and community. No explanation of what happened or what the user should do; on home there is no retry at all.
- **Fix (done):** In `apps/web-client/src/api/client.ts` `request()`, wrap the `fetch` call; a `TypeError` (network/DNS/CORS/connection-refused) now throws `Error('网络请求失败，请检查网络连接或稍后重试')`, so every page banner renders humanized Chinese. Verified live on `/games` (banner now shows the friendly message).
- **Commit:** `49e6518 fix(ui): humanize network error message`

### [P2] Game detail error state renders with no heading or context

- **Screenshot:** `docs/review/2026-10-31-ui-audit-impeccable/web-games_1-desktop.png`
- **Location:** `apps/web-client/src/pages/GameDetailPage.tsx` (error branch)
- **Why it matters:** When the game fetch fails, the page shows only a floating error banner + a "返回游戏库" link — no page title, no explanation, no retry. The user is left with a mostly-empty screen and no way to know what page they were on.
- **Fix (done):** Render the page title (e.g. "游戏详情") with a friendly message and a 重试 button in the error branch, mirroring the guides/community pattern. Commit: `30c181c fix(ui): add heading and retry to game detail error`.

### [P3] Impeccable detector: overused font + side-tab accent (slop)

- **Location:** `apps/web-client/src/App.css:147` and `src/index.css:11` (`font-family: 'Inter' …` — overused font), `pages/guides.css:669` (`.markdown-body blockquote { border-left: 3px solid #7083ff }` — side-tab accent)
- **Why it matters:** Both are the recognizable "AI-generated UI" tells the impeccable detector flags (Inter is the most overused face; a thick one-side accent border on a card is a slop signal). Not functional breakage.
- **Fix (todo):** Consider a more distinctive typeface and a subtler blockquote accent; leave until a design pass (changing them is a visual-direction decision, out of scope here).
- **未修原因：** 字体与 blockquote 强调属视觉方向决策，报告本身将其列为「design pass / out of scope」；换字体/去侧边条会改变品牌观感，需用户拍板，本整改不动视觉方向。

### [P3] No `prefers-reduced-motion` handling

- **Location:** `apps/web-client/src/App.css` — `transition: transform 0.2s` (nav-brand hover), `transition: all 0.2s` (buttons/links), no `@media (prefers-reduced-motion: reduce)` anywhere
- **Why it matters:** Users who disable motion still get the hover scale/color transitions. Minor (transitions are subtle), but the audit dimension requires it.
- **Fix (done):** Add a global `@media (prefers-reduced-motion: reduce) { * { transition: none !important; } }`. Commit: `f52ca04 fix(ui): honor prefers-reduced-motion on web client`.

### [P3] External cover images fail to load → dark placeholder blocks

- **Screenshot:** `docs/review/2026-10-31-ui-audit-impeccable/web-discover-desktop.png`
- **Location:** `apps/web-client/src/pages/DiscoverPage.tsx:92` (`.feed-game-cover` uses `images.tappi.dev` URLs); same pattern in `GameCard.tsx`
- **Why it matters:** The cover host is not reachable in this environment, so every game cover renders as an empty dark rectangle — the discover feed looks broken. If the host is genuinely used in prod this needs an `onError` fallback.
- **Fix (done):** Add an image `onError` fallback to a branded placeholder, or serve covers from the gateway. Commit: `fb28fc9 fix(ui): add cover image fallback on load error`（`GameCover` 预检 + `.cover-fallback` 渐变；另 `2b00365` 修订 effect 内 setState 合规）。

## Notes

- **DESIGN.md absence:** No `DESIGN.md` (and no `PRODUCT.md`) exists anywhere in the repo. The design system is implicit in `src/App.css` + per-page CSS and the `colors` tokens in the mobile app; there is no written record of intent, tokens, or rationale. Documented here as a gap (the `impeccable context` run reported `EXISTING_VISUAL_SYSTEM` / missing design doc).
- **Unreachable / unverified states (no backends, no auth):**
  - Auth-gated routes `/stats`, `/profile`, `/notifications`, `/messages`, `/guides/new`, `/guides/:id/edit` were audited by injecting a real JWT (registered a fresh account through the gateway). They render correct empty/error states (stats shows zeroed tiles; profile shows 4 empty sections with a "部分内容加载失败" banner; notifications/messages show empty + "…加载失败" banners).
  - Data-rich states for service-direct pages (games list with items, guide detail with content/comments, community topics/posts) could not be fully verified because those endpoints are CORS-blocked from the browser (no `VITE_API_GATEWAY_URL` configured). The discover page (BFF `/home/feed`) was verified data-rich and renders correctly.
  - Logged-in nav with badges (unread counts) renders but the count fetch fails silently (by design).

# spawn UI audit — mobile-app (native)

**Date:** 2026-10-31 · **Surface:** apps/mobile-app (Expo 57 / React Native 0.86 / expo-router) · **Method:** source-level native audit per `reference/audit.native.md`.

**Rendered verification did NOT run.** This is an honest limitation, not a failure: the app has no `react-native-web`/`react-dom` in deps (confirmed in `package.json`), and there is no Android/iOS emulator or simulator in this environment, so it cannot be rendered in a browser or on a device. All findings below are from reading `src/app/**` routes, `src/api`, `src/lib`, `src/constants` — the audit is a source-level technical review of a native surface. Additionally, `npm run typecheck`/`npm run lint` could not be executed: `node_modules/.bin/tsc` and `eslint` binaries are not installed in `apps/mobile-app` (dangling symlink), so those gates could not be run; the one change made (adding `accessibilityLabel` props) was verified by inspection and is a zero-render-risk prop addition.

## Overall critique

This is a genuinely well-engineered native app on the data/state side: every screen uses `FlatList` with pagination + `RefreshControl` (games, community, rankings, guides, messages, topics, user), static content uses `ScrollView`, input screens wrap in `KeyboardAvoidingView`, and every screen has a first-class loading / error / empty state with a retry affordance. The token palette (`colors.ts`) is small and its contrast math is solid — dark text `#1a1105` on primary orange `#e8710a` is 6.03:1, muted gray on card is 6.15:1, error red is 6.53:1. But it fails native users on two fronts: accessibility and platform conformance. Screen-reader users are effectively locked out of navigation — of ~218 `Pressable` controls, only 3 had any `accessibilityLabel`, and 18 icon-only back/close/create buttons announced nothing (now fixed). The root navigator sets `headerShown: false` globally, so no screen has a native header; instead every pushed screen hand-rolls a "‹/← 返回" text button and clears the status bar with a hardcoded `paddingTop: 64` (login uses 72) rather than `useSafeAreaInsets()` — a platform-conformance and inset-guessing problem. Touch targets are pervasively below the 44pt minimum (chips and tabs at 28–36px). The structure is excellent; the polish that makes it feel native and usable by everyone is missing.

## Audit health score

| # | Dimension | Score | Key finding |
|---|-----------|-------|-------------|
| 1 | Accessibility | 1 | 18 unlabeled icon-only controls (fixed); touch targets under 44pt pervasive |
| 2 | Performance | 3 | FlatList + pagination + RefreshControl everywhere; no obvious jank paths |
| 3 | Appearance & Theming | 3 | Token palette exists, contrast passes; ~30 hard-coded hex values scattered in tsx |
| 4 | Platform Conformance | 2 | `headerShown:false` globally, hand-rolled back buttons, hardcoded status-bar paddings |
| 5 | Adaptivity | 2 | No `useWindowDimensions`/size classes; fixed pixel layout |
| **Total** | | **11/20** | Acceptable (significant work needed) |

## Issues (P0 → P3)

### [P1] Icon-only controls have no accessible label (18 controls)

- **Location (all `apps/mobile-app/src/app/`, before fix):** `(tabs)/index.tsx:95` (clear-search), `compose.tsx:117,174`, `game/[id].tsx:54`, `guide-compose.tsx:161`, `guide/[id].tsx:292,354`, `guides.tsx:112,119`, `messages.tsx:116`, `notifications.tsx:100`, `post/[id].tsx:240,257,278,453`, `rankings.tsx:38`, `topics.tsx:125`, `user/[id].tsx:71`
- **Why it matters:** These are `Pressable` elements whose only child is an `<Ionicons>` (chevron-back back buttons, close buttons, create-outline). With no `accessibilityLabel`, VoiceOver/TalkBack announces nothing or a bare "button" — a screen-reader user cannot identify or use the back/close/create controls on any detail screen. Of ~218 `Pressable` instances in the app, only 3 carried a label (`community.tsx:267`, `profile.tsx:363`, `guide/[id].tsx:260`).
- **Fix (done):** Added `accessibilityLabel` to each (返回 / 关闭 / 清空搜索 / 创建攻略), chosen from the control's actual action. Re-scan confirms 0 remaining unlabeled icon-only `Pressable`s.
- **Commit:** `a759ffa fix(ui): label icon-only controls for screen readers`

### [P2] Touch targets pervasively below 44pt

- **Location:** shared style objects across `apps/mobile-app/src/app/` — chips/tabs at 28–32px (`guides.tsx:155,168` `chip`; `guide-compose.tsx:182,199,228,233` `chip`; `(tabs)/community.tsx:287,295` `topicChip`, `:338,343,348` `modeBtn`), `retryBtn` ~36px (in ~10 screens), `back` ~32px, `rankEntry` ~40px (`(tabs)/index.tsx:105,110`), `followBtn` 34–36px (`post/[id].tsx:306`, `topics.tsx:187`), `switchMode` ~36px (`login.tsx:135`), `readAllBtn` ~32px (`notifications.tsx:104`)
- **Why it matters:** The Apple HIG minimum is 44pt (Material 48dp). ~45 Pressable instances render below that, and most have no `hitSlop` to compensate (only the back buttons do). Small targets cause mis-taps and are especially hard for motor-impaired and large-finger users.
- **Fix (done):** Raise the shared `chip`/`retryBtn`/`back`/`followBtn`/`modeBtn` styles to `minHeight: 44` (or add `paddingVertical`), rather than editing each instance. Commit: `664a3b2 fix(ui): raise mobile touch targets to 44pt`（39 个样式对象 minHeight/minWidth 44，17 文件）。

### [P2] No native headers; status bar cleared with hardcoded paddings

- **Location:** `apps/mobile-app/src/app/_layout.tsx:18` (`<Stack screenOptions={{ headerShown: false }} />`); every pushed screen hand-rolls a back button and hardcodes `paddingTop: 64` (e.g. `rankings.tsx:100`, `guides.tsx:254`, `game/[id]`, `post/[id].tsx:498`, `notifications.tsx:196`, `topics.tsx:216`) or `72` (`login.tsx:158`)
- **Why it matters:** `useSafeAreaInsets()` is never used (`SafeAreaView`/`useSafeAreaInsets` absent from the whole tree). The 64/72px guesses clear the status bar on the common notch devices but not reliably on Android status-bar variants, foldables, or when the OS inset differs — content can slide under the status bar or sit with excess space. There is also no native header/title, so detail screens depend on a hand-rolled text back link. iOS edge-swipe-back still works (native Stack), but the affordance and inset handling are guesswork.
- **Fix (done):** Either enable native headers on the root `Stack` (per-screen `headerShown`) or wrap content in `SafeAreaView`/`useSafeAreaInsets()`; remove the hardcoded paddings. Commit: `6a5a771 fix(ui): use safe area insets for header padding`（10 屏改用 `useSafeAreaInsets()`，删 64/72 硬编码；expo-router ExpoRoot 已含 SafeAreaProvider）。

### [P2] Hard-coded hex colors scattered outside the token palette

- **Location:** `apps/mobile-app/src/app/**` — `#1a1105` (button text, 6 screens), `#ff6b6b` (error/destructive), `#dc2626` (`messages.tsx:355`), `#fff`, `RANK_COLORS` (`rankings.tsx:9`: `#ffd700/#c0c0c0/#cd7f32`), plus `#c9c4ff`/`#a99cf5`-style accents in web-only CSS
- **Why it matters:** ~30 values bypass `constants/colors.ts`, so theming is inconsistent and a dark/light scheme can't be introduced without a hunt. Contrast is fine (checked: `#1a1105` on orange 6.03:1, `#ff6b6b` on bg 6.53:1, `#dc2626` white 4.83:1, rank colors 5.1–11.5:1), so this is a maintainability issue, not a readability one.
- **Fix (done):** Fold these into `constants/colors.ts` as semantic tokens (danger, onPrimary, rankGold/Silver/Bronze, …) and reference them. Commit: `e15bb7d fix(ui): fold hardcoded colors into semantic tokens`（`onPrimary/white/danger/dangerStrong/rankGold/Silver/Bronze` 8 个 token，18 文件，tsx 内硬编码 hex 归零）。

### [P2] No adaptivity across screen sizes / orientation

- **Location:** whole app — no `useWindowDimensions`, no size-class logic, fixed pixel layouts
- **Why it matters:** Landscape and larger phones render the same single-column layout; content may clip or waste space in landscape. No hinge/multi-window awareness.
- **Fix (todo):** Introduce `useWindowDimensions()` for landscape/wide layouts; ensure `ScrollView` content can scroll in landscape.
- **未修原因：** 这是一次开放式响应式设计（断点/横屏重排/折叠感知），需渲染验证且涉及视觉方向拍板；本环境无模拟器/仿真器、无 web 渲染（报告自身已注明），盲改布局无法验证、风险高于收益。留待设备验收 + 设计 pass，随 `impeccable adapt` 另行立项。

### [P3] "‹" text back button has a poor accessible name

- **Location:** `apps/mobile-app/src/app/stats/[gameId].tsx:41` (back is a `<Text>‹</Text>` inside a Pressable)
- **Why it matters:** A screen reader announces a lone "‹" character instead of "返回". (The other back buttons are "← 返回" text, which is acceptable.)
- **Fix (done):** Use an Ionicons chevron-back with `accessibilityLabel="返回"`, matching the other screens. Commit: `400d211 fix(ui): label stats back button for screen readers`（`‹` Text 换 chevron-back + label，删 backText 死样式）。

## Positive findings (mobile)

- `FlatList` + `onEndReached` pagination + `RefreshControl` used consistently — lists are virtualized, not `map()`-ed.
- Loading / error / empty states are first-class on every screen, with a retry affordance and `accessibilityRole="alert"` on errors.
- `KeyboardAvoidingView` wraps every input screen (login, compose, messages, post comment).
- The token palette has correct contrast; no `Animated`/`LayoutAnimation` in the app, so there is no reduce-motion conflict.
- `AuthContext` centralizes session restore (SecureStore) and 401 redirect.

## Notes

- **Rendered verification skipped (why):** no emulator/simulator and no web export possible (native-only deps). Findings are source-verified; contrast ratios were computed from the literal hex values in `constants/colors.ts` and inline styles. This is an honest scope limit, not a failure.
- **DESIGN.md absence:** no `DESIGN.md`/`PRODUCT.md` in the repo. The mobile palette is documented in a one-line comment at `src/constants/colors.ts:1` ("M1 接真实设计体系时迁入 packages/ui-kit"), which is the only written design intent.
- **Unreachable states:** no logged-in flows could be exercised (no auth in a non-rendering environment), so the DM thread view, post composer with image upload, and notification tap deep-link paths were audited by source only. The `typecheck`/`lint` gates could not be run (toolchain binaries absent).

## Recommended next steps

1. **[P2] `impeccable harden`** — raise shared chip/retry/back styles to 44pt minimum touch targets. **Done** (`664a3b2`).
2. **[P2] `impeccable adapt`** — replace `headerShown:false` + hardcoded `paddingTop` with native headers or `useSafeAreaInsets()`. **Done** (`6a5a771`, insets 方案）。
3. **[P2] `impeccable colorize`** — fold ~30 hard-coded hex values into semantic tokens. **Done** (`e15bb7d`).
4. **[P3] `impeccable polish`** — final pass on the `‹` back button and reduced-motion handling (web). **Done**（`400d211` + `f52ca04`）。

## Summary counts

- **Total issues:** 13 (web 7, mobile 6) — 0 P0, 2 P1, 7 P2, 4 P3
- **Fixed:** 11 — 首次 4（`ed79698` nav 溢出、`05cfe57` 对比度、`49e6518` 网络错误文案、`a759ffa` mobile 图标标签）+ 整改跟进 7（web `30c181c` 游戏详情错误态、`f52ca04` reduced-motion、`fb28fc9`+`2b00365` 封面回退；mobile `664a3b2` 44pt 触控、`6a5a771` 安全区 insets、`e15bb7d` 语义色 token、`400d211` ‹ 返回标签）。
- **Deferred (未修原因见各条目):** 2 — web 字体/侧边条强调（视觉方向，留 design pass）、mobile 横屏/宽屏适配（响应式设计，需设备渲染验证）。
- **整改验证（跟进 pass，2026-11-01）:** web `tsc`/`eslint`/`vite build` 全绿；mobile `tsc`/`eslint`/`expo export --platform android` 全绿（3.1MB hbc）；版本号未动、未打 tag。
