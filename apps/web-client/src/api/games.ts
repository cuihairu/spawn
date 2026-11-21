// 游戏域：精选/推荐/检索/详情（game-catalog，经网关反代）。
import { apiGatewayUrl, gameServiceUrl, request, userServiceUrl } from './http'
import type { ApiResponse } from './http'

export interface GameSummary {
  id: string
  title: string
  description?: string
  coverImage?: string
  genres: string[]
  platforms: string[]
  score?: number
  tags?: string[]
}

export type GameDto = {
  id: string
  title: string
  description?: string
  cover_image?: string
  coverImage?: string
  genres?: string[]
  platforms?: string[]
  score?: number
  tags?: string[]
  release_date?: string
  developer?: string
  publisher?: string
  trending_score?: number
  screenshots?: string[]
  video_url?: string
  changelog?: string
  developer_website?: string
  store_link?: string
  download_url?: string
  play_url?: string
}

// 游戏详情在列表字段之外补充发行信息、资料与入口（game-catalog GET /games/:id）
export interface GameDetail extends GameSummary {
  releaseDate?: string
  developer?: string
  publisher?: string
  trendingScore?: number
  screenshots?: string[]
  videoUrl?: string
  changelog?: string
  developerWebsite?: string
  storeLink?: string
  downloadUrl?: string
  playUrl?: string
  // GP-401 玩家评分聚合（与运营 score 编辑分分离展示）
  rating?: RatingSummary
}

export async function fetchFeaturedGames(limit = 6): Promise<GameSummary[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ games: GameDto[] }>(
    baseUrl,
    `/games/featured?limit=${limit}`,
  )
  return (payload.games ?? []).map(mapGameSummary)
}

export async function fetchRecommendations(
  userId: number,
  token: string,
  limit = 5,
): Promise<GameSummary[]> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const response = await request<ApiResponse<{ recommendations: GameDto[] }>>(
    baseUrl,
    `/users/${userId}/recommendations?limit=${limit}`,
    { headers },
  )
  return (response.data?.recommendations ?? []).map(mapGameSummary)
}

function mapGameSummary(raw: GameDto): GameSummary {
  return {
    ...raw,
    coverImage: raw.coverImage ?? raw.cover_image,
    genres: raw.genres ?? [],
    platforms: raw.platforms ?? [],
    tags: raw.tags ?? [],
  }
}

// 游戏库列表（game-catalog GET /games，支持关键词/类型过滤与分页）
export async function fetchGames(
  params?: {
    keyword?: string
    genre?: string
    platform?: string
    tag?: string
    sort?: string
    limit?: number
    offset?: number
  },
): Promise<{ games: GameSummary[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.keyword) searchParams.set('keyword', params.keyword)
  if (params?.genre) searchParams.set('genre', params.genre)
  if (params?.platform) searchParams.set('platform', params.platform)
  if (params?.tag) searchParams.set('tag', params.tag)
  if (params?.sort) searchParams.set('sort', params.sort)
  searchParams.set('limit', String(params?.limit && params.limit > 0 ? params.limit : 20))
  searchParams.set('offset', String(params?.offset && params.offset > 0 ? params.offset : 0))

  const payload = await request<{ games: GameDto[]; total: number }>(
    baseUrl,
    `/games?${searchParams.toString()}`,
  )
  return { games: (payload.games ?? []).map(mapGameSummary), total: payload.total ?? 0 }
}

// 运营维护入口：整体覆盖游戏标签（如标记/取消「联运」）。登录用户按现有写接口口径可调，
// 角色/权限细化随运营后台（GP-103）落地。
export async function updateGameTags(
  id: string,
  tags: string[],
  token: string,
): Promise<GameDetail> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ game: GameDto }>(baseUrl, `/games/${encodeURIComponent(id)}/tags`, {
    method: 'PATCH',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ tags }),
  })
  return mapGameDetail(payload.game)
}

// 游戏详情（game-catalog GET /games/:id，返回 { game, rating } 包裹；
// PATCH /tags 响应无 rating，故该字段可选）
export async function fetchGameById(id: string): Promise<GameDetail> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ game: GameDto; rating?: RatingSummary }>(
    baseUrl,
    `/games/${encodeURIComponent(id)}`,
  )
  return { ...mapGameDetail(payload.game), rating: payload.rating }
}

// 详情字段映射（fetchGameById / updateGameTags 共用）
export function mapGameDetail(raw: GameDto): GameDetail {
  return {
    ...mapGameSummary(raw),
    releaseDate: raw.release_date,
    developer: raw.developer,
    publisher: raw.publisher,
    trendingScore: raw.trending_score,
    screenshots: raw.screenshots ?? [],
    videoUrl: raw.video_url,
    changelog: raw.changelog,
    developerWebsite: raw.developer_website,
    storeLink: raw.store_link,
    downloadUrl: raw.download_url,
    playUrl: raw.play_url,
  }
}

// ==================== GP-401 玩家评分/长评/收藏 + GP-306 开服预约 ====================
// 玩家面端点全部经网关反代到 game-catalog；与运营 score 编辑分分离，
// 评分不带正文、长评不带分（独立写入口径）。

export interface RatingSummary {
  average: number
  count: number
}

export interface GameServerView {
  id: number
  gameId: string
  name: string
  region: string
  status: string
  plannedOpenAt: string
}

