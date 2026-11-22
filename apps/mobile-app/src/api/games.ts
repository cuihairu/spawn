// 游戏目录域：列表/详情/最新版本/精选（game-catalog，经网关反代）。
import { ApiError, request, requestEnvelope, GAME_SERVICE_URL } from './http';

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
  screenshots?: string[];
  video_url?: string;
  changelog?: string;
  developer_website?: string;
  store_link?: string;
  download_url?: string;
  play_url?: string;
}

export interface GameListResult {
  games: Game[];
  total: number;
  limit: number;
  offset: number;
}

export interface GamesQuery {
  keyword?: string;
  tag?: string;
  limit?: number;
  offset?: number;
}

// ========== 游戏目录（:8890） ==========

export async function fetchGames(query: GamesQuery = {}): Promise<GameListResult> {
  const params = new URLSearchParams();
  if (query.keyword) params.set('keyword', query.keyword);
  if (query.tag) params.set('tag', query.tag);
  params.set('limit', String(query.limit ?? 20));
  params.set('offset', String(query.offset ?? 0));
  return request<GameListResult>(`${GAME_SERVICE_URL}/games?${params.toString()}`);
}

// 详情接口返回 { game, rating } 包裹（契约见 services/game-catalog/game.api；
// rating 为 GP-401 玩家评分聚合，与运营 score 编辑分分离）
export async function fetchGameById(id: string): Promise<GameDetailPayload> {
  return request<GameDetailPayload>(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}`);
}


// GP-105 游戏当前已发布版本（受控下载元数据）：/games/:id/versions/latest。
// 无已发布版本时后端返回 404 → 返回 null（调用方回退商店链接，F08）。
export interface LatestGameVersion {
  id: string;
  version: string;
  changelog?: string;
  package_name?: string;
  version_code?: number;
  size_bytes?: number;
  hash?: string;
  downloadable?: boolean;
}

export async function fetchLatestGameVersion(gameId: string): Promise<LatestGameVersion | null> {
  try {
    const payload = await requestEnvelope<{ version: LatestGameVersion }>(
      `${GAME_SERVICE_URL}/games/${encodeURIComponent(gameId)}/versions/latest`,
    );
    return payload.version;
  } catch (err) {
    if (err instanceof ApiError && (err.status === 404 || err.status === 400)) {
      return null; // 暂无已发布版本：客户端走商店链接承接
    }
    throw err;
  }
}

export async function fetchFeaturedGames(limit = 20): Promise<Game[]> {
  const payload = await request<{ games: Game[] }>(`${GAME_SERVICE_URL}/games/featured?limit=${limit}`);
  return payload.games ?? [];
}

// ========== GP-401 评分/长评/收藏 + GP-306 开服预约（玩家面，经网关反代） ==========
// 评分与长评为独立写入口径（评分不带正文、长评不带分）；评分请求字段名是
// rating 非 score（契约 RateGameRequest）。

export interface RatingSummary {
  average: number;
  count: number;
}

export interface GameDetailPayload {
  game: Game;
  rating?: RatingSummary;
}

export interface GameServerView {
  id: number;
  gameId: string;
  name: string;
  region: string;
  status: string;
  plannedOpenAt: string;
}

export interface GameReview {
  id: number;
  gameId: string;
  userId: number;
  rating: number;
  content: string;
  devReply: string;
  devReplyAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface GameReviewList {
  items: GameReview[];
  total: number;
}

export interface GameReservation {
  gameId: string;
  serverId: number;
  serverName: string;
  serverStatus: string;
  plannedOpenAt: string;
  createdAt: string;
}

function mapReview(raw: {
  id: number;
  game_id: string;
  user_id: number;
  rating: number;
  content: string;
  dev_reply: string;
  dev_reply_at: string;
  created_at: string;
  updated_at: string;
}): GameReview {
  return {
    id: raw.id,
    gameId: raw.game_id,
    userId: raw.user_id,
    rating: raw.rating,
    content: raw.content,
    devReply: raw.dev_reply,
    devReplyAt: raw.dev_reply_at,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  };
}

export async function submitGameRating(id: string, score: number, token: string): Promise<RatingSummary> {
  const payload = await request<{ summary: RatingSummary }>(
    `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/rating`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ rating: score }),
    },
  );
  return payload.summary;
}

export async function fetchGameReviews(id: string, limit = 20, offset = 0): Promise<GameReviewList> {
  const payload = await request<{ items: Parameters<typeof mapReview>[0][]; total: number }>(
    `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/reviews?limit=${limit}&offset=${offset}`,
  );
  return { items: (payload.items ?? []).map(mapReview), total: payload.total ?? 0 };
}

export async function createGameReview(id: string, content: string, token: string): Promise<GameReview> {
  return mapReview(
    await request<Parameters<typeof mapReview>[0]>(
      `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/reviews`,
      {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: JSON.stringify({ content }),
      },
    ),
  );
}

export async function updateGameReview(
  id: string,
  reviewId: number,
  content: string,
  token: string,
): Promise<GameReview> {
  return mapReview(
    await request<Parameters<typeof mapReview>[0]>(
      `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/reviews/${reviewId}`,
      {
        method: 'PUT',
        headers: { Authorization: `Bearer ${token}` },
        body: JSON.stringify({ content }),
      },
    ),
  );
}

export async function reportGameReview(
  id: string,
  reviewId: number,
  reason: string,
  token: string,
): Promise<{ id: number; created: boolean }> {
  return request<{ id: number; created: boolean }>(
    `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/reviews/${reviewId}/report`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: JSON.stringify({ reason }),
    },
  );
}

export async function addFavorite(id: string, token: string): Promise<{ created: boolean }> {
  const payload = await request<{ created: boolean }>(
    `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/favorite`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
  return { created: payload.created };
}

export async function removeFavorite(id: string, token: string): Promise<void> {
  await request(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/favorite`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function fetchMyFavoriteGameIds(token: string): Promise<Set<string>> {
  const payload = await request<{ items: { game: Game }[] }>(`${GAME_SERVICE_URL}/me/favorites`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return new Set((payload.items ?? []).map((item) => item.game.id));
}

export async function fetchGameServers(id: string): Promise<GameServerView[]> {
  const payload = await request<{
    items: { id: number; game_id: string; name: string; region: string; status: string; planned_open_at: string }[];
  }>(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/servers`);
  return (payload.items ?? []).map((s) => ({
    id: s.id,
    gameId: s.game_id,
    name: s.name,
    region: s.region,
    status: s.status,
    plannedOpenAt: s.planned_open_at,
  }));
}

export async function reserveServer(id: string, serverId: number, token: string): Promise<{ created: boolean }> {
  const payload = await request<{ created: boolean }>(
    `${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/servers/${serverId}/reserve`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
  return { created: payload.created };
}

export async function cancelReservation(id: string, serverId: number, token: string): Promise<void> {
  await request(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/servers/${serverId}/reserve`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function fetchMyReservations(id: string, token: string): Promise<GameReservation[]> {
  const payload = await request<{
    items: {
      game_id: string;
      server_id: number;
      server_name: string;
      server_status: string;
      planned_open_at: string;
      created_at: string;
    }[];
  }>(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}/reservations/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return (payload.items ?? []).map((r) => ({
    gameId: r.game_id,
    serverId: r.server_id,
    serverName: r.server_name,
    serverStatus: r.server_status,
    plannedOpenAt: r.planned_open_at,
    createdAt: r.created_at,
  }));
}
