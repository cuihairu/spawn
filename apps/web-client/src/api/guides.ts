// 攻略域：列表/详情/创建/发布/点赞/收藏（content-service，经网关反代）。
import { apiGatewayUrl, contentServiceUrl, request } from './http'

export interface Guide {
  id: number
  gameId: string
  title: string
  content: string
  format: 'text' | 'markdown'
  authorId: number
  authorName?: string
  status: 'draft' | 'published'
  tags: string[]
  viewCount: number
  likeCount: number
  commentCount: number
  createdAt: string
  updatedAt: string
  publishedAt?: string
}

export interface CreateGuideParams {
  gameId: string
  title: string
  content: string
  format?: 'text' | 'markdown'
  tags?: string[]
}

// ========== 攻略相关 API ==========

type ContentGuideDto = {
  id: number
  game_id: string
  game_title?: string
  title: string
  content: string
  format?: string
  summary?: string
  cover_image?: string
  author_id: number
  author_name?: string
  tags?: string[]
  views?: number
  likes?: number
  is_published?: boolean
  created_at: string
  updated_at: string
}

type ListGuidesResponseDto = {
  code: number
  message: string
  data: ContentGuideDto[]
  total: number
  page: number
}

type GuideResponseDto = {
  code: number
  message: string
  data: ContentGuideDto
}

type PublishGuideResponseDto = { code: number; message: string }
type LikeGuideResponseDto = { code: number; message: string; likes: number }
type FavoriteGuideResponseDto = {
  code: number
  message: string
  favorited: boolean
  count: number
}

function mapContentGuide(dto: ContentGuideDto): Guide {
  const isPublished = Boolean(dto.is_published)
  return {
    id: dto.id,
    gameId: dto.game_id,
    title: dto.title,
    content: dto.content,
    format: dto.format === 'markdown' ? 'markdown' : 'text',
    authorId: dto.author_id,
    authorName: dto.author_name,
    status: isPublished ? 'published' : 'draft',
    tags: dto.tags ?? [],
    viewCount: dto.views ?? 0,
    likeCount: dto.likes ?? 0,
    commentCount: 0,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

export async function fetchGuides(params?: {
  gameId?: string
  authorId?: number
  keyword?: string
  tag?: string
  sort?: string
  limit?: number
  offset?: number
  token?: string
}): Promise<Guide[]> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.gameId) searchParams.set('game_id', params.gameId)
  if (params?.authorId) searchParams.set('author_id', String(params.authorId))
  if (params?.keyword) searchParams.set('keyword', params.keyword)
  if (params?.tag) searchParams.set('tag', params.tag)
  if (params?.sort) searchParams.set('sort', params.sort)
  const pageSize = params?.limit && params.limit > 0 ? params.limit : 20
  const offset = params?.offset && params.offset > 0 ? params.offset : 0
  const page = Math.floor(offset / pageSize) + 1
  searchParams.set('page', String(page))
  searchParams.set('page_size', String(pageSize))

  const headers: Record<string, string> = {}
  if (params?.token) {
    headers.Authorization = `Bearer ${params.token}`
  }

  const response = await request<ListGuidesResponseDto>(baseUrl, `/api/v1/guides?${searchParams.toString()}`, {
    headers,
  })

  return (response.data ?? []).map(mapContentGuide)
}

export async function fetchGuideById(id: number, token?: string): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await request<GuideResponseDto>(baseUrl, `/api/v1/guides/${id}`, { headers })
  if (!response.data) {
    throw new Error('攻略不存在')
  }
  return mapContentGuide(response.data)
}

export async function createGuide(
  params: CreateGuideParams,
  token: string,
): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const response = await request<GuideResponseDto>(baseUrl, '/api/v1/guides', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      game_id: params.gameId,
      title: params.title,
      content: params.content,
      format: params.format ?? 'text',
      tags: params.tags ?? [],
    }),
  })
  if (!response.data) {
    throw new Error('创建攻略失败')
  }
  return mapContentGuide(response.data)
}

export async function updateGuide(
  id: number,
  params: Partial<CreateGuideParams>,
  token: string,
): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const body: Record<string, unknown> = {}
  if (params.title) body.title = params.title
  if (params.content) body.content = params.content
  if (params.format) body.format = params.format
  if (params.tags) body.tags = params.tags

  const response = await request<GuideResponseDto>(baseUrl, `/api/v1/guides/${id}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  })
  if (!response.data) {
    throw new Error('更新攻略失败')
  }
  return mapContentGuide(response.data)
}

export async function publishGuide(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<PublishGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/publish`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function likeGuide(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<LikeGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

// ========== 攻略收藏相关 API ==========

export async function favoriteGuide(
  id: number,
  token: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  if (payload.code !== 200) throw new Error(payload.message || '收藏失败')
  return { favorited: payload.favorited, count: payload.count }
}

export async function unfavoriteGuide(
  id: number,
  token: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  if (payload.code !== 200) throw new Error(payload.message || '取消收藏失败')
  return { favorited: payload.favorited, count: payload.count }
}

// 收藏状态：匿名只拿收藏总数，带 token 才含个人 favorited
export async function fetchFavoriteStatus(
  id: number,
  token?: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    headers,
  })
  if (payload.code !== 200) throw new Error(payload.message || '查询收藏状态失败')
  return { favorited: payload.favorited, count: payload.count }
}

export async function fetchMyFavorites(token: string, limit = 50): Promise<Guide[]> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const pageSize = limit > 0 ? limit : 50
  const response = await request<ListGuidesResponseDto>(
    baseUrl,
    `/api/v1/guides/favorites?page=1&page_size=${pageSize}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  )
  if (response.code !== 200) {
    throw new Error(response.message || '收藏列表加载失败')
  }
  return (response.data ?? []).map(mapContentGuide)
}
