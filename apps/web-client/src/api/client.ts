export interface UserInfo {
  id: number
  username: string
  email: string
  nickname: string
}

export interface LoginResult {
  token: string
  userInfo: UserInfo
}

export interface GameSummary {
  id: string
  title: string
  description?: string
  coverImage?: string
  genres: string[]
  platforms: string[]
  score?: number
  tags?: string[]
}

export interface Guide {
  id: number
  gameId: string
  title: string
  content: string
  format: 'text' | 'markdown'
  authorId: number
  authorName?: string
  status: 'draft' | 'published'
  tags: string[]
  viewCount: number
  likeCount: number
  commentCount: number
  createdAt: string
  updatedAt: string
  publishedAt?: string
}

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

export interface CreateGuideParams {
  gameId: string
  title: string
  content: string
  format?: 'text' | 'markdown'
  tags?: string[]
}

export interface CreateCommentParams {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  content: string
  parentId?: number
  replyToId?: number
}

type GameDto = {
  id: string
  title: string
  description?: string
  cover_image?: string
  coverImage?: string
  genres?: string[]
  platforms?: string[]
  score?: number
  tags?: string[]
  release_date?: string
  developer?: string
  publisher?: string
  trending_score?: number
  screenshots?: string[]
  video_url?: string
  changelog?: string
  developer_website?: string
  store_link?: string
  download_url?: string
  play_url?: string
}

// 游戏详情在列表字段之外补充发行信息、资料与入口（game-catalog GET /games/:id）
export interface GameDetail extends GameSummary {
  releaseDate?: string
  developer?: string
  publisher?: string
  trendingScore?: number
  screenshots?: string[]
  videoUrl?: string
  changelog?: string
  developerWebsite?: string
  storeLink?: string
  downloadUrl?: string
  playUrl?: string
}

interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

const apiGatewayUrl = import.meta.env.VITE_API_GATEWAY_URL as string | undefined
const communityServiceUrl =
  (import.meta.env.VITE_COMMUNITY_SERVICE_URL as string | undefined) ??
  'http://localhost:8892'
const userServiceUrl =
  (import.meta.env.VITE_USER_SERVICE_URL as string | undefined) ??
  'http://localhost:8888'
const gameServiceUrl =
  (import.meta.env.VITE_GAME_SERVICE_URL as string | undefined) ??
  'http://localhost:8890'
const contentServiceUrl =
  (import.meta.env.VITE_CONTENT_SERVICE_URL as string | undefined) ??
  'http://localhost:8891'

type LoginResponseDto = {
  code: number
  message: string
  data?: { token?: string; user_info?: UserInfo }
}

async function request<T>(
  baseUrl: string,
  path: string,
  options: RequestInit = {},
): Promise<T> {
  let response: Response
  try {
    response = await fetch(new URL(path, baseUrl), {
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers ?? {}),
      },
      ...options,
    })
  } catch (err) {
    // 网络层失败（DNS/CORS/连接拒绝）会抛 TypeError，浏览器原文 "Failed to fetch"
    // 对中文用户没有意义，统一换成可读文案。
    if (err instanceof TypeError) {
      throw new Error('网络请求失败，请检查网络连接或稍后重试')
    }
    throw err
  }

  if (!response.ok) {
    let detail = response.statusText
    try {
      const payload = (await response.json()) as { message?: string }
      detail = payload.message ?? detail
    } catch {
      // ignore
    }
    throw new Error(detail || `请求失败(${response.status})`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

export async function login(username: string, password: string): Promise<LoginResult> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<LoginResponseDto>(baseUrl, '/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  if (payload.code !== 200 || !payload.data?.token || !payload.data.user_info) {
    throw new Error(payload.message || '登录失败')
  }
  return {
    token: payload.data.token,
    userInfo: payload.data.user_info,
  }
}

type RegisterResponseDto = {
  code: number
  message: string
  data?: { user_id?: number }
}

// 注册新账号（POST /auth/register，信封响应）：成功返回用户 id。
// 后端无邮箱验证环节，注册即建号；随后由调用方自动登录。
export async function register(params: {
  username: string
  email: string
  password: string
}): Promise<number> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<RegisterResponseDto>(baseUrl, '/auth/register', {
    method: 'POST',
    body: JSON.stringify(params),
  })
  if (payload.code !== 200 || !payload.data?.user_id) {
    throw new Error(payload.message || '注册失败')
  }
  return payload.data.user_id
}

