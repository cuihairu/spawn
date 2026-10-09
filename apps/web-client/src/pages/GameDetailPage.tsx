import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { fetchGameById, fetchGuides, type GameDetail, type Guide } from '../api/client'
import './games.css'

// 游戏详情（/games/:id）：hero + 简介/类型/平台/标签 + 相关攻略，对等 mobile game/[id]。
const GameDetailPage = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [game, setGame] = useState<GameDetail | null>(null)
  const [guides, setGuides] = useState<Guide[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (gameId: string) => {
    setLoading(true)
    setError(null)
    try {
      const detail = await fetchGameById(gameId)
      setGame(detail)
    } catch (err) {
      setError((err as Error).message || '加载游戏详情失败')
    } finally {
      setLoading(false)
    }
  }, [])

  // 攻略区独立加载，失败静默不阻塞主内容（与 mobile 同款策略）
  const loadGuides = useCallback(async (gameId: string) => {
    try {
      const result = await fetchGuides({ gameId, limit: 5 })
      setGuides(result)
    } catch {
      setGuides([])
    }
  }, [])

  useEffect(() => {
    if (!id) return
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => {
      load(id)
      loadGuides(id)
    })
  }, [id, load, loadGuides])

  if (loading && !game) return <div className="games-page">加载中...</div>

  if (error && !game) {
    return (
      <div className="games-page">
        <div className="error-banner" role="alert">
          {error}
        </div>
        <button type="button" className="secondary-btn" onClick={() => navigate('/games')}>
          返回游戏库
        </button>
      </div>
    )
  }

  if (!game) return null

  return (
    <div className="games-page">
      <div className="games-header">
        <div>
          <h1>{game.title}</h1>
          <p className="secondary-text">
            {game.developer || '未知工作室'}
            {game.releaseDate ? ` · ${game.releaseDate}` : ''}
            {game.publisher ? ` · 发行：${game.publisher}` : ''}
          </p>
        </div>
        <div className="inline-actions">
          <Link to="/games" className="secondary-btn">
            ← 返回游戏库
          </Link>
        </div>
      </div>

      <section className="game-detail-hero">
        <div className="game-detail-score">
          <span className="game-detail-score-value">{game.score ? game.score.toFixed(1) : '—'}</span>
          <span className="game-detail-score-label">评分</span>
        </div>
        {game.coverImage ? (
          <div
            className="game-detail-cover"
            style={{ backgroundImage: `url(${game.coverImage})` }}
            aria-hidden="true"
          />
        ) : null}
      </section>

      {game.description ? (
        <section className="game-detail-section">
          <h2>简介</h2>
          <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{game.description}</p>
        </section>
      ) : null}

      {game.genres.length ? (
        <section className="game-detail-section">
          <h2>类型</h2>
          <div className="game-detail-chips">
            {game.genres.map((genre) => (
              <button key={genre} type="button" className="genre-chip" onClick={() => navigate(`/games?genre=${encodeURIComponent(genre)}`)}>
                {genre}
              </button>
            ))}
          </div>
        </section>
      ) : null}

      {game.platforms.length ? (
        <section className="game-detail-section">
          <h2>平台</h2>
          <div className="game-detail-chips">
            {game.platforms.map((platform) => (
              <span key={platform} className="game-detail-chip game-detail-chip--muted">
                {platform}
              </span>
            ))}
          </div>
        </section>
      ) : null}

      {game.tags?.length ? (
        <section className="game-detail-section">
          <h2>标签</h2>
          <div className="game-detail-chips">
            {game.tags.map((tag) => (
              <span key={tag} className="game-detail-chip game-detail-chip--accent">
                {tag}
              </span>
            ))}
          </div>
        </section>
      ) : null}

      <section className="game-detail-section">
        <div className="game-detail-section-head">
          <h2>攻略</h2>
          <Link
            to={`/guides?gameId=${encodeURIComponent(game.id)}&title=${encodeURIComponent(game.title)}`}
            className="secondary-btn"
          >
            全部攻略 →
          </Link>
        </div>
        {guides.length ? (
          <ul className="game-detail-guides">
            {guides.map((guide) => (
              <li key={guide.id}>
                <Link to={`/guides/${guide.id}`} className="game-detail-guide-row">
                  <span className="game-detail-guide-title">{guide.title}</span>
                  <span className="secondary-text">
                    {guide.authorName || '匿名'} · {guide.viewCount} 次阅读
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="secondary-text">暂无攻略，去社区看看大家的讨论</p>
        )}
      </section>

      <section className="game-detail-section">
        <h2>热度</h2>
        <p className="secondary-text">🔥 trending score {game.trendingScore ?? 0}</p>
      </section>
    </div>
  )
}

export default GameDetailPage
