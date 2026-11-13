// 战绩面板域（data-panel，经网关反代）。
// ========== 战绩面板相关 API（data-panel，经网关反代） ==========
import { gatewayBaseUrl, request } from './http'

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