export interface PublicUserInfo {
  id: number
  username: string
  nickname: string
}

// 公开用户资料（GET /users/:id 免鉴权；邮箱字段不取不展示）
export async function fetchUserInfo(userId: number): Promise<PublicUserInfo> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const payload = await request<ApiResponse<{ id: number; username: string; nickname?: string }>>(
    baseUrl,
    `/users/${userId}`,
  )
  if (!payload.data || payload.code !== 200) {
    throw new Error(payload.message || '用户不存在')
  }
  return {
    id: payload.data.id,
    username: payload.data.username,
    nickname: payload.data.nickname || payload.data.username,
  }
}

export async function fetchFeaturedGames(limit = 6): Promise<GameSummary[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ games: GameDto[] }>(
    baseUrl,
    `/games/featured?limit=${limit}`,
  )
  return (payload.games ?? []).map(mapGameSummary)
}

export async function fetchRecommendations(
  userId: number,
  token: string,
  limit = 5,
): Promise<GameSummary[]> {
  const baseUrl = apiGatewayUrl ?? userServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const response = await request<ApiResponse<{ recommendations: GameDto[] }>>(
    baseUrl,
    `/users/${userId}/recommendations?limit=${limit}`,
    { headers },
  )
  return (response.data?.recommendations ?? []).map(mapGameSummary)
}

function mapGameSummary(raw: GameDto): GameSummary {
  return {
    ...raw,
    coverImage: raw.coverImage ?? raw.cover_image,
    genres: raw.genres ?? [],
    platforms: raw.platforms ?? [],
    tags: raw.tags ?? [],
  }
}

// 游戏库列表（game-catalog GET /games，支持关键词/类型过滤与分页）
export async function fetchGames(
  params?: {
    keyword?: string
    genre?: string
    platform?: string
    tag?: string
    sort?: string
    limit?: number
    offset?: number
  },
): Promise<{ games: GameSummary[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.keyword) searchParams.set('keyword', params.keyword)
  if (params?.genre) searchParams.set('genre', params.genre)
  if (params?.platform) searchParams.set('platform', params.platform)
  if (params?.tag) searchParams.set('tag', params.tag)
  if (params?.sort) searchParams.set('sort', params.sort)
  searchParams.set('limit', String(params?.limit && params.limit > 0 ? params.limit : 20))
  searchParams.set('offset', String(params?.offset && params.offset > 0 ? params.offset : 0))

  const payload = await request<{ games: GameDto[]; total: number }>(
    baseUrl,
    `/games?${searchParams.toString()}`,
  )
  return { games: (payload.games ?? []).map(mapGameSummary), total: payload.total ?? 0 }
}

// 运营维护入口：整体覆盖游戏标签（如标记/取消「联运」）。登录用户按现有写接口口径可调，
// 角色/权限细化随运营后台（GP-103）落地。
export async function updateGameTags(
  id: string,
  tags: string[],
  token: string,
): Promise<GameDetail> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ game: GameDto }>(baseUrl, `/games/${encodeURIComponent(id)}/tags`, {
    method: 'PATCH',
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify({ tags }),
  })
  return mapGameDetail(payload.game)
}

// 游戏详情（game-catalog GET /games/:id，返回 { game } 包裹）
export async function fetchGameById(id: string): Promise<GameDetail> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ game: GameDto }>(baseUrl, `/games/${encodeURIComponent(id)}`)
  return mapGameDetail(payload.game)
}

