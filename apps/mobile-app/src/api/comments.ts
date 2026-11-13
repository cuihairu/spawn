// 攻略评论域（content-service）：列表/创建/点赞/删除。
import { requestEnvelope, requestEnvelopeFull, CONTENT_SERVICE_URL } from './http';
import type { ApiResponseEnvelope } from './http';

export interface Comment {
  id: number;
  target_type: string;
  target_id: number;
  user_id: number;
  user_name: string;
  content: string;
  parent_id?: number;
  reply_to_id?: number;
  likes: number;
  created_at: string;
  updated_at: string;
}

export interface CommentListResult {
  comments: Comment[];
  total: number;
}

interface CommentListEnvelope extends ApiResponseEnvelope<Comment[]> {
  total: number;
  page: number;
}

export async function fetchComments(targetId: number, page = 1, pageSize = 20): Promise<CommentListResult> {
  const params = new URLSearchParams();
  params.set('target_type', 'guide');
  params.set('target_id', String(targetId));
  params.set('page', String(page));
  params.set('page_size', String(pageSize));
  const payload = await requestEnvelopeFull<CommentListEnvelope>(
    `${CONTENT_SERVICE_URL}/api/v1/comments?${params.toString()}`,
  );
  return { comments: payload.data ?? [], total: payload.total };
}

// 发表评论/回复（需 Bearer；未登录/过期走 401 统一处理）。
// parentId 为被回复评论 id（客户端侧组树用），不传即顶级评论。
export async function createComment(
  targetId: number,
  content: string,
  token: string,
  parentId?: number,
): Promise<Comment> {
  return requestEnvelope<Comment>(`${CONTENT_SERVICE_URL}/api/v1/comments`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({
      target_type: 'guide',
      target_id: targetId,
      content,
      ...(parentId ? { parent_id: parentId } : {}),
    }),
  });
}

// 点赞评论（单调累加，无取消接口；需 Bearer）
export async function likeComment(id: number, token: string): Promise<void> {
  await requestEnvelopeFull<ApiResponseEnvelope<unknown>>(
    `${CONTENT_SERVICE_URL}/api/v1/comments/${id}/like`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
}

// 删除评论（仅作者本人；需 Bearer）
export async function deleteComment(id: number, token: string): Promise<void> {
  await requestEnvelopeFull<ApiResponseEnvelope<unknown>>(
    `${CONTENT_SERVICE_URL}/api/v1/comments/${id}`,
    { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } },
  );
}
