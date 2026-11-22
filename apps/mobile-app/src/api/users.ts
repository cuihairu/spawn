// 用户域：登录/注册/资料（经网关自有聚合 handler）。
import { requestEnvelope, USER_SERVICE_URL } from './http';

export interface UserInfo {
  id: number;
  username: string;
  email: string;
  nickname?: string;
}

// 契约见 services/game-catalog/game.api（字段为 snake_case）

export interface RegisterPayload {
  username: string;
  email: string;
  password: string;
  nickname?: string;
}

// ========== 用户服务（:8888） ==========

export interface LoginResult {
  token: string;
  user: UserInfo;
}

// 登录与注册同款业务信封：code 200 时 data 内含 token + user_info，
// 失败走 httperr → 真 401/400（request 统一抛 ApiError）。
export async function login(username: string, password: string): Promise<LoginResult> {
  const data = await requestEnvelope<{ token: string; user_info: UserInfo }>(
    `${USER_SERVICE_URL}/auth/login`,
    {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    },
  );
  return { token: data.token, user: data.user_info };
}

export async function register(payload: RegisterPayload): Promise<number> {
  const data = await requestEnvelope<{ user_id: number }>(`${USER_SERVICE_URL}/auth/register`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
  return data.user_id;
}

export async function fetchCurrentUser(userId: number, token: string): Promise<UserInfo> {
  return requestEnvelope<UserInfo>(`${USER_SERVICE_URL}/users/${userId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

// 公开用户资料：GET /users/:id 免鉴权（邮箱字段取到也不展示）
export async function fetchPublicUser(userId: number): Promise<UserInfo> {
  return requestEnvelope<UserInfo>(`${USER_SERVICE_URL}/users/${userId}`);
}