// 详情字段映射（fetchGameById / updateGameTags 共用）
function mapGameDetail(raw: GameDto): GameDetail {
  return {
    ...mapGameSummary(raw),
    releaseDate: raw.release_date,
    developer: raw.developer,
    publisher: raw.publisher,
    trendingScore: raw.trending_score,
    screenshots: raw.screenshots ?? [],
    videoUrl: raw.video_url,
    changelog: raw.changelog,
    developerWebsite: raw.developer_website,
    storeLink: raw.store_link,
    downloadUrl: raw.download_url,
    playUrl: raw.play_url,
  }
}

// ========== 攻略相关 API ==========

type ContentGuideDto = {
  id: number
  game_id: string
  game_title?: string
  title: string
  content: string
  format?: string
  summary?: string
  cover_image?: string
  author_id: number
  author_name?: string
  tags?: string[]
  views?: number
  likes?: number
  is_published?: boolean
  created_at: string
  updated_at: string
}

type ListGuidesResponseDto = {
  code: number
  message: string
  data: ContentGuideDto[]
  total: number
  page: number
}

type GuideResponseDto = {
  code: number
  message: string
  data: ContentGuideDto
}

type PublishGuideResponseDto = { code: number; message: string }
type LikeGuideResponseDto = { code: number; message: string; likes: number }
type FavoriteGuideResponseDto = {
  code: number
  message: string
  favorited: boolean
  count: number
}

function mapContentGuide(dto: ContentGuideDto): Guide {
  const isPublished = Boolean(dto.is_published)
  return {
    id: dto.id,
    gameId: dto.game_id,
    title: dto.title,
    content: dto.content,
    format: dto.format === 'markdown' ? 'markdown' : 'text',
    authorId: dto.author_id,
    authorName: dto.author_name,
    status: isPublished ? 'published' : 'draft',
    tags: dto.tags ?? [],
    viewCount: dto.views ?? 0,
    likeCount: dto.likes ?? 0,
    commentCount: 0,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

export async function fetchGuides(params?: {
  gameId?: string
  authorId?: number
  keyword?: string
  tag?: string
  sort?: string
  limit?: number
  offset?: number
  token?: string
}): Promise<Guide[]> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const searchParams = new URLSearchParams()
  if (params?.gameId) searchParams.set('game_id', params.gameId)
  if (params?.authorId) searchParams.set('author_id', String(params.authorId))
  if (params?.keyword) searchParams.set('keyword', params.keyword)
  if (params?.tag) searchParams.set('tag', params.tag)
  if (params?.sort) searchParams.set('sort', params.sort)
  const pageSize = params?.limit && params.limit > 0 ? params.limit : 20
  const offset = params?.offset && params.offset > 0 ? params.offset : 0
  const page = Math.floor(offset / pageSize) + 1
  searchParams.set('page', String(page))
  searchParams.set('page_size', String(pageSize))

  const headers: Record<string, string> = {}
  if (params?.token) {
    headers.Authorization = `Bearer ${params.token}`
  }

  const response = await request<ListGuidesResponseDto>(baseUrl, `/api/v1/guides?${searchParams.toString()}`, {
    headers,
  })

  return (response.data ?? []).map(mapContentGuide)
}

export async function fetchGuideById(id: number, token?: string): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await request<GuideResponseDto>(baseUrl, `/api/v1/guides/${id}`, { headers })
  if (!response.data) {
    throw new Error('攻略不存在')
  }
  return mapContentGuide(response.data)
}

export async function createGuide(
  params: CreateGuideParams,
  token: string,
): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const response = await request<GuideResponseDto>(baseUrl, '/api/v1/guides', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      game_id: params.gameId,
      title: params.title,
      content: params.content,
      format: params.format ?? 'text',
      tags: params.tags ?? [],
    }),
  })
  if (!response.data) {
    throw new Error('创建攻略失败')
  }
  return mapContentGuide(response.data)
}

