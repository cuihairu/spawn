import { Platform } from 'react-native';

// 服务地址：Android 模拟器用 10.0.2.2 访问宿主机回环（Expo 开发惯例），
// iOS 模拟器/本机调试用 localhost；上生产前换成网关域名（docs/mobile_plan.md 风险表）。
const host = Platform.OS === 'android' ? '10.0.2.2' : 'localhost';
export const USER_SERVICE_URL = `http://${host}:8888`;
export const GAME_SERVICE_URL = `http://${host}:8890`;
export const CONTENT_SERVICE_URL = `http://${host}:8891`;
export const COMMUNITY_SERVICE_URL = `http://${host}:8892`;

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

export interface Comment {
  id: number;
  target_type: string;
  target_id: number;
  user_id: number;
  user_name: string;
  content: string;
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

// 发表评论（需 Bearer；未登录/过期走 401 统一处理）。回复层级（parent_id）首期不启用。
export async function createComment(targetId: number, content: string, token: string): Promise<Comment> {
  return requestEnvelope<Comment>(`${CONTENT_SERVICE_URL}/api/v1/comments`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ target_type: 'guide', target_id: targetId, content }),
  });
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

export interface CreatePostPayload {
  topicId: number;
  title: string;
  content: string;
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
