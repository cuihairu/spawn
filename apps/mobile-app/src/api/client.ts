import { Platform } from 'react-native';

// 服务地址：Android 模拟器用 10.0.2.2 访问宿主机回环（Expo 开发惯例），
// iOS 模拟器/本机调试用 localhost；上生产前换成网关域名（docs/mobile_plan.md 风险表）。
const host = Platform.OS === 'android' ? '10.0.2.2' : 'localhost';
export const USER_SERVICE_URL = `http://${host}:8888`;
export const GAME_SERVICE_URL = `http://${host}:8890`;

export interface UserInfo {
  id: number;
  username: string;
  email: string;
  nickname?: string;
}

// 契约见 services/game-catalog/game.api（字段为 snake_case）
export interface Game {
  id: string;
  title: string;
  description?: string;
  genres?: string[];
  platforms?: string[];
  release_date?: string;
  developer?: string;
  publisher?: string;
  tags?: string[];
  score?: number;
  trending_score?: number;
  cover_image?: string;
}

export interface GameListResult {
  games: Game[];
  total: number;
  limit: number;
  offset: number;
}

export interface GamesQuery {
  keyword?: string;
  limit?: number;
  offset?: number;
}

export interface RegisterPayload {
  username: string;
  email: string;
  password: string;
  nickname?: string;
}

// 后端错误统一形态：HTTP 非 2xx，或业务信封 code != 200（register 等业务错误走 HTTP 200 + 信封 code）。
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

// 401 回调：由 AuthContext 注册，API 层收到 401 时清会话并跳登录页（避免 client↔auth 循环依赖）。
let unauthorizedHandler: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler;
}

// 错误体可能是 JSON（信封/中间件）也可能是纯文本（如 /auth/login 失败返回「用户名或密码错误」）。
async function parseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return null;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return { message: text };
  }
}

function envelopeMessage(payload: unknown, fallback: string): string {
  if (payload && typeof payload === 'object' && 'message' in payload) {
    const message = (payload as { message?: unknown }).message;
    if (typeof message === 'string' && message) return message;
  }
  return fallback;
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });

  if (response.status === 401) {
    const payload = await parseBody(response);
    unauthorizedHandler?.();
    throw new ApiError(envelopeMessage(payload, '登录已过期，请重新登录'), 401);
  }

  const payload = await parseBody(response);
  if (!response.ok) {
    throw new ApiError(envelopeMessage(payload, `请求失败(${response.status})`), response.status);
  }
  return payload as T;
}

interface ApiResponseEnvelope<T> {
  code: number;
  message: string;
  data?: T;
}

// 业务信封请求：code != 200 视为失败并抛 ApiError（信封 code 兼作 status）。
async function requestEnvelope<T>(url: string, init?: RequestInit): Promise<T> {
  const payload = await request<ApiResponseEnvelope<T>>(url, init);
  if (payload.code !== 200) {
    throw new ApiError(payload.message || `请求失败(${payload.code})`, payload.code);
  }
  return payload.data as T;
}

// ========== 用户服务（:8888） ==========

interface LoginResponseDto {
  token: string;
  user_info: UserInfo;
}

export interface LoginResult {
  token: string;
  user: UserInfo;
}

export async function login(username: string, password: string): Promise<LoginResult> {
  const payload = await request<LoginResponseDto>(`${USER_SERVICE_URL}/auth/login`, {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  return { token: payload.token, user: payload.user_info };
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

// ========== 游戏目录（:8890） ==========

export async function fetchGames(query: GamesQuery = {}): Promise<GameListResult> {
  const params = new URLSearchParams();
  if (query.keyword) params.set('keyword', query.keyword);
  params.set('limit', String(query.limit ?? 20));
  params.set('offset', String(query.offset ?? 0));
  return request<GameListResult>(`${GAME_SERVICE_URL}/games?${params.toString()}`);
}

// 详情接口返回 { game: Game } 包裹（契约见 services/game-catalog/game.api）
export async function fetchGameById(id: string): Promise<Game> {
  const payload = await request<{ game: Game }>(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}`);
  return payload.game;
}

export async function fetchFeaturedGames(limit = 20): Promise<Game[]> {
  const payload = await request<{ games: Game[] }>(`${GAME_SERVICE_URL}/games/featured?limit=${limit}`);
  return payload.games ?? [];
}