export async function updateGuide(
  id: number,
  params: Partial<CreateGuideParams>,
  token: string,
): Promise<Guide> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const body: Record<string, unknown> = {}
  if (params.title) body.title = params.title
  if (params.content) body.content = params.content
  if (params.format) body.format = params.format
  if (params.tags) body.tags = params.tags

  const response = await request<GuideResponseDto>(baseUrl, `/api/v1/guides/${id}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  })
  if (!response.data) {
    throw new Error('更新攻略失败')
  }
  return mapContentGuide(response.data)
}

export async function publishGuide(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<PublishGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/publish`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function likeGuide(id: number, token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  await request<LikeGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

// ========== 攻略收藏相关 API ==========

export async function favoriteGuide(
  id: number,
  token: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  if (payload.code !== 200) throw new Error(payload.message || '收藏失败')
  return { favorited: payload.favorited, count: payload.count }
}

export async function unfavoriteGuide(
  id: number,
  token: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  if (payload.code !== 200) throw new Error(payload.message || '取消收藏失败')
  return { favorited: payload.favorited, count: payload.count }
}

// 收藏状态：匿名只拿收藏总数，带 token 才含个人 favorited
export async function fetchFavoriteStatus(
  id: number,
  token?: string,
): Promise<{ favorited: boolean; count: number }> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const payload = await request<FavoriteGuideResponseDto>(baseUrl, `/api/v1/guides/${id}/favorite`, {
    headers,
  })
  if (payload.code !== 200) throw new Error(payload.message || '查询收藏状态失败')
  return { favorited: payload.favorited, count: payload.count }
}

export async function fetchMyFavorites(token: string, limit = 50): Promise<Guide[]> {
  const baseUrl = apiGatewayUrl ?? contentServiceUrl
  const pageSize = limit > 0 ? limit : 50
  const response = await request<ListGuidesResponseDto>(
    baseUrl,
    `/api/v1/guides/favorites?page=1&page_size=${pageSize}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  )
  if (response.code !== 200) {
    throw new Error(response.message || '收藏列表加载失败')
  }
  return (response.data ?? []).map(mapContentGuide)
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

// ========== 社区相关 API ==========

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
type CommonResp = { code: number; message: string }
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

// ===== 站内通知（community 域）：字段与 PostComment 同款 snake 直传 =====
export interface AppNotification {
  id: number
  user_id: number
  actor_id: number
  actor_name?: string
  type: 'like_post' | 'comment_post' | 'reply_comment' | 'follow_user' | string
  target_id: number
  content: string
  is_read: boolean
  created_at: string
}

type NotificationsResp = { notifications: AppNotification[]; total: number }
type UnreadCountResp = { count: number }

export async function fetchNotifications(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ notifications: AppNotification[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<NotificationsResp>(
    baseUrl,
    `/api/v1/notifications?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return { notifications: payload.notifications ?? [], total: payload.total ?? 0 }
}

export async function fetchUnreadNotificationCount(token: string): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, '/api/v1/notifications/unread-count', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}

export async function markAllNotificationsRead(token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, '/api/v1/notifications/read-all', {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// 单条通知标记已读（幂等；他人/不存在回 404 信封由 request 统一抛错）。
export async function markNotificationRead(token: string, id: number): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/notifications/${id}/read`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// ===== 站内私信（community 域）：snake 直传，与通知同款网关优先 =====
export interface DirectMessage {
  id: number
  sender_id: number
  recipient_id: number
  content: string
  is_read: boolean
  created_at: string
}

export interface DMConversation {
  peer_id: number
  last_message: string
  last_at: string
  unread: number
}

type SendMessageResp = { message: DirectMessage }
type DMThreadResp = { messages: DirectMessage[]; total: number }
type DMConversationsResp = { conversations: DMConversation[] }

export async function sendDirectMessage(
  token: string,
  recipientId: number,
  content: string,
): Promise<DirectMessage> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<SendMessageResp>(baseUrl, '/api/v1/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ recipient_id: recipientId, content }),
  })
  return payload.message
}

export async function fetchDMConversations(token: string): Promise<DMConversation[]> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<DMConversationsResp>(baseUrl, '/api/v1/messages/conversations', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.conversations ?? []
}

export async function fetchDMThread(
  token: string,
  peerId: number,
  limit = 50,
  offset = 0,
): Promise<{ messages: DirectMessage[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<DMThreadResp>(
    baseUrl,
    `/api/v1/messages/thread/${peerId}?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return { messages: payload.messages ?? [], total: payload.total ?? 0 }
}

// 会话置已读；返回剩余未读总数（徽标同步）。
export async function markDMThreadRead(token: string, peerId: number): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, `/api/v1/messages/thread/${peerId}/read`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}

export async function fetchDMUnreadCount(token: string): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, '/api/v1/messages/unread-count', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}

