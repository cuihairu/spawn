// 设计拆解域（community-service，经网关反代）：策划视角游戏拆解库 + 品类聚合
// + 拆解讨论区。后端契约 snake_case；读端点公开，写端点登录/管理员语义由
// 服务端校验（仅作者可编辑、精选仅审核管理员、删评论仅本人）。
import { apiGatewayUrl, communityServiceUrl, request } from './http'

export interface DesignBreakdown {
  id: number
  game_title: string
  category: string
  summary: string
  core_loop: string
  progression: string
  economy: string
  combat: string
  growth_lines: string
  social: string
  monetization: string
  fairness: string
  worth_stealing: string
  pitfalls: string
  sources: string
  author_id: number
  author_name: string
  featured: boolean
  created_at: string
  updated_at: string
}

export interface DesignCategoryRow {
  category: string
  count: number
}

export interface DesignComment {
  id: number
  breakdown_id: number
  author_id: number
  author_name: string
  content: string
  created_at: string
}

type CategoriesResp = { categories: DesignCategoryRow[] }
type BreakdownsResp = { items: DesignBreakdown[]; total: number }
type BreakdownResp = { breakdown: DesignBreakdown }
type CommentsResp = { comments: DesignComment[] }
type CommentResp = { comment: DesignComment }
type CommonResp = { code: number; message: string }

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
] as const

export type DesignSectionKey = (typeof DESIGN_SECTIONS)[number]['key']

export interface DesignDraft {
  game_title: string
  category: string
  summary: string
  core_loop?: string
  progression?: string
  economy?: string
  combat?: string
  growth_lines?: string
  social?: string
  monetization?: string
  fairness?: string
  worth_stealing?: string
  pitfalls?: string
  sources?: string
}

function designBaseUrl(): string {
  return apiGatewayUrl ?? communityServiceUrl
}

// fetchDesignCategories 品类聚合行（白名单顺序，含 0 行品类）。
export async function fetchDesignCategories(): Promise<DesignCategoryRow[]> {
  const payload = await request<CategoriesResp>(designBaseUrl(), '/api/v1/design/categories')
  return payload.categories ?? []
}

// fetchDesignBreakdowns 拆解列表（品类/精选筛选 + 分页；条目不含全文分节）。
export async function fetchDesignBreakdowns(params?: {
  category?: string
  featured?: boolean
  limit?: number
  offset?: number
}): Promise<{ items: DesignBreakdown[]; total: number }> {
  const searchParams = new URLSearchParams()
  if (params?.category) searchParams.set('category', params.category)
  if (params?.featured) searchParams.set('featured', 'true')
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))
  const qs = searchParams.toString()
  const payload = await request<BreakdownsResp>(
    designBaseUrl(),
    `/api/v1/design/breakdowns${qs ? `?${qs}` : ''}`,
  )
  return { items: payload.items ?? [], total: payload.total ?? 0 }
}

// fetchDesignBreakdown 拆解详情（全文）。
export async function fetchDesignBreakdown(id: number): Promise<DesignBreakdown> {
  const payload = await request<BreakdownResp>(designBaseUrl(), `/api/v1/design/breakdowns/${id}`)
  return payload.breakdown
}

// fetchDesignComments 拆解讨论区评论（id 升序会话式阅读顺序）。
export async function fetchDesignComments(
  id: number,
  limit = 50,
  offset = 0,
): Promise<DesignComment[]> {
  const payload = await request<CommentsResp>(
    designBaseUrl(),
    `/api/v1/design/breakdowns/${id}/comments?limit=${limit}&offset=${offset}`,
  )
  return payload.comments ?? []
}

// createDesignBreakdown 发布拆解（登录即可）。
export async function createDesignBreakdown(draft: DesignDraft, token: string): Promise<number> {
  const payload = await request<BreakdownResp>(designBaseUrl(), '/api/v1/design/breakdowns', {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(draft),
  })
  return payload.breakdown.id
}

// updateDesignBreakdown 编辑拆解（仅作者，他人编辑后端按不存在处理）。
export async function updateDesignBreakdown(
  id: number,
  draft: DesignDraft,
  token: string,
): Promise<void> {
  await request<BreakdownResp>(designBaseUrl(), `/api/v1/design/breakdowns/${id}`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(draft),
  })
}

// setDesignFeatured 精选运营位（仅审核管理员）。
export async function setDesignFeatured(id: number, featured: boolean, token: string): Promise<void> {
  await request<CommonResp>(designBaseUrl(), `/api/v1/design/breakdowns/${id}/feature`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ featured }),
  })
}

// createDesignComment 讨论区发言（登录）。
export async function createDesignComment(
  id: number,
  content: string,
  token: string,
): Promise<DesignComment> {
  const payload = await request<CommentResp>(designBaseUrl(), `/api/v1/design/breakdowns/${id}/comments`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ content }),
  })
  return payload.comment
}

// deleteDesignComment 删除自己的讨论区评论。
export async function deleteDesignComment(
  id: number,
  commentId: number,
  token: string,
): Promise<void> {
  await request<CommonResp>(designBaseUrl(), `/api/v1/design/breakdowns/${id}/comments/${commentId}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}
