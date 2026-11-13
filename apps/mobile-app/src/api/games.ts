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

// 详情接口返回 { game: Game } 包裹（契约见 services/game-catalog/game.api）
export async function fetchGameById(id: string): Promise<Game> {
  const payload = await request<{ game: Game }>(`${GAME_SERVICE_URL}/games/${encodeURIComponent(id)}`);
  return payload.game;
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