export interface GameReview {
  id: number
  gameId: string
  userId: number
  rating: number
  content: string
  devReply: string
  devReplyAt: string
  createdAt: string
  updatedAt: string
}

export interface GameReviewList {
  items: GameReview[]
  total: number
}

export interface GameReservation {
  gameId: string
  serverId: number
  serverName: string
  serverStatus: string
  plannedOpenAt: string
  createdAt: string
}

function mapReview(raw: {
  id: number
  game_id: string
  user_id: number
  rating: number
  content: string
  dev_reply: string
  dev_reply_at: string
  created_at: string
  updated_at: string
}): GameReview {
  return {
    id: raw.id,
    gameId: raw.game_id,
    userId: raw.user_id,
    rating: raw.rating,
    content: raw.content,
    devReply: raw.dev_reply,
    devReplyAt: raw.dev_reply_at,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  }
}

// 玩家评分（1-5 整数星，覆盖式；请求字段名是 rating）；响应带最新聚合。
export async function submitGameRating(
  id: string,
  score: number,
  token: string,
): Promise<RatingSummary> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ summary: RatingSummary }>(
    baseUrl,
    `/games/${encodeURIComponent(id)}/rating`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ rating: score }),
    },
  )
  return payload.summary
}

export async function fetchGameReviews(
  id: string,
  limit = 20,
  offset = 0,
): Promise<GameReviewList> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{
    items: Parameters<typeof mapReview>[0][]
    total: number
  }>(baseUrl, `/games/${encodeURIComponent(id)}/reviews?limit=${limit}&offset=${offset}`)
  return { items: (payload.items ?? []).map(mapReview), total: payload.total ?? 0 }
}

// 发长评（10-4000 字；一游戏一评，重复 409 提示改用编辑）。
export async function createGameReview(id: string, content: string, token: string): Promise<GameReview> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return mapReview(
    await request<Parameters<typeof mapReview>[0]>(baseUrl, `/games/${encodeURIComponent(id)}/reviews`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ content }),
    }),
  )
}

// 作者改评（仅本人，覆盖式）。
export async function updateGameReview(
  id: string,
  reviewId: number,
  content: string,
  token: string,
): Promise<GameReview> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return mapReview(
    await request<Parameters<typeof mapReview>[0]>(
      baseUrl,
      `/games/${encodeURIComponent(id)}/reviews/${reviewId}`,
      {
        method: 'PUT',
        headers: { Authorization: `Bearer ${token}` },
        body: JSON.stringify({ content }),
      },
    ),
  )
}

// 举报长评（一人一评一报，重复举报幂等返回既有记录）。
export async function reportGameReview(
  id: string,
  reviewId: number,
  reason: string,
  token: string,
): Promise<{ id: number; created: boolean }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return request(baseUrl, `/games/${encodeURIComponent(id)}/reviews/${reviewId}/report`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ reason }),
  })
}

// 收藏/取消（幂等；created=false 表示此前已收藏）。
export async function addFavorite(id: string, token: string): Promise<{ created: boolean }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ created: boolean }>(
    baseUrl,
    `/games/${encodeURIComponent(id)}/favorite`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  )
  return { created: payload.created }
}

export async function removeFavorite(id: string, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  await request(baseUrl, `/games/${encodeURIComponent(id)}/favorite`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// 我的收藏（上限 100）；详情页用它判断当前游戏是否已收藏。
export async function fetchMyFavoriteGameIds(token: string): Promise<Set<string>> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ items: { game: GameDto }[] }>(baseUrl, '/me/favorites', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return new Set((payload.items ?? []).map((item) => item.game.id))
}

// 公开开服表（closed 区服对玩家隐藏，排期随状态露出）。
export async function fetchGameServers(id: string): Promise<GameServerView[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{
    items: { id: number; game_id: string; name: string; region: string; status: string; planned_open_at: string }[]
  }>(baseUrl, `/games/${encodeURIComponent(id)}/servers`)
  return (payload.items ?? []).map((s) => ({
    id: s.id,
    gameId: s.game_id,
    name: s.name,
    region: s.region,
    status: s.status,
    plannedOpenAt: s.planned_open_at,
  }))
}

// 开服预约（幂等 created 标记；已开服 409）。取消 DELETE 返回 {message}。
export async function reserveServer(
  id: string,
  serverId: number,
  token: string,
): Promise<{ created: boolean }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ created: boolean }>(
    baseUrl,
    `/games/${encodeURIComponent(id)}/servers/${serverId}/reserve`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  )
  return { created: payload.created }
}

export async function cancelReservation(id: string, serverId: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  await request(baseUrl, `/games/${encodeURIComponent(id)}/servers/${serverId}/reserve`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function fetchMyReservations(id: string, token: string): Promise<GameReservation[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{
    items: {
      game_id: string
      server_id: number
      server_name: string
      server_status: string
      planned_open_at: string
      created_at: string
    }[]
  }>(baseUrl, `/games/${encodeURIComponent(id)}/reservations/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return (payload.items ?? []).map((r) => ({
    gameId: r.game_id,
    serverId: r.server_id,
    serverName: r.server_name,
    serverStatus: r.server_status,
    plannedOpenAt: r.planned_open_at,
    createdAt: r.created_at,
  }))
}
