// 请求封装与各服务基地址：网关地址优先（VITE_API_GATEWAY_URL），无网关
// 时按服务直连地址回退。所有域模块共用 request()——网络层失败统一换成
// 可读文案，非 2xx 从响应体提 message，204 返回 undefined。
export interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

export const apiGatewayUrl = import.meta.env.VITE_API_GATEWAY_URL as string | undefined
export const communityServiceUrl =
  (import.meta.env.VITE_COMMUNITY_SERVICE_URL as string | undefined) ??
  'http://localhost:8892'
export const userServiceUrl =
  (import.meta.env.VITE_USER_SERVICE_URL as string | undefined) ??
  'http://localhost:8888'
export const gameServiceUrl =
  (import.meta.env.VITE_GAME_SERVICE_URL as string | undefined) ??
  'http://localhost:8890'
export const contentServiceUrl =
  (import.meta.env.VITE_CONTENT_SERVICE_URL as string | undefined) ??
  'http://localhost:8891'

export async function request<T>(
  baseUrl: string,
  path: string,
  options: RequestInit = {},
): Promise<T> {
  let response: Response
  try {
    response = await fetch(new URL(path, baseUrl), {
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers ?? {}),
      },
      ...options,
    })
  } catch (err) {
    // 网络层失败（DNS/CORS/连接拒绝）会抛 TypeError，浏览器原文 "Failed to fetch"
    // 对中文用户没有意义，统一换成可读文案。
    if (err instanceof TypeError) {
      throw new Error('网络请求失败，请检查网络连接或稍后重试')
    }
    throw err
  }

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

// 网关自有端点（/upload、/home/feed）专用：无网关地址时按本地默认网关回退。
export const gatewayBaseUrl = apiGatewayUrl ?? 'http://localhost:8800'
