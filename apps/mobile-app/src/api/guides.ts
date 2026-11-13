import { requestEnvelope, requestEnvelopeFull, CONTENT_SERVICE_URL } from './http';
import type { ApiResponseEnvelope } from './http';

// ========== 内容服务（:8891）：攻略 + 评论 ==========
// 契约见 services/content-service/content.api：HTTP 200 + {code,message,data} 信封，
// code != 200 为业务失败（如攻略不存在 code 404）；列表带 total/page，分页用 page/page_size。

export interface Guide {
  id: number;
  game_id: string;
  game_title: string;
  title: string;
  content: string;
  // text/markdown（content-service 向后兼容字段，缺省按 text 渲染）
  format?: string;
  summary: string;
  cover_image?: string;
  author_id: number;
  author_name: string;
  tags?: string[];
  views: number;
  likes: number;
  is_published: boolean;
  created_at: string;
  updated_at: string;
}

export interface GuideListResult {
  guides: Guide[];
  total: number;
  page: number;
}

export interface GuidesQuery {
  gameId?: string;
  authorId?: number;
  keyword?: string;
  tag?: string;
  sort?: string;
  page?: number;
  pageSize?: number;
}

interface GuideListEnvelope extends ApiResponseEnvelope<Guide[]> {
  total: number;
  page: number;
}

export async function fetchGuides(
  query: GuidesQuery = {},
  token?: string | null,
): Promise<GuideListResult> {
  const params = new URLSearchParams();
  params.set('page', String(query.page ?? 1));
  params.set('page_size', String(query.pageSize ?? 20));
  if (query.gameId) params.set('game_id', query.gameId);
  // author_id 过滤：我的攻略（个人中心）；带 token 时中间件可选鉴权注入
  // ctx，作者查自己可见草稿（content-service ListGuidesLogic 契约）
  if (query.authorId) params.set('author_id', String(query.authorId));
  if (query.keyword) params.set('keyword', query.keyword);
  if (query.tag) params.set('tag', query.tag);
  if (query.sort) params.set('sort', query.sort);
  const payload = await requestEnvelopeFull<GuideListEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/guides?${params.toString()}`,
    token ? { headers: { Authorization: `Bearer ${token}` } } : undefined,
  );
  return { guides: payload.data ?? [], total: payload.total, page: payload.page };
}

export async function fetchGuideById(id: number, token?: string | null): Promise<Guide> {
  // 可选鉴权：草稿详情仅作者本人（带 Bearer）可读，公开详情匿名可读
  return requestEnvelope<Guide>(`${CONTENT_SERVICE_URL}/api/v1/guides/${id}`, {
    ...(token ? { headers: { Authorization: `Bearer ${token}` } } : {}),
  });
}

// ========== 攻略收藏（content-service 收藏端点） ==========

export interface FavoriteStatus {
  favorited: boolean;
  count: number;
}

interface FavoriteEnvelope extends ApiResponseEnvelope<unknown> {
  favorited: boolean;
  count: number;
}

// 收藏状态：匿名只拿总数（favorited 恒 false），带 token 才含个人状态
export async function fetchFavoriteStatus(
  id: number,
  token?: string | null,
): Promise<FavoriteStatus> {
  const payload = await requestEnvelopeFull<FavoriteEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/guides/${id}/favorite`,
    token ? { headers: { Authorization: `Bearer ${token}` } } : undefined,
  );
  return { favorited: payload.favorited, count: payload.count };
}

export async function favoriteGuide(id: number, token: string): Promise<FavoriteStatus> {
  const payload = await requestEnvelopeFull<FavoriteEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/guides/${id}/favorite`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
  return { favorited: payload.favorited, count: payload.count };
}

export async function unfavoriteGuide(id: number, token: string): Promise<FavoriteStatus> {
  const payload = await requestEnvelopeFull<FavoriteEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/guides/${id}/favorite`,
    { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } },
  );
  return { favorited: payload.favorited, count: payload.count };
}

// 我的收藏（收藏时间倒序，带 Bearer）
export async function fetchMyFavorites(
  token: string,
  pageSize = 20,
): Promise<{ guides: Guide[]; total: number }> {
  const payload = await requestEnvelopeFull<GuideListEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/guides/favorites?page=1&page_size=${pageSize}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { guides: payload.data ?? [], total: payload.total };
}

// ========== 攻略创作（移动端写攻略入口） ==========

export interface CreateGuidePayload {
  gameId: string;
  title: string;
  content: string;
  format?: string;
  summary?: string;
  tags?: string[];
}

// 创建攻略（默认草稿；对齐 content-service CreateGuideRequest 契约）
export async function createGuide(payload: CreateGuidePayload, token: string): Promise<Guide> {
  return requestEnvelope<Guide>(`${CONTENT_SERVICE_URL}/api/v1/guides`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      game_id: payload.gameId,
      title: payload.title,
      content: payload.content,
      ...(payload.format ? { format: payload.format } : {}),
      ...(payload.summary ? { summary: payload.summary } : {}),
      ...(payload.tags?.length ? { tags: payload.tags } : {}),
    }),
  });
}

// 发布攻略（草稿 → 已发布）
export async function publishGuide(id: number, token: string): Promise<void> {
  await requestEnvelopeFull<ApiResponseEnvelope<unknown>>(
    `${CONTENT_SERVICE_URL}/api/v1/guides/${id}/publish`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
    },
  );
}

export interface UpdateGuidePayload {
  title?: string;
  content?: string;
  format?: string;
  summary?: string;
  tags?: string[];
}

// 更新攻略（作者本人；部分更新，后端对空字段跳过不改——与 web 编辑器同契约）
export async function updateGuide(
  id: number,
  payload: UpdateGuidePayload,
  token: string,
): Promise<Guide> {
  return requestEnvelope<Guide>(`${CONTENT_SERVICE_URL}/api/v1/guides/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      ...(payload.title ? { title: payload.title } : {}),
      ...(payload.content ? { content: payload.content } : {}),
      ...(payload.format ? { format: payload.format } : {}),
      ...(payload.summary ? { summary: payload.summary } : {}),
      ...(payload.tags?.length ? { tags: payload.tags } : {}),
    }),
  });
}
