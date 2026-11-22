// 社区域（community-service，经网关反代）：话题/帖子/热帖/搜索、帖子评论、
// 帖子写操作与关注关系/关注流（均需 Bearer）。
import { request, COMMUNITY_SERVICE_URL } from './http';

// ========== 社区服务（:8892）：话题 + 帖子 ==========
// 契约见 services/community/community.api：GET 返回裸 {posts|topics, total}（HTTP 状态即成败）；
// 点赞/分享返回 {code:0,message:"ok"}，计数单调累加（无取消接口）。

export interface Topic {
  id: number;
  name: string;
  description: string;
  icon?: string;
  cover_image?: string;
  post_count: number;
  follower_count: number;
  is_official: boolean;
  created_at: string;
  updated_at: string;
}

export interface Post {
  id: number;
  topic_id: number;
  author_id: number;
  author_name?: string;
  title: string;
  content: string;
  images?: string[];
  type: string;
  tags?: string[];
  view_count: number;
  like_count: number;
  comment_count: number;
  share_count: number;
  is_pinned: boolean;
  is_hot: boolean;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface PostListResult {
  posts: Post[];
  total: number;
}

export async function fetchTopics(limit = 20, offset = 0): Promise<Topic[]> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  const payload = await request<{ topics: Topic[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/topics?${params.toString()}`,
  );
  return payload.topics ?? [];
}

export interface PostsQuery {
  topicId?: number;
  authorId?: number;
  hot?: boolean;
  limit?: number;
  offset?: number;
}

export async function fetchPosts(query: PostsQuery = {}): Promise<PostListResult> {
  const params = new URLSearchParams();
  if (query.topicId) params.set('topic_id', String(query.topicId));
  if (query.authorId) params.set('author_id', String(query.authorId));
  params.set('limit', String(query.limit ?? 20));
  params.set('offset', String(query.offset ?? 0));
  const path = query.hot ? '/posts/hot' : '/posts';
  const payload = await request<{ posts: Post[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1${path}?${params.toString()}`,
  );
  return { posts: payload.posts ?? [], total: payload.total ?? 0 };
}

// 帖子关键词搜索（公开端点）：标题/正文包含匹配，服务端空白 q 返回空结果集。
export async function searchPosts(q: string, limit = 20, offset = 0): Promise<PostListResult> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) });
  const payload = await request<{ posts: Post[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/posts/search?${params.toString()}`,
  );
  return { posts: payload.posts ?? [], total: payload.total ?? 0 };
}

export async function fetchPostById(id: number): Promise<Post> {
  const payload = await request<{ post: Post }>(`${COMMUNITY_SERVICE_URL}/api/v1/posts/${id}`);
  return payload.post;
}

// ========== 帖子评论（community 域，区别于攻略评论的 content 域 createComment） ==========

export interface PostComment {
  id: number;
  post_id: number;
  author_id: number;
  author_name?: string;
  content: string;
  parent_id?: number;
  reply_to_author_name?: string;
  created_at: string;
  updated_at: string;
}

// 帖子评论列表（平铺，回复靠 reply_to_author_name 前缀区分；匿名可读）
export async function fetchPostComments(
  postId: number,
  limit = 50,
  offset = 0,
): Promise<{ comments: PostComment[]; total: number }> {
  const payload = await request<{ comments: PostComment[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/posts/${postId}/comments?limit=${limit}&offset=${offset}`,
  );
  return { comments: payload.comments ?? [], total: payload.total ?? 0 };
}

// 发表评论/回复（parent_id 为被回复评论 id；需 Bearer）
export async function createPostComment(
  postId: number,
  content: string,
  parentId: number | undefined,
  token: string,
): Promise<PostComment> {
  const payload = await request<{ comment: PostComment }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/posts/${postId}/comments`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ content, parent_id: parentId }),
    },
  );
  return payload.comment;
}

// 删除评论（仅作者本人；计数契约单调不回退，调用方不回退展示计数）
export async function deletePostComment(
  postId: number,
  commentId: number,
  token: string,
): Promise<void> {
  await request<{ code: number; message: string }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/posts/${postId}/comments/${commentId}`,
    { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } },
  );
}

export interface CreatePostPayload {
  topicId: number;
  title: string;
  content: string;
  images?: string[];
}

export async function createPost(payload: CreatePostPayload, token: string): Promise<Post> {
  const result = await request<{ post: Post }>(`${COMMUNITY_SERVICE_URL}/api/v1/posts`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({
      topic_id: payload.topicId,
      title: payload.title,
      content: payload.content,
      type: 'discussion',
      images: payload.images ?? [],
    }),
  });
  return result.post;
}

// 点赞/分享：空对象体（LikePostReq/SharePostReq 只有路径参数），成功 {code:0}。
export async function likePost(id: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/posts/${id}/like`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({}),
  });
}

export async function sharePost(id: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/posts/${id}/share`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({}),
  });
}

// 举报帖子（内容审核入口）：reason 可选；同人同帖重复举报后端幂等去重。
export async function reportPost(id: number, token: string, reason?: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/posts/${id}/report`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ reason: reason ?? '' }),
  });
}

// ========== 关注关系 + 关注流（M3，均需 Bearer） ==========

// 关注流：关注话题 ∪ 关注作者的帖子，created_at 倒序，limit/offset 分页；
// 无任何关注 → 空列表 + total 0（空态由界面呈现）。
export async function fetchFollowedPosts(token: string, limit = 20, offset = 0): Promise<PostListResult> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  const payload = await request<{ posts: Post[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/posts/followed?${params.toString()}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { posts: payload.posts ?? [], total: payload.total ?? 0 };
}

// 我的点赞（M3 个人中心）：community GET /users/likes，按点赞时间倒序分页
export async function fetchLikedPosts(token: string, limit = 20, offset = 0): Promise<PostListResult> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  const payload = await request<{ posts: Post[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/users/likes?${params.toString()}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { posts: payload.posts ?? [], total: payload.total ?? 0 };
}

export async function fetchFollowingTopics(token: string): Promise<Topic[]> {
  const payload = await request<{ topics: Topic[] }>(`${COMMUNITY_SERVICE_URL}/api/v1/topics/following`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return payload.topics ?? [];
}

// 已关注用户 id 列表（community 不持有用户资料，名字由帖子 author_name 自带）
export async function fetchFollowingUserIds(token: string): Promise<number[]> {
  const payload = await request<{ user_ids: number[] }>(`${COMMUNITY_SERVICE_URL}/api/v1/users/following`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return payload.user_ids ?? [];
}

export async function followTopic(topicId: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/topics/${topicId}/follow`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({}),
  });
}

export async function unfollowTopic(topicId: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/topics/${topicId}/follow`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function followUser(userId: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/users/${userId}/follow`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({}),
  });
}

export async function unfollowUser(userId: number, token: string): Promise<void> {
  await request(`${COMMUNITY_SERVICE_URL}/api/v1/users/${userId}/follow`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
}
