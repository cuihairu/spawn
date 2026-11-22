import { Platform } from 'react-native';

// BFF 统一入口（第一阶段）：mobile-app 全量经 api-gateway（:8800）——登录/注册/资料
// 走自有聚合 handler，社区/内容/游戏走反代（internal/{community,content,users,games}
// 代理路由 + /games/featured 等自有 handler），直连各服务的拓扑对 App 收敛为一个地址。
// Android 模拟器用 10.0.2.2 访问宿主机回环（Expo 开发惯例），iOS 模拟器用 localhost；
// 生产用 EXPO_PUBLIC_API_URL 指向网关域名。
const devHost = Platform.OS === 'android' ? '10.0.2.2' : 'localhost';
export const gatewayBase = process.env.EXPO_PUBLIC_API_URL ?? `http://${devHost}:8800`;
export const USER_SERVICE_URL = gatewayBase;
export const GAME_SERVICE_URL = gatewayBase;
export const CONTENT_SERVICE_URL = gatewayBase;
export const COMMUNITY_SERVICE_URL = gatewayBase;

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

// 401 广播（upload 等绕过 request() 的端点复用同一回调）。
export function notifyUnauthorized(): void {
  unauthorizedHandler?.();
}

// 错误体可能是 JSON（信封/中间件）也可能是纯文本（个别上游直接回文本）。
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

export async function request<T>(url: string, init?: RequestInit): Promise<T> {
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

export interface ApiResponseEnvelope<T> {
  code: number;
  message: string;
  data?: T;
}

// 业务信封请求：code != 200 视为失败并抛 ApiError（信封 code 兼作 status）。
export async function requestEnvelopeFull<T extends ApiResponseEnvelope<unknown>>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const payload = await request<T>(url, init);
  if (payload.code !== 200) {
    throw new ApiError(payload.message || `请求失败(${payload.code})`, payload.code);
  }
  return payload;
}

// 只要 data 字段的信封请求（详情接口）。
export async function requestEnvelope<T>(url: string, init?: RequestInit): Promise<T> {
  const payload = await requestEnvelopeFull<ApiResponseEnvelope<T>>(url, init);
  return payload.data as T;
}
