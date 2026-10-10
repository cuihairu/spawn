import { Platform } from 'react-native';

// BFF 统一入口（第一阶段）：mobile-app 全量经 api-gateway（:8800）——登录/注册/资料
// 走自有聚合 handler，社区/内容/游戏走反代（internal/{community,content,users,games}
// 代理路由 + /games/featured 等自有 handler），直连各服务的拓扑对 App 收敛为一个地址。
// Android 模拟器用 10.0.2.2 访问宿主机回环（Expo 开发惯例），iOS 模拟器用 localhost；
// 生产用 EXPO_PUBLIC_API_URL 指向网关域名。
const devHost = Platform.OS === 'android' ? '10.0.2.2' : 'localhost';
const gatewayBase = process.env.EXPO_PUBLIC_API_URL ?? `http://${devHost}:8800`;
export const USER_SERVICE_URL = gatewayBase;
export const GAME_SERVICE_URL = gatewayBase;
export const CONTENT_SERVICE_URL = gatewayBase;
export const COMMUNITY_SERVICE_URL = gatewayBase;

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
async function requestEnvelopeFull<T extends ApiResponseEnvelope<unknown>>(
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
async function requestEnvelope<T>(url: string, init?: RequestInit): Promise<T> {
  const payload = await requestEnvelopeFull<ApiResponseEnvelope<T>>(url, init);
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

// 公开用户资料：GET /users/:id 免鉴权（邮箱字段取到也不展示）
export async function fetchPublicUser(userId: number): Promise<UserInfo> {
  return requestEnvelope<UserInfo>(`${USER_SERVICE_URL}/users/${userId}`);
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

export async function fetchPostById(id: number): Promise<Post> {
  const payload = await request<{ post: Post }>(`${COMMUNITY_SERVICE_URL}/api/v1/posts/${id}`);
  return payload.post;
}

// ========== 站内通知（community 域）：字段 snake 直传，三端点全需 Bearer ==========

export interface AppNotification {
  id: number;
  user_id: number;
  actor_id: number;
  actor_name?: string;
  type: string; // like_post/comment_post/reply_comment/follow_user
  target_id: number;
  content: string;
  is_read: boolean;
  created_at: string;
}

// 我的通知（id 倒序分页；列表页免二次取帖，content 已含冗余摘要）
export async function fetchNotifications(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ notifications: AppNotification[]; total: number }> {
  const payload = await request<{ notifications: AppNotification[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { notifications: payload.notifications ?? [], total: payload.total ?? 0 };
}

// 未读数（社区 Tab 铃铛徽标轮询用）
export async function fetchUnreadNotificationCount(token: string): Promise<number> {
  const payload = await request<{ count: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications/unread-count`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return payload.count ?? 0;
}

// 全部已读（幂等，无未读同样 ok）
export async function markAllNotificationsRead(token: string): Promise<void> {
  await request<{ code: number; message: string }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications/read-all`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
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

// ========== 统一上传入口（api-gateway POST /upload）==========
// 帖子图片先经网关落 uploads 卷，返回 "/uploads/<hash>.<ext>" 相对 URL；
// 业务侧只存 URL，渲染时用 resolveImageUrl 拼网关来源。
export interface UploadAsset {
  uri: string;
  fileName?: string | null;
  mimeType?: string | null;
}

export async function uploadImage(asset: UploadAsset, token: string): Promise<string> {
  const form = new FormData();
  // RN 的 FormData 接受 {uri,name,type} 文件对象（DOM 类型上按 Blob 形状断言）。
  form.append('file', {
    uri: asset.uri,
    name: asset.fileName ?? 'upload.jpg',
    type: asset.mimeType ?? 'image/jpeg',
  } as unknown as Blob);
  const response = await fetch(`${gatewayBase}/upload`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  if (response.status === 401) {
    unauthorizedHandler?.();
    throw new ApiError('登录已过期，请重新登录', 401);
  }
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new ApiError(payload?.error ?? `上传失败（${response.status}）`, response.status);
  }
  const payload = (await response.json()) as { url: string };
  return payload.url;
}

// 帖子图片存的是网关相对路径（/uploads/<hash>.<ext>），渲染前拼上网关来源。
export function resolveImageUrl(url?: string): string | undefined {
  if (!url) return undefined;
  if (/^https?:\/\//.test(url)) return url;
  return `${gatewayBase}${url.startsWith('/') ? '' : '/'}${url}`;
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

// ========== BFF 聚合端点（:8800 自有 handler） ==========

// GET /home/feed：一次请求拉精选游戏 / 热帖 / 话题 / 攻略四个列表，
// 网关侧已做字段裁剪（正文/图片等大字段不下发，正文仅截断出 summary）。
// degraded 列出因上游故障被降级为空列表的分组，UI 可按组做降级提示。
export interface HomeFeed {
  featured_games: Pick<Game, 'id' | 'title' | 'cover_image' | 'score' | 'genres' | 'platforms'>[];
  hot_posts: {
    id: number;
    topic_id: number;
    author_id: number;
    author_name: string;
    title: string;
    summary: string;
    like_count: number;
    comment_count: number;
    created_at: string;
  }[];
  topics: Pick<Topic, 'id' | 'name' | 'post_count' | 'follower_count' | 'is_official'>[];
  guides: Pick<Guide, 'id' | 'game_id' | 'game_title' | 'title' | 'summary' | 'author_name' | 'likes' | 'views' | 'created_at'>[];
  degraded: string[];
}

// limit 为每组条数（网关缺省 5，上限 20）。匿名可用，无需 token。
export async function fetchHomeFeed(limit = 5): Promise<HomeFeed> {
  return request<HomeFeed>(`${gatewayBase}/home/feed?limit=${limit}`);
}

// ========== 战绩面板（data-panel，经网关反代 /api/v1/stats/*，M4 增量）==========
// 契约见 services/data-panel/data-panel.api：字段 snake_case；
// win_rate/kd 为读侧计算的小数（非百分数），空用户返回全零汇总而非 404。

export interface StatsSummary {
  user_id: number;
  total_matches: number;
  total_wins: number;
  win_rate: number;
  total_kills: number;
  total_deaths: number;
  total_assists: number;
  kd: number;
  total_score: number;
  total_rank_points: number;
  game_count: number;
  last_played_at: string;
}

export interface GameStat {
  game_id: string;
  game_title: string;
  matches: number;
  wins: number;
  win_rate: number;
  kills: number;
  deaths: number;
  assists: number;
  kd: number;
  score: number;
  rank_points: number;
  last_played_at: string;
  created_at: string;
  updated_at: string;
}

// GET /api/v1/stats/users/:id/summary：跨游戏汇总（公开只读）
export async function fetchStatsSummary(userId: number): Promise<StatsSummary> {
  const payload = await request<{ summary: StatsSummary }>(
    `${gatewayBase}/api/v1/stats/users/${userId}/summary`,
  );
  return payload.summary;
}

export interface GameStatListResult {
  games: GameStat[];
  total: number;
}

// GET /api/v1/stats/users/:id/games：按场次降序明细，limit/offset 分页（服务端钳制）
export async function fetchUserGameStats(
  userId: number,
  limit = 20,
  offset = 0,
): Promise<GameStatListResult> {
  const params = new URLSearchParams();
  params.set('limit', String(limit));
  params.set('offset', String(offset));
  const payload = await request<{ games: GameStat[]; total: number }>(
    `${gatewayBase}/api/v1/stats/users/${userId}/games?${params.toString()}`,
  );
  return { games: payload.games ?? [], total: payload.total ?? 0 };
}

// GET /api/v1/stats/users/:id/games/:game_id：单游戏明细，未命中 404（stat not found）
export async function fetchUserGameStat(userId: number, gameId: string): Promise<GameStat> {
  const payload = await request<{ stat: GameStat }>(
    `${gatewayBase}/api/v1/stats/users/${userId}/games/${encodeURIComponent(gameId)}`,
  );
  return payload.stat;
}
