import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchUserGameStats,
  fetchUserStatsSummary,
  type GameStat,
  type StatsSummary,
} from '../api/client'
import './stats.css'

interface Props {
  userId: number
  nickname?: string
}

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`
}

function formatDate(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString('zh-CN')
}

// 战绩面板（M4 data-panel web 最小面）：顶部 KPI 汇总 + 按游戏明细表，
// 数据仅经网关反代（/api/v1/stats/*）。
const StatsPage = ({ userId, nickname }: Props) => {
  const [summary, setSummary] = useState<StatsSummary | null>(null)
  const [games, setGames] = useState<GameStat[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    const [summaryRes, gamesRes] = await Promise.allSettled([
      fetchUserStatsSummary(userId),
      fetchUserGameStats(userId, 20, 0),
    ])
    if (summaryRes.status === 'fulfilled') setSummary(summaryRes.value)
    else setError('战绩汇总加载失败')
    if (gamesRes.status === 'fulfilled') {
      setGames(gamesRes.value.games)
      setTotal(gamesRes.value.total)
    } else if (summaryRes.status === 'fulfilled') {
      setError('按游戏明细加载失败')
    } else {
      setError('战绩数据加载失败')
    }
    setLoading(false)
  }, [userId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(load)
  }, [load])

  return (
    <div className="stats-page">
      <div className="stats-header">
        <div>
          <h1>战绩面板</h1>
          <p className="secondary-text">
            👤 {nickname || `用户${userId}`}
            {summary?.lastPlayedAt ? ` · 最近游戏 ${formatDate(summary.lastPlayedAt)}` : ''}
          </p>
        </div>
        <button type="button" className="secondary-btn" onClick={load} disabled={loading}>
          {loading ? '刷新中...' : '刷新'}
        </button>
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : null}

      <div className="stat-kpi-grid">
        <div className="stat-kpi">
          <span className="stat-kpi-label">总场次</span>
          <span className="stat-kpi-value">{summary ? summary.totalMatches : '—'}</span>
        </div>
        <div className="stat-kpi">
          <span className="stat-kpi-label">胜场</span>
          <span className="stat-kpi-value">{summary ? summary.totalWins : '—'}</span>
        </div>
        <div className="stat-kpi">
          <span className="stat-kpi-label">胜率</span>
          <span className="stat-kpi-value">
            {summary ? formatPercent(summary.winRate) : '—'}
          </span>
        </div>
        <div className="stat-kpi">
          <span className="stat-kpi-label">K / D</span>
          <span className="stat-kpi-value">
            {summary
              ? `${summary.totalKills} / ${summary.totalDeaths}`
              : '—'}
          </span>
        </div>
        <div className="stat-kpi">
          <span className="stat-kpi-label">KD 比</span>
          <span className="stat-kpi-value">{summary ? summary.kd.toFixed(2) : '—'}</span>
        </div>
        <div className="stat-kpi">
          <span className="stat-kpi-label">游戏数</span>
          <span className="stat-kpi-value">{summary ? summary.gameCount : '—'}</span>
        </div>
      </div>

      <section className="community-card">
        <h2>按游戏明细（{total}）</h2>
        {loading && games.length === 0 ? (
          <div className="secondary-text">加载中...</div>
        ) : games.length === 0 ? (
          <div className="secondary-text">
            暂无战绩记录，去<Link to="/games">游戏库</Link>挑一款开玩
          </div>
        ) : (
          <div className="stat-table-wrap">
            <table className="stat-table">
              <thead>
                <tr>
                  <th>游戏</th>
                  <th>场次</th>
                  <th>胜/率</th>
                  <th>K/D/A</th>
                  <th>KD 比</th>
                  <th>分数</th>
                  <th>段位分</th>
                  <th>最近</th>
                </tr>
              </thead>
              <tbody>
                {games.map((stat) => (
                  <tr key={stat.gameId}>
                    <td>
                      <span className="stat-game-link">{stat.gameTitle || stat.gameId}</span>
                    </td>
                    <td>{stat.matches}</td>
                    <td>
                      {stat.wins} / {formatPercent(stat.winRate)}
                    </td>
                    <td>
                      {stat.kills}/{stat.deaths}/{stat.assists}
                    </td>
                    <td>{stat.kd.toFixed(2)}</td>
                    <td>{stat.score}</td>
                    <td>{stat.rankPoints}</td>
                    <td>{formatDate(stat.lastPlayedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

export default StatsPage
