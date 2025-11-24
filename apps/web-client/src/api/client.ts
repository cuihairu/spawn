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