// 网关自有端点（POST /upload、GET /home/feed）专用：无网关地址时按本地默认网关回退。
const gatewayBaseUrl = apiGatewayUrl ?? 'http://localhost:8800'

export async function uploadImage(file: File, token: string): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const response = await fetch(new URL('/upload', gatewayBaseUrl), {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null
    const message = payload?.error ?? `上传失败（${response.status}）`
    throw new Error(message)
  }
  const payload = (await response.json()) as { url: string }
  return payload.url
}

// 帖子图片存的是网关相对路径（/uploads/<hash>.<ext>），渲染前拼上网关来源。
export function resolveImageUrl(url?: string): string | undefined {
  if (!url) return undefined
  if (/^https?:\/\//.test(url)) return url
  return `${gatewayBaseUrl}${url.startsWith('/') ? '' : '/'}${url}`
}

// BFF 聚合端点 GET /home/feed 的裁剪契约：网关侧已按列表展示裁剪字段，
// 正文只留 summary；degraded 列出因上游故障被降级为空列表的分组。
export interface FeedGame {
  id: string
  title: string
  cover_image?: string
  score: number
  genres: string[]
  platforms: string[]
}

export interface FeedPost {
  id: number
  topic_id: number
  author_id: number
  author_name?: string
  title: string
  summary: string
  like_count: number
  comment_count: number
  created_at: string
}

export interface FeedTopic {
  id: number
  name: string
  post_count: number
  follower_count: number
  is_official: boolean
}

export interface FeedGuide {
  id: number
  game_id: string
  game_title: string
  title: string
  summary: string
  author_name?: string
  likes: number
  views: number
  created_at: string
}

export interface HomeFeed {
  featured_games: FeedGame[]
  hot_posts: FeedPost[]
  topics: FeedTopic[]
  guides: FeedGuide[]
  degraded: string[]
}

export async function fetchHomeFeed(limit = 8): Promise<HomeFeed> {
  const payload = await request<HomeFeed>(gatewayBaseUrl, `/home/feed?limit=${limit}`)
  return { ...payload, degraded: payload.degraded ?? [] }
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

// ========== 战绩面板相关 API（data-panel，经网关反代） ==========

// 单款游戏战绩行（data-panel GET /api/v1/stats/users/:id/games/:game_id）
export interface GameStat {
  gameId: string
  gameTitle: string
  matches: number
  wins: number
  winRate: number
  kills: number
  deaths: number
  assists: number
  kd: number
  score: number
  rankPoints: number
  lastPlayedAt: string
  createdAt: string
  updatedAt: string
}

// 跨游戏汇总（data-panel GET /api/v1/stats/users/:id/summary）
export interface StatsSummary {
  userId: number
  totalMatches: number
  totalWins: number
  winRate: number
  totalKills: number
  totalDeaths: number
  totalAssists: number
  kd: number
  totalScore: number
  totalRankPoints: number
  gameCount: number
  lastPlayedAt: string
}

type StatSummaryDto = {
  summary: {
    user_id: number
    total_matches: number
    total_wins: number
    win_rate: number
    total_kills: number
    total_deaths: number
    total_assists: number
    kd: number
    total_score: number
    total_rank_points: number
    game_count: number
    last_played_at: string
  }
}

type GameStatDto = {
  game_id: string
  game_title: string
  matches: number
  wins: number
  win_rate: number
  kills: number
  deaths: number
  assists: number
  kd: number
  score: number
  rank_points: number
  last_played_at: string
  created_at: string
  updated_at: string
}

function mapGameStat(dto: GameStatDto): GameStat {
  return {
    gameId: dto.game_id,
    gameTitle: dto.game_title,
    matches: dto.matches,
    wins: dto.wins,
    winRate: dto.win_rate,
    kills: dto.kills,
    deaths: dto.deaths,
    assists: dto.assists,
    kd: dto.kd,
    score: dto.score,
    rankPoints: dto.rank_points,
    lastPlayedAt: dto.last_played_at,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

// 战绩查询仅走网关（data-panel 无直连地址口径，同 fetchHomeFeed）
export async function fetchUserStatsSummary(userId: number): Promise<StatsSummary> {
  const payload = await request<StatSummaryDto>(
    gatewayBaseUrl,
    `/api/v1/stats/users/${userId}/summary`,
  )
  const raw = payload.summary
  return {
    userId: raw.user_id,
    totalMatches: raw.total_matches,
    totalWins: raw.total_wins,
    winRate: raw.win_rate,
    totalKills: raw.total_kills,
    totalDeaths: raw.total_deaths,
    totalAssists: raw.total_assists,
    kd: raw.kd,
    totalScore: raw.total_score,
    totalRankPoints: raw.total_rank_points,
    gameCount: raw.game_count,
    lastPlayedAt: raw.last_played_at,
  }
}

export async function fetchUserGameStats(
  userId: number,
  limit = 20,
  offset = 0,
): Promise<{ games: GameStat[]; total: number }> {
  const payload = await request<{ games: GameStatDto[]; total: number }>(
    gatewayBaseUrl,
    `/api/v1/stats/users/${userId}/games?limit=${limit}&offset=${offset}`,
  )
  return { games: (payload.games ?? []).map(mapGameStat), total: payload.total ?? 0 }
}

export async function fetchUserGameStat(userId: number, gameId: string): Promise<GameStat> {
  const payload = await request<{ stat: GameStatDto }>(
    gatewayBaseUrl,
    `/api/v1/stats/users/${userId}/games/${encodeURIComponent(gameId)}`,
  )
  return mapGameStat(payload.stat)
}

// ========== 运营后台（GP-103，game-catalog 域，经网关反代） ==========
// game-catalog 返回裸 JSON（非信封），错误经 HTTP 状态码 + {code,message}，
// request() 统一在 !response.ok 时抛 message。

export type PlatformRole = 'platform_admin' | 'operator' | 'partner' | 'player'

export interface MeInfo {
  userId: number
  username: string
  role: PlatformRole
  partnerId: number
}

export interface PartnerInfo {
  id: number
  name: string
  contactEmail: string
  status: 'active' | 'suspended' | string
  createdAt: string
}

export interface GameOwnershipInfo {
  gameId: string
  partnerId: number
  grantedBy: number
  grantedAt: string
  status: string
}

export interface GameReviewInfo {
  id: number
  gameId: string
  versionId: string
  submittedBy: number
  submittedAt: string
  status: string
  reviewerId: number
  reviewReason: string
  reviewedAt: string
}

export interface GameVersionInfo {
  id: string
  gameId: string
  version: string
  changelog: string
  status: string
  reviewReason: string
  createdAt: string
}

export interface AuditLogInfo {
  id: number
  actorId: number
  actorRole: string
  action: string
  resourceType: string
  resourceId: string
  beforeJson: string
  afterJson: string
  ip: string
  createdAt: string
}

// 后台/合作方列表里的游戏：含审核状态字段（详情字段复用 GameDetail）
export interface AdminGame extends GameDetail {
  status: string
  reviewReason: string
}

type MeDto = { user_id: number; username: string; role: PlatformRole; partner_id: number }
type PartnerDto = { id: number; name: string; contact_email?: string; status?: string; created_at?: string }
type OwnershipDto = { game_id: string; partner_id: number; granted_by: number; granted_at: string; status: string }
type ReviewDto = {
  id: number
  game_id: string
  version_id?: string
  submitted_by: number
  submitted_at: string
  status: string
  reviewer_id: number
  review_reason: string
  reviewed_at: string
}
type VersionDto = {
  id: string
  game_id: string
  version: string
  changelog?: string
  status: string
  review_reason?: string
  created_at: string
}
type AuditDto = {
  id: number
  actor_id: number
  actor_role: string
  action: string
  resource_type: string
  resource_id: string
  before_json: string
  after_json: string
  ip: string
  created_at: string
}

function adminHeaders(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` }
}

function mapAdminGame(dto: GameDto & { status?: string; review_reason?: string }): AdminGame {
  const detail = mapGameDetail(dto)
  return { ...detail, status: dto.status ?? 'approved', reviewReason: dto.review_reason ?? '' }
}

// 当前账号平台角色（导航/入口权限用）
export async function fetchMe(token: string): Promise<MeInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<MeDto>(baseUrl, '/me', { headers: adminHeaders(token) })
  return { userId: payload.user_id, username: payload.username, role: payload.role, partnerId: payload.partner_id ?? 0 }
}

export async function grantRole(params: { userId: number; role: string; partnerId?: number }, token: string): Promise<MeInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<MeDto>(baseUrl, '/admin/roles', {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ user_id: params.userId, role: params.role, partner_id: params.partnerId ?? 0 }),
  })
  return { userId: payload.user_id, username: payload.username ?? '', role: payload.role, partnerId: payload.partner_id ?? 0 }
}

export async function createPartner(params: { name: string; contactEmail?: string }, token: string): Promise<PartnerInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<PartnerDto>(baseUrl, '/admin/partners', {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ name: params.name, contact_email: params.contactEmail ?? '' }),
  })
  return mapPartner(payload)
}

export async function fetchPartners(token: string): Promise<PartnerInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ partners: PartnerDto[] }>(baseUrl, '/admin/partners', { headers: adminHeaders(token) })
  return (payload.partners ?? []).map(mapPartner)
}

export async function setPartnerStatus(id: number, status: string, token: string): Promise<PartnerInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<PartnerDto>(baseUrl, `/admin/partners/${id}`, {
    method: 'PATCH',
    headers: adminHeaders(token),
    body: JSON.stringify({ status }),
  })
  return mapPartner(payload)
}

export async function assignOwnership(gameId: string, partnerId: number, token: string): Promise<GameOwnershipInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<OwnershipDto>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/ownership`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ partner_id: partnerId }),
  })
  return mapOwnership(payload)
}

