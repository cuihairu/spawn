export interface UserInfo {
  id: number
  username: string
  email: string
  nickname: string
}

export interface LoginResult {
  token: string
  userInfo: UserInfo
}

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

export interface Guide {
  id: number
  gameId: string
  title: string
  content: string
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

export interface Comment {
  id: number
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  content: string
  authorId: number
  authorName?: string
  parentId?: number
  likeCount: number
  createdAt: string
  updatedAt: string
  replies?: Comment[]
}

export interface CreateGuideParams {
  gameId: string
  title: string
  content: string
  tags?: string[]
}

export interface CreateCommentParams {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  content: string
  parentId?: number
}

type GameDto = {
  id: string
  title: string
  description?: string
  cover_image?: string
  coverImage?: string
  genres?: string[]
  platforms?: string[]
  score?: number
  tags?: string[]
}

interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

const apiGatewayUrl = import.meta.env.VITE_API_GATEWAY_URL as string | undefined
const userServiceUrl =
  (import.meta.env.VITE_USER_SERVICE_URL as string | undefined) ??
  'http://localhost:8888'
const gameServiceUrl =
  (import.meta.env.VITE_GAME_SERVICE_URL as string | undefined) ??
  'http://localhost:8890'
const contentServiceUrl =
  (import.meta.env.VITE_CONTENT_SERVICE_URL as string | undefined) ??
  'http://localhost:8891'

type LoginResponseDto = {
  token: string
  user_info: UserInfo
}

async function request<T>(
  baseUrl: string,
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(new URL(path, baseUrl), {
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
    ...options,
  })

  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = (await response.json()) as { message?: string }
      detail = payload.message ?? detail
    } catch {
      // ignore
    }
    throw new Error(detail || `请求失败(${response.status})`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

export async function login(username: string, password: string): Promise<LoginResult> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<LoginResponseDto>(baseUrl, '/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  return {
    token: payload.token,
    userInfo: payload.user_info,
  }
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

// ========== 攻略相关 API ==========

export async function fetchGuides(params?: {
  gameId?: string
  status?: 'draft' | 'published'
  authorId?: number
  limit?: number
  offset?: number
  token?: string
}): Promise<Guide[]> {
  const searchParams = new URLSearchParams()
  if (params?.gameId) searchParams.set('game_id', params.gameId)
  if (params?.status) searchParams.set('status', params.status)
  if (params?.authorId) searchParams.set('author_id', String(params.authorId))
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))

  const headers: Record<string, string> = {}
  if (params?.token) {
    headers.Authorization = `Bearer ${params.token}`
  }

  const response = await request<ApiResponse<{ guides: Guide[] }>>(
    contentServiceUrl,
    `/api/v1/guides?${searchParams.toString()}`,
    { headers },
  )
  return response.data?.guides ?? []
}

export async function fetchGuideById(id: number, token?: string): Promise<Guide> {
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await request<ApiResponse<Guide>>(
    contentServiceUrl,
    `/api/v1/guides/${id}`,
    { headers },
  )
  if (!response.data) {
    throw new Error('攻略不存在')
  }
  return response.data
}

export async function createGuide(
  params: CreateGuideParams,
  token: string,
): Promise<Guide> {
  const response = await request<ApiResponse<Guide>>(contentServiceUrl, '/api/v1/guides', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      game_id: params.gameId,
      title: params.title,
      content: params.content,
      tags: params.tags ?? [],
    }),
  })
  if (!response.data) {
    throw new Error('创建攻略失败')
  }
  return response.data
}

export async function updateGuide(
  id: number,
  params: Partial<CreateGuideParams>,
  token: string,
): Promise<Guide> {
  const body: Record<string, unknown> = {}
  if (params.gameId) body.game_id = params.gameId
  if (params.title) body.title = params.title
  if (params.content) body.content = params.content
  if (params.tags) body.tags = params.tags

  const response = await request<ApiResponse<Guide>>(
    contentServiceUrl,
    `/api/v1/guides/${id}`,
    {
      method: 'PUT',
      headers: {
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    },
  )
  if (!response.data) {
    throw new Error('更新攻略失败')
  }
  return response.data
}

export async function publishGuide(id: number, token: string): Promise<void> {
  await request<ApiResponse<unknown>>(contentServiceUrl, `/api/v1/guides/${id}/publish`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function likeGuide(id: number, token: string): Promise<void> {
  await request<ApiResponse<unknown>>(contentServiceUrl, `/api/v1/guides/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

// ========== 评论相关 API ==========

export async function fetchComments(params: {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  limit?: number
  offset?: number
}): Promise<Comment[]> {
  const searchParams = new URLSearchParams({
    target_type: params.targetType,
    target_id: String(params.targetId),
  })
  if (params.limit) searchParams.set('limit', String(params.limit))
  if (params.offset) searchParams.set('offset', String(params.offset))

  const response = await request<ApiResponse<{ comments: Comment[] }>>(
    contentServiceUrl,
    `/api/v1/comments?${searchParams.toString()}`,
  )
  return response.data?.comments ?? []
}

export async function createComment(
  params: CreateCommentParams,
  token: string,
): Promise<Comment> {
  const response = await request<ApiResponse<Comment>>(contentServiceUrl, '/api/v1/comments', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      target_type: params.targetType,
      target_id: params.targetId,
      content: params.content,
      parent_id: params.parentId,
    }),
  })
  if (!response.data) {
    throw new Error('发表评论失败')
  }
  return response.data
}

export async function deleteComment(id: number, token: string): Promise<void> {
  await request<ApiResponse<unknown>>(contentServiceUrl, `/api/v1/comments/${id}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function likeComment(id: number, token: string): Promise<void> {
  await request<ApiResponse<unknown>>(contentServiceUrl, `/api/v1/comments/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}
