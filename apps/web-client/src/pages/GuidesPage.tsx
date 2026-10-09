import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import type { Guide } from '../api/client'
import { fetchGuides } from '../api/client'
import { markdownToPlainText } from '../lib/markdown'
import './guides.css'

interface Props {
  token?: string
  userId?: number
}

const GuidesPage = ({ token, userId }: Props) => {
  const [guides, setGuides] = useState<Guide[]>([])
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<'all' | 'my'>('all')
  const [keyword, setKeyword] = useState('')
  const [searchText, setSearchText] = useState('')
  const [error, setError] = useState<string | null>(null)
  // 游戏详情页跳转带 ?gameId=&title= 过滤本游戏攻略（对等 mobile /guides?game_id=）
  const [searchParams, setSearchParams] = useSearchParams()
  const gameId = searchParams.get('gameId') ?? ''
  const gameTitle = searchParams.get('title') ?? gameId

  const loadGuides = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params = {
        ...(filter === 'my' && userId ? { authorId: userId } : {}),
        ...(keyword ? { keyword } : {}),
        ...(gameId ? { gameId } : {}),
        limit: 50,
        token,
      }
      const data = await fetchGuides(params)
      setGuides(data)
    } catch (err) {
      setError((err as Error).message || '加载攻略失败')
    } finally {
      setLoading(false)
    }
  }, [filter, keyword, gameId, token, userId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(loadGuides)
  }, [loadGuides])

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  }

  return (
    <div className="guides-page">
      <div className="guides-header">
        <h1>游戏攻略</h1>
        <div className="guides-actions">
          <form
            className="guide-search-form"
            role="search"
            onSubmit={(e) => {
              e.preventDefault()
              setKeyword(searchText.trim())
            }}
          >
            <input
              type="search"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder="搜索攻略标题、摘要或正文"
              aria-label="搜索攻略"
            />
            {searchText && (
              <button
                type="button"
                className="guide-search-clear"
                aria-label="清空搜索"
                onClick={() => {
                  setSearchText('')
                  setKeyword('')
                }}
              >
                ×
              </button>
            )}
            <button type="submit" className="guide-search-submit">
              搜索
            </button>
          </form>
          <div className="filter-tabs">
            <button
              type="button"
              className={`filter-tab ${filter === 'all' ? 'active' : ''}`}
              onClick={() => setFilter('all')}
            >
              全部攻略
            </button>
            {token && userId && (
              <button
                type="button"
                className={`filter-tab ${filter === 'my' ? 'active' : ''}`}
                onClick={() => setFilter('my')}
              >
                我的攻略
              </button>
            )}
          </div>
          {token && (
            <Link to="/guides/new" className="create-guide-btn">
              ✍️ 创建攻略
            </Link>
          )}
        </div>
      </div>

      {gameId ? (
        <div className="guide-game-filter">
          <span>按游戏过滤中：{gameTitle}</span>
          <button
            type="button"
            className="guide-search-clear"
            aria-label="清除游戏过滤"
            onClick={() => {
              const next = new URLSearchParams(searchParams)
              next.delete('gameId')
              next.delete('title')
              setSearchParams(next, { replace: true })
            }}
          >
            ×
          </button>
        </div>
      ) : null}

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      {loading ? (
        <div className="loading-state">加载中...</div>
      ) : guides.length === 0 ? (
        <div className="empty-state">
          <p>
            {keyword
              ? `没有找到与「${keyword}」匹配的攻略`
              : filter === 'my'
                ? '你还没有创建攻略'
                : '暂无攻略'}
          </p>
          {keyword && (
            <button
              type="button"
              className="create-guide-btn"
              onClick={() => {
                setSearchText('')
                setKeyword('')
              }}
            >
              清空搜索
            </button>
          )}
          {!keyword && token && (
            <Link to="/guides/new" className="create-guide-btn">
              创建第一篇攻略
            </Link>
          )}
        </div>
      ) : (
        <div className="guides-grid">
          {guides.map((guide) => (
            <Link key={guide.id} to={`/guides/${guide.id}`} className="guide-card">
              <div className="guide-card-header">
                <div className="guide-title-row">
                  <h3>{guide.title}</h3>
                  <span className={`guide-status ${guide.status}`}>
                    {guide.status === 'draft' ? '草稿' : '已发布'}
                  </span>
                </div>
                <div className="guide-tags">
                  {guide.tags.slice(0, 3).map((tag) => (
                    <span key={tag} className="guide-tag">
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
              <p className="guide-preview">
                {(() => {
                  const excerpt =
                    guide.format === 'markdown' ? markdownToPlainText(guide.content) : guide.content
                  return excerpt.length > 150 ? `${excerpt.substring(0, 150)}...` : excerpt
                })()}
              </p>
              <div className="guide-meta">
                <span className="guide-author">
                  {guide.authorName || `用户${guide.authorId}`}
                </span>
                <span className="guide-date">{formatDate(guide.createdAt)}</span>
              </div>
              <div className="guide-stats">
                <span>👁️ {guide.viewCount}</span>
                <span>👍 {guide.likeCount}</span>
                <span>💬 {guide.commentCount}</span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

export default GuidesPage
