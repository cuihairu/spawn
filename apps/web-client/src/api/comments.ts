// 攻略评论域（content-service）：列表/创建/删除/点赞 + 楼中楼树。
import { apiGatewayUrl, contentServiceUrl, request } from './http'

export interface Comment {
  id: number
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  content: string
  authorId: number
  authorName?: string
  parentId?: number
  likeCount: number
  createdAt: string
  updatedAt: string
  replies?: Comment[]
}

export interface CreateCommentParams {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  content: string
  parentId?: number
  replyToId?: number
}

// ========== 评论相关 API ==========

type ContentCommentDto = {
  id: number
  target_type: 'guide' | 'game' | 'comment' | string
  target_id: number
  user_id: number
  user_name?: string
  content: string
  parent_id?: number
  reply_to_id?: number
  likes?: number
  created_at: string
  updated_at: string
}

type ListCommentsResponseDto = {
  code: number
  message: string
  data: ContentCommentDto[]
  total: number
  page: number
}

type CommentResponseDto = {
  code: number
  message: string
  data: ContentCommentDto
}

type DeleteCommentResponseDto = { code: number; message: string }
type LikeCommentResponseDto = { code: number; message: string; likes: number }

function mapContentComment(dto: ContentCommentDto): Comment {
  return {
    id: dto.id,
    targetType: dto.target_type as Comment['targetType'],
    targetId: dto.target_id,
    content: dto.content,
    authorId: dto.user_id,
    authorName: dto.user_name,
    parentId: dto.parent_id && dto.parent_id > 0 ? dto.parent_id : undefined,
    likeCount: dto.likes ?? 0,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

function buildCommentTree(flat: Comment[]): Comment[] {
  const byId = new Map<number, Comment>()
  for (const c of flat) {
    byId.set(c.id, { ...c, replies: [] })
  }

  const roots: Comment[] = []
  for (const c of byId.values()) {
    const parentId = c.parentId
    if (parentId && byId.has(parentId)) {
      byId.get(parentId)!.replies!.push(c)
      continue
    }
    roots.push(c)
  }

  for (const c of byId.values()) {
    if (c.replies && c.replies.length === 0) {
      delete c.replies
    }
  }

  return roots
}

export async function fetchComments(params: {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  limit?: number
  offset?: number
}): Promise<Comment[]> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const pageSize = params.limit && params.limit > 0 ? params.limit : 20
  const offset = params.offset && params.offset > 0 ? params.offset : 0
  const page = Math.floor(offset / pageSize) + 1

  const searchParams = new URLSearchParams({
    target_type: params.targetType,
    target_id: String(params.targetId),
    page: String(page),
    page_size: String(pageSize),
  })

  const response = await request<ListCommentsResponseDto>(baseUrl, `/api/v1/comments?${searchParams.toString()}`)
  const flat = (response.data ?? []).map(mapContentComment)
  return buildCommentTree(flat)
}

export async function createComment(
  params: CreateCommentParams,
  token: string,
): Promise<Comment> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const response = await request<CommentResponseDto>(baseUrl, '/api/v1/comments', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      target_type: params.targetType,
      target_id: params.targetId,
      content: params.content,
      parent_id: params.parentId,
      reply_to_id: params.replyToId,
    }),
  })
  if (!response.data) {
    throw new Error('发表评论失败')
  }
  return mapContentComment(response.data)
}

export async function deleteComment(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<DeleteCommentResponseDto>(baseUrl, `/api/v1/comments/${id}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function likeComment(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<LikeCommentResponseDto>(baseUrl, `/api/v1/comments/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}
