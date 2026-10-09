import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { fetchGames, type GameSummary } from '../api/client'
import GameCard from '../components/GameCard'
import './games.css'

const PAGE_SIZE = 20

// 游戏库（/games）：关键词/类型过滤 + 分页加载，消费 game-catalog GET /games（经网关）。
const GamesPage = () => {
  const [games, setGames] = useState<GameSummary[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [searchText, setSearchText] = useState('')
  const [keyword, setKeyword] = useState('')
  // 游戏详情页类型 chip 跳转带 ?genre=，与本地切换共用一份状态
  const [searchParams, setSearchParams] = useSearchParams()
  const [genre, setGenre] = useState(searchParams.get('genre') ?? '')

  const selectGenre = (next: string) => {
    setGenre(next)
    const params = new URLSearchParams(searchParams)
    if (next) params.set('genre', next)
    else params.delete('genre')
    setSearchParams(params, { replace: true })
  }

  const loadGames = useCallback(
    async (nextOffset: number) => {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchGames({
          ...(keyword ? { keyword } : {}),
          ...(genre ? { genre } : {}),
          limit: PAGE_SIZE,
          offset: nextOffset,
        })
        // 关键词/类型变化时重置列表，翻页时追加
        setGames((prev) => (nextOffset === 0 ? data.games : [...prev, ...data.games]))
        setTotal(data.total)
        setOffset(nextOffset)
      } catch (err) {
        setError((err as Error).message || '加载游戏库失败')
      } finally {
        setLoading(false)
      }
    },
    [keyword, genre],
  )

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => loadGames(0))
  }, [loadGames])

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setKeyword(searchText.trim())
  }

  const hasMore = games.length < total

  return (
    <div className="games-page">
      <div className="games-header">
        <h1>游戏库</h1>
        <form className="games-search-form" role="search" onSubmit={handleSearch}>
          <input
            type="search"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="搜索游戏名称或关键词"
            aria-label="搜索游戏"
          />
          <button type="submit" className="primary-btn" disabled={loading}>
            搜索
          </button>
        </form>
      </div>

      <div className="games-filter-bar">
        <button
          type="button"
          className={`genre-chip ${genre === '' ? 'genre-chip--active' : ''}`}
          onClick={() => selectGenre('')}
        >
          全部
        </button>
        {['RPG', '动作', '射击', '策略', '冒险', '模拟'].map((g) => (
          <button
            key={g}
            type="button"
            className={`genre-chip ${genre === g ? 'genre-chip--active' : ''}`}
            onClick={() => selectGenre(g)}
          >
            {g}
          </button>
        ))}
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
          <button type="button" className="secondary-btn" onClick={() => loadGames(0)}>
            重试
          </button>
        </div>
      ) : null}

      {loading && games.length === 0 ? (
        <div className="games-empty">加载中...</div>
      ) : games.length === 0 ? (
        <div className="games-empty">
          <p>没有找到匹配的游戏</p>
          <small>换个关键词或清除类型过滤试试</small>
        </div>
      ) : (
        <>
          <div className="games-grid">
            {games.map((game) => (
              <GameCard key={game.id} game={game} />
            ))}
          </div>
          <div className="games-pagination">
            {hasMore ? (
              <button
                type="button"
                className="secondary-btn"
                onClick={() => loadGames(offset + PAGE_SIZE)}
                disabled={loading}
              >
                {loading ? '加载中...' : `加载更多（${games.length}/${total}）`}
              </button>
            ) : (
              <span className="games-pagination-done">共 {total} 款游戏</span>
            )}
          </div>
        </>
      )}
    </div>
  )
}

export default GamesPage
