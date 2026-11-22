import { request, gatewayBase } from './http';

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
