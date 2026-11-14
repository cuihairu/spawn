// 设计拆解域（community-service，经网关反代）：策划视角游戏拆解库 + 品类聚合
// + 拆解讨论区。契约 snake_case；读端点公开，写端点登录/管理员语义由服务端
// 校验（仅作者可编辑、精选仅审核管理员、删评论仅本人）。
import { request, COMMUNITY_SERVICE_URL } from './http';

export interface DesignBreakdown {
  id: number;
  game_title: string;
  category: string;
  summary: string;
  core_loop: string;
  progression: string;
  economy: string;
  combat: string;
  growth_lines: string;
  social: string;
  monetization: string;
  fairness: string;
  worth_stealing: string;
  pitfalls: string;
  sources: string;
  author_id: number;
  author_name: string;
  featured: boolean;
  created_at: string;
  updated_at: string;
}

export interface DesignCategoryRow {
  category: string;
  count: number;
}

export interface DesignComment {
  id: number;
  breakdown_id: number;
  author_id: number;
  author_name: string;
  content: string;
  created_at: string;
}

// DESIGN_SECTIONS 拆解模板分节（与后端字段一一对应，详情页按此顺序渲染）。
export const DESIGN_SECTIONS = [
  { key: 'core_loop', title: '核心循环' },
  { key: 'progression', title: '进度系统' },
  { key: 'economy', title: '经济系统' },
  { key: 'combat', title: '战斗系统' },
  { key: 'growth_lines', title: '养成线' },
  { key: 'social', title: '社交设计' },
  { key: 'monetization', title: '付费设计' },
  { key: 'fairness', title: '公平性' },
  { key: 'worth_stealing', title: '值得抄' },
  { key: 'pitfalls', title: '踩坑' },
] as const;

export type DesignSectionKey = (typeof DESIGN_SECTIONS)[number]['key'];

export interface DesignDraft {
  game_title: string;
  category: string;
  summary: string;
  core_loop?: string;
  progression?: string;
  economy?: string;
  combat?: string;
  growth_lines?: string;
  social?: string;
  monetization?: string;
  fairness?: string;
  worth_stealing?: string;
  pitfalls?: string;
  sources?: string;
}

export interface DesignBreakdownsQuery {
  category?: string;
  featured?: boolean;
  limit?: number;
  offset?: number;
}

// 品类聚合行（白名单顺序，含 0 行品类）。
export async function fetchDesignCategories(): Promise<DesignCategoryRow[]> {
  const payload = await request<{ categories: DesignCategoryRow[] }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/categories`,
  );
  return payload.categories ?? [];
}

// 拆解列表（品类/精选筛选 + 分页；条目只回摘要字段，全文进详情）。
export async function fetchDesignBreakdowns(
  query: DesignBreakdownsQuery = {},
): Promise<{ items: DesignBreakdown[]; total: number }> {
  const params = new URLSearchParams();
  if (query.category) params.set('category', query.category);
  if (query.featured) params.set('featured', 'true');
  params.set('limit', String(query.limit ?? 20));
  params.set('offset', String(query.offset ?? 0));
  const payload = await request<{ items: DesignBreakdown[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns?${params.toString()}`,
  );
  return { items: payload.items ?? [], total: payload.total ?? 0 };
}

// 拆解详情（全文）。
export async function fetchDesignBreakdown(id: number): Promise<DesignBreakdown> {
  const payload = await request<{ breakdown: DesignBreakdown }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${id}`,
  );
  return payload.breakdown;
}

// 讨论区评论列表（id 升序会话式阅读顺序；公开读）。
export async function fetchDesignComments(
  breakdownId: number,
  limit = 50,
  offset = 0,
): Promise<DesignComment[]> {
  const payload = await request<{ comments: DesignComment[] }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${breakdownId}/comments?limit=${limit}&offset=${offset}`,
  );
  return payload.comments ?? [];
}

// 发布拆解（登录即可）。
export async function createDesignBreakdown(draft: DesignDraft, token: string): Promise<number> {
  const payload = await request<{ breakdown: DesignBreakdown }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify(draft),
    },
  );
  return payload.breakdown.id;
}

// 编辑拆解（仅作者；非作者后端按不存在处理）。
export async function updateDesignBreakdown(
  id: number,
  draft: DesignDraft,
  token: string,
): Promise<void> {
  await request<{ breakdown: DesignBreakdown }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${id}`,
    {
      method: 'PUT',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify(draft),
    },
  );
}

// 精选运营位（仅审核管理员）。
export async function setDesignFeatured(
  id: number,
  featured: boolean,
  token: string,
): Promise<void> {
  await request<{ code: number; message: string }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${id}/feature`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ featured }),
    },
  );
}

// 讨论区发言（登录）。
export async function createDesignComment(
  breakdownId: number,
  content: string,
  token: string,
): Promise<DesignComment> {
  const payload = await request<{ comment: DesignComment }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${breakdownId}/comments`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ content }),
    },
  );
  return payload.comment;
}

// 删除自己的讨论区评论（仅作者本人）。
export async function deleteDesignComment(
  breakdownId: number,
  commentId: number,
  token: string,
): Promise<void> {
  await request<{ code: number; message: string }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/design/breakdowns/${breakdownId}/comments/${commentId}`,
    { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } },
  );
}