export async function fetchReviews(params: { status?: string; limit?: number; offset?: number }, token: string): Promise<{ reviews: GameReviewInfo[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params.status) searchParams.set('status', params.status)
  searchParams.set('limit', String(params.limit ?? 20))
  searchParams.set('offset', String(params.offset ?? 0))
  const payload = await request<{ reviews: ReviewDto[]; total: number }>(baseUrl, `/admin/reviews?${searchParams.toString()}`, { headers: adminHeaders(token) })
  return { reviews: (payload.reviews ?? []).map(mapReview), total: payload.total ?? 0 }
}

export async function decideReview(id: number, decision: 'approve' | 'reject', reason: string, token: string): Promise<GameReviewInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<ReviewDto>(baseUrl, `/admin/reviews/${id}/decision`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ decision, reason }),
  })
  return mapReview(payload)
}

export async function publishGame(id: string, token: string): Promise<AdminGame> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<GameDto>(baseUrl, `/admin/games/${encodeURIComponent(id)}/publish`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapAdminGame(payload)
}

export async function unpublishGame(id: string, token: string): Promise<AdminGame> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<GameDto>(baseUrl, `/admin/games/${encodeURIComponent(id)}/unpublish`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapAdminGame(payload)
}

export async function fetchGameVersions(gameId: string, token: string): Promise<GameVersionInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ versions: VersionDto[] }>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/versions`, { headers: adminHeaders(token) })
  return (payload.versions ?? []).map(mapVersion)
}

export async function fetchAuditLogs(params: { resourceType?: string; limit?: number; offset?: number }, token: string): Promise<{ logs: AuditLogInfo[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params.resourceType) searchParams.set('resource_type', params.resourceType)
  searchParams.set('limit', String(params.limit ?? 50))
  searchParams.set('offset', String(params.offset ?? 0))
  const payload = await request<{ logs: AuditDto[]; total: number }>(baseUrl, `/admin/audit?${searchParams.toString()}`, { headers: adminHeaders(token) })
  return { logs: (payload.logs ?? []).map(mapAudit), total: payload.total ?? 0 }
}

// ===== 合作方自助（partner 角色） =====

export async function fetchPartnerGames(token: string): Promise<AdminGame[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ games: (GameDto & { status?: string; review_reason?: string })[] }>(baseUrl, '/partner/games', { headers: adminHeaders(token) })
  return (payload.games ?? []).map(mapAdminGame)
}

export async function partnerSubmitReview(gameId: string, versionId: string | undefined, token: string): Promise<GameReviewInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<ReviewDto>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/submit`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ version_id: versionId ?? '' }),
  })
  return mapReview(payload)
}

