// 社区域（community-service，经网关反代）：话题/帖子/热帖/搜索/关注，
// 以及帖子评论（区别于攻略评论的 content 域）。
// ========== 社区相关 API ==========
import { apiGatewayUrl, communityServiceUrl, request } from './http'

export interface Topic {
  id: number
  name: string
  description: string
  icon?: string
  coverImage?: string
  postCount: number
  followerCount: number
  isOfficial: boolean
  createdAt: string
  updatedAt: string
}

export interface Post {
  id: number
  topicId: number
  authorId: number
  authorName?: string
  title: string
  content: string
  images?: string[]
  type: string
  tags?: string[]
  viewCount: number
  likeCount: number
  commentCount: number
  shareCount: number
  isPinned: boolean
  isHot: boolean
  status: string
  createdAt: string
  updatedAt: string
}

type TopicsResp = { topics: Topic[]; total: number }
type TopicResp = { topic: Topic }
type PostsResp = { posts: Post[]; total: number }
type PostResp = { post: Post }
export type CommonResp = { code: number; message: string }
type FollowingResp = { topics: Topic[] }

function mapTopic(raw: Topic): Topic {
  return {
    ...raw,
    coverImage: raw.coverImage ?? (raw as { cover_image?: string }).cover_image,
    postCount: raw.postCount ?? (raw as { post_count?: number }).post_count ?? 0,
    followerCount: raw.followerCount ?? (raw as { follower_count?: number }).follower_count ?? 0,
    isOfficial: raw.isOfficial ?? (raw as { is_official?: boolean }).is_official ?? false,
    createdAt: raw.createdAt ?? (raw as { created_at?: string }).created_at ?? '',
    updatedAt: raw.updatedAt ?? (raw as { updated_at?: string }).updated_at ?? '',
  }
}

function mapPost(raw: Post): Post {
  return {
    ...raw,
    topicId: raw.topicId ?? (raw as { topic_id?: number }).topic_id ?? 0,
    authorId: raw.authorId ?? (raw as { author_id?: number }).author_id ?? 0,
    authorName: raw.authorName ?? (raw as { author_name?: string }).author_name,
    viewCount: raw.viewCount ?? (raw as { view_count?: number }).view_count ?? 0,
    likeCount: raw.likeCount ?? (raw as { like_count?: number }).like_count ?? 0,
    commentCount: raw.commentCount ?? (raw as { comment_count?: number }).comment_count ?? 0,
    shareCount: raw.shareCount ?? (raw as { share_count?: number }).share_count ?? 0,
    isPinned: raw.isPinned ?? (raw as { is_pinned?: boolean }).is_pinned ?? false,
    isHot: raw.isHot ?? (raw as { is_hot?: boolean }).is_hot ?? false,
    createdAt: raw.createdAt ?? (raw as { created_at?: string }).created_at ?? '',
    updatedAt: raw.updatedAt ?? (raw as { updated_at?: string }).updated_at ?? '',
  }
}

export async function fetchTopics(params?: {
  keyword?: string
  isOfficial?: boolean
  limit?: number
  offset?: number
}): Promise<{ topics: Topic[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.keyword) searchParams.set('keyword', params.keyword)
  if (typeof params?.isOfficial === 'boolean') {
    searchParams.set('is_official', params.isOfficial ? 'true' : 'false')
  }
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))

  const payload = await request<TopicsResp>(baseUrl, `/api/v1/topics?${searchParams.toString()}`)
  return {
    topics: (payload.topics ?? []).map(mapTopic),
    total: payload.total ?? 0,
  }
}

export async function fetchTopicById(id: number): Promise<Topic> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<TopicResp>(baseUrl, `/api/v1/topics/${id}`)
  return mapTopic(payload.topic)
}

export async function fetchFollowingTopics(token: string): Promise<Topic[]> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<FollowingResp>(baseUrl, `/api/v1/topics/following`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return (payload.topics ?? []).map(mapTopic)
}

export async function createTopic(
  params: { name: string; description?: string; icon?: string; coverImage?: string },
  token: string,
): Promise<Topic> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<TopicResp>(baseUrl, '/api/v1/topics', {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({
      name: params.name,
      description: params.description ?? '',
      icon: params.icon,
      cover_image: params.coverImage,
    }),
  })
  return mapTopic(payload.topic)
}

