import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchFeaturedGames,
  fetchRecommendations,
  login,
  type GameSummary,
  type UserInfo,
} from '../api/client'
import GameCard from '../components/GameCard'

interface Props {
  auth: { token: string; user: UserInfo } | null
  onAuthChange: (auth: { token: string; user: UserInfo } | null) => void
}

const HomePage = ({ auth, onAuthChange }: Props) => {
  const [form, setForm] = useState({ username: '', password: '' })
  const [featured, setFeatured] = useState<GameSummary[]>([])
  const [recommendations, setRecommendations] = useState<GameSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [recoLoading, setRecoLoading] = useState(false)
  const [banner, setBanner] = useState<string | null>(null)
  const userId = auth?.user?.id
  const token = auth?.token

  useEffect(() => {
    const loadFeatured = async () => {
      try {
        const games = await fetchFeaturedGames()
        setFeatured(games)
      } catch (err) {
        setBanner((err as Error).message ?? '加载精选游戏失败')
      }
    }
    loadFeatured()
  }, [])

  const handleLogin = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!form.username || !form.password) return
    setLoading(true)
    setBanner(null)

    try {
      const result = await login(form.username.trim(), form.password)
      const authData = { token: result.token, user: result.userInfo }
      onAuthChange(authData)
    } catch (error) {
      setBanner((error as Error).message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  const hydrateRecommendations = useCallback(async (userId: number, token: string) => {
    setRecoLoading(true)
    try {
      const items = await fetchRecommendations(userId, token, 6)
      setRecommendations(items)
    } catch (error) {
      setBanner((error as Error).message || '获取推荐失败')
    } finally {
      setRecoLoading(false)
    }
  }, [])

  const greeting = useMemo(() => {
    if (!auth?.user) return '登录后即可同步个人推荐'
    return `欢迎回来，${auth.user.nickname || auth.user.username}`
  }, [auth])

  useEffect(() => {
    if (!userId || !token) {
      setRecommendations([])
      return
    }

    setRecommendations([])
    hydrateRecommendations(userId, token)
  }, [hydrateRecommendations, token, userId])

  return (
    <div className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">Tappi · 游戏体验服务</p>
          <h1>账号与游戏目录一体化示例</h1>
          <p className="subtitle">
            打通用户服务与游戏目录，登录即可获取实时推荐，并通过 Web 客户端观察整体体验。
          </p>
        </div>
        {auth ? (
          <div className="auth-card">
            <h2>已登录</h2>
            <p className="helper-text">{greeting}</p>
            <div className="quick-links">
              <Link to="/community" className="ghost-button">
                进入社区 →
              </Link>
              <Link to="/guides" className="ghost-button">
                浏览攻略 →
              </Link>
            </div>
          </div>
        ) : (
          <form className="auth-card" onSubmit={handleLogin}>
            <h2>快速登录</h2>
            <label>
              <span>用户名</span>
              <input
                type="text"
                value={form.username}
                onChange={(event) => setForm((prev) => ({ ...prev, username: event.target.value }))}
                placeholder="例如 demo 或 tester"
              />
            </label>
            <label>
              <span>密码</span>
              <input
                type="password"
                value={form.password}
                onChange={(event) => setForm((prev) => ({ ...prev, password: event.target.value }))}
                placeholder="输入任意测试密码"
              />
            </label>
            <button type="submit" disabled={loading}>
              {loading ? '登录中...' : '登录并获取推荐'}
            </button>
            <p className="helper-text">{greeting}</p>
          </form>
        )}
      </header>

      {banner ? (
        <div className="banner" role="alert">
          {banner}
        </div>
      ) : null}

      <section aria-labelledby="featured-title">
        <div className="section-headline">
          <div>
            <p className="eyebrow">Game Catalog Service</p>
            <h2 id="featured-title">当前热度最高的游戏</h2>
          </div>
          <span className="secondary-text">实时由游戏目录服务计算</span>
        </div>
        <div className="game-grid">
          {featured.map((game) => (
            <GameCard key={game.id} game={game} />
          ))}
        </div>
      </section>

      <section aria-labelledby="recommendation-title">
        <div className="section-headline">
          <div>
            <p className="eyebrow">User Service + Game Catalog</p>
            <h2 id="recommendation-title">你的个性化推荐</h2>
          </div>
          {auth ? (
            <button
              type="button"
              className="ghost-button"
              onClick={() => hydrateRecommendations(auth.user.id, auth.token)}
              disabled={recoLoading}
            >
              {recoLoading ? '刷新中...' : '刷新推荐'}
            </button>
          ) : (
            <span className="secondary-text">登录后可体验跨服务推荐链路</span>
          )}
        </div>
        {recommendations.length === 0 ? (
          <div className="empty-state">
            <p>暂无数据</p>
            <small>登录用户后自动回传推荐结果</small>
          </div>
        ) : (
          <div className="game-grid">
            {recommendations.map((game) => (
              <GameCard key={`reco-${game.id}`} game={game} highlight />
            ))}
          </div>
        )}
      </section>

      <section aria-labelledby="guides-preview-title" style={{ marginTop: '3rem' }}>
        <div className="section-headline">
          <div>
            <p className="eyebrow">Content Service</p>
            <h2 id="guides-preview-title">游戏攻略</h2>
          </div>
          <Link to="/guides" className="ghost-button">
            查看更多 →
          </Link>
        </div>
        <p className="secondary-text" style={{ marginTop: '1rem' }}>
          浏览热门攻略，分享游戏心得，与玩家互动交流
        </p>
      </section>
    </div>
  )
}

export default HomePage