export async function partnerCreateVersion(gameId: string, params: { version: string; changelog?: string }, token: string): Promise<GameVersionInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<VersionDto>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/versions`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ version: params.version, changelog: params.changelog ?? '' }),
  })
  return mapVersion(payload)
}

export async function fetchPartnerVersions(gameId: string, token: string): Promise<GameVersionInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ versions: VersionDto[] }>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/versions`, { headers: adminHeaders(token) })
  return (payload.versions ?? []).map(mapVersion)
}

function mapPartner(dto: PartnerDto): PartnerInfo {
  return {
    id: dto.id,
    name: dto.name,
    contactEmail: dto.contact_email ?? '',
    status: dto.status ?? 'active',
    createdAt: dto.created_at ?? '',
  }
}

function mapOwnership(dto: OwnershipDto): GameOwnershipInfo {
  return { gameId: dto.game_id, partnerId: dto.partner_id, grantedBy: dto.granted_by, grantedAt: dto.granted_at, status: dto.status }
}

function mapReview(dto: ReviewDto): GameReviewInfo {
  return {
    id: dto.id,
    gameId: dto.game_id,
    versionId: dto.version_id ?? '',
    submittedBy: dto.submitted_by,
    submittedAt: dto.submitted_at,
    status: dto.status,
    reviewerId: dto.reviewer_id ?? 0,
    reviewReason: dto.review_reason ?? '',
    reviewedAt: dto.reviewed_at ?? '',
  }
}

function mapVersion(dto: VersionDto): GameVersionInfo {
  return {
    id: dto.id,
    gameId: dto.game_id,
    version: dto.version,
    changelog: dto.changelog ?? '',
    status: dto.status,
    reviewReason: dto.review_reason ?? '',
    createdAt: dto.created_at,
  }
}

function mapAudit(dto: AuditDto): AuditLogInfo {
  return {
    id: dto.id,
    actorId: dto.actor_id,
    actorRole: dto.actor_role,
    action: dto.action,
    resourceType: dto.resource_type,
    resourceId: dto.resource_id,
    beforeJson: dto.before_json,
    afterJson: dto.after_json,
    ip: dto.ip,
    createdAt: dto.created_at,
  }
}