export async function followTopic(topicId: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/topics/${topicId}/follow`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function unfollowTopic(topicId: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/topics/${topicId}/follow`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function fetchPosts(params?: {
  topicId?: number
  authorId?: number
  type?: string
  status?: string
  isHot?: boolean
  limit?: number
  offset?: number
}): Promise<{ posts: Post[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.topicId) searchParams.set('topic_id', String(params.topicId))
  if (params?.authorId) searchParams.set('author_id', String(params.authorId))
  if (params?.type) searchParams.set('type', params.type)
  if (params?.status) searchParams.set('status', params.status)
  if (typeof params?.isHot === 'boolean') searchParams.set('is_hot', params.isHot ? 'true' : 'false')
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))

  const payload = await request<PostsResp>(baseUrl, `/api/v1/posts?${searchParams.toString()}`)
  return {
    posts: (payload.posts ?? []).map(mapPost),
    total: payload.total ?? 0,
  }
}

export async function fetchHotPosts(limit = 20): Promise<Post[]> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostsResp>(baseUrl, `/api/v1/posts/hot?limit=${limit}`)
  return (payload.posts ?? []).map(mapPost)
}

// 帖子关键词搜索（公开端点）：标题/正文包含匹配，服务端空白 q 返回空结果集。
export async function searchPosts(
  q: string,
  limit = 50,
  offset = 0,
): Promise<{ posts: Post[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const searchParams = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  const payload = await request<PostsResp>(baseUrl, `/api/v1/posts/search?${searchParams.toString()}`)
  return {
    posts: (payload.posts ?? []).map(mapPost),
    total: payload.total ?? 0,
  }
}

export async function fetchPostById(id: number): Promise<Post> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostResp>(baseUrl, `/api/v1/posts/${id}`)
  return mapPost(payload.post)
}

// 关注流（M3 契约，与 mobile 对等）：按关注话题/用户的帖子聚合，最近更新倒序。
export async function fetchFollowedPosts(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ posts: Post[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostsResp>(baseUrl, `/api/v1/posts/followed?limit=${limit}&offset=${offset}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { posts: (payload.posts ?? []).map(mapPost), total: payload.total ?? 0 }
}

// 我的点赞：点赞关系 join 帖子，点赞时间倒序分页。
export async function fetchLikedPosts(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ posts: Post[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostsResp>(baseUrl, `/api/v1/users/likes?limit=${limit}&offset=${offset}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { posts: (payload.posts ?? []).map(mapPost), total: payload.total ?? 0 }
}

// ===== 帖子评论（community 域，区别于攻略评论的 content 域 createComment）=====
export interface PostComment {
  id: number
  post_id: number
  author_id: number
  author_name?: string
  content: string
  parent_id?: number
  reply_to_author_name?: string
  created_at: string
  updated_at: string
}

type PostCommentResp = { comment: PostComment }
type PostCommentsResp = { comments: PostComment[]; total: number }

export async function fetchPostComments(
  postId: number,
  limit = 50,
  offset = 0,
): Promise<{ comments: PostComment[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostCommentsResp>(
    baseUrl,
    `/api/v1/posts/${postId}/comments?limit=${limit}&offset=${offset}`,
  )
  return { comments: payload.comments ?? [], total: payload.total ?? 0 }
}

export async function createPostComment(
  postId: number,
  content: string,
  parentId: number | undefined,
  token: string,
): Promise<PostComment> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostCommentResp>(baseUrl, `/api/v1/posts/${postId}/comments`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ content, parent_id: parentId }),
  })
  return payload.comment
}

export async function deletePostComment(
  postId: number,
  commentId: number,
  token: string,
): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/posts/${postId}/comments/${commentId}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function createPost(
  params: {
    topicId: number
    title: string
    content: string
    tags?: string[]
    type?: string
    images?: string[]
  },
  token: string,
): Promise<Post> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostResp>(baseUrl, '/api/v1/posts', {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({
      topic_id: params.topicId,
      title: params.title,
      content: params.content,
      type: params.type ?? 'discussion',
      tags: params.tags ?? [],
      images: params.images ?? [],
    }),
  })
  return mapPost(payload.post)
}

export async function updatePost(
  id: number,
  params: { title?: string; content?: string; tags?: string[] },
  token: string,
): Promise<Post> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<PostResp>(baseUrl, `/api/v1/posts/${id}`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({
      title: params.title,
      content: params.content,
      tags: params.tags,
    }),
  })
  return mapPost(payload.post)
}

export async function deletePost(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/posts/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function likePost(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/posts/${id}/like`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

export async function sharePost(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/posts/${id}/share`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// 举报帖子（内容审核入口）：reason 可选；同人同帖重复举报后端幂等去重。
export async function reportPost(id: number, token: string, reason?: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/posts/${id}/report`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ reason: reason ?? '' }),
  })
}
