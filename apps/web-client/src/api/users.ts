// 用户域：登录/注册/公开资料（user-service，经网关反代）。
import { apiGatewayUrl, request, userServiceUrl } from './http'
import type { ApiResponse } from './http'

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

type LoginResponseDto = {
  code: number
  message: string
  data?: { token?: string; user_info?: UserInfo }
}

export async function login(username: string, password: string): Promise<LoginResult> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<LoginResponseDto>(baseUrl, '/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  if (payload.code !== 200 || !payload.data?.token || !payload.data.user_info) {
    throw new Error(payload.message || '登录失败')
  }
  return {
    token: payload.data.token,
    userInfo: payload.data.user_info,
  }
}

type RegisterResponseDto = {
  code: number
  message: string
  data?: { user_id?: number }
}

// GP-403 渠道归因：落地页 ?channel&click_id 的 click_id 缓存键（App 挂载时
// 写入，注册成功后清除；归因窗口由 game-catalog 点击记录判定）。
export const PROMO_CLICK_STORAGE_KEY = 'tappi.promo_click_id'

// 注册新账号（POST /auth/register，信封响应）：成功返回用户 id。
// 后端无邮箱验证环节，注册即建号；随后由调用方自动登录。
// GP-403 渠道归因：clickId 来自落地页 ?channel&click_id（缓存于 localStorage），
// 注册即按该 click_id 归因渠道（user-service 同步解析 game-catalog 点击）。
export async function register(params: {
  username: string
  email: string
  password: string
  clickId?: string
}): Promise<number> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const body: Record<string, string> = { username: params.username, email: params.email, password: params.password }
  if (params.clickId) body.click_id = params.clickId
  const payload = await request<RegisterResponseDto>(baseUrl, '/auth/register', {
    method: 'POST',
    body: JSON.stringify(body),
  })
  if (payload.code !== 200 || !payload.data?.user_id) {
    throw new Error(payload.message || '注册失败')
  }
  return payload.data.user_id
}

export interface PublicUserInfo {
  id: number
  username: string
  nickname: string
}

// 公开用户资料（GET /users/:id 免鉴权；邮箱字段不取不展示）
export async function fetchUserInfo(userId: number): Promise<PublicUserInfo> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<ApiResponse<{ id: number; username: string; nickname?: string }>>(
    baseUrl,
    `/users/${userId}`,
  )
  if (!payload.data || payload.code !== 200) {
    throw new Error(payload.message || '用户不存在')
  }
  return {
    id: payload.data.id,
    username: payload.data.username,
    nickname: payload.data.nickname || payload.data.username,
  }
}
