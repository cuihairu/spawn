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

// 游戏详情（game-catalog GET /games/:id，返回 { game } 包裹）
export async function fetchGameById(id: string): Promise<GameDetail> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ game: GameDto }>(baseUrl, `/games/${encodeURIComponent(id)}`)
  return mapGameDetail(payload.game)
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
