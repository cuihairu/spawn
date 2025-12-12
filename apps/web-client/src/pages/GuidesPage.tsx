import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Guide } from '../api/client'
import { fetchGuides } from '../api/client'
import './guides.css'

interface Props {
  token?: string
  userId?: number
}

const GuidesPage = ({ token, userId }: Props) => {
  const [guides, setGuides] = useState<Guide[]>([])
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState<'all' | 'my'>('all')
  const [error, setError] = useState<string | null>(null)

  const loadGuides = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params = {
        status: 'published' as const,
        ...(filter === 'my' && userId ? { authorId: userId } : {}),
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
  }, [filter, token, userId])

  useEffect(() => {
    loadGuides()
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
          <div className="filter-tabs">
            <button
              type="button"
              className={`filter-tab ${filter === 'all' ? 'active' : ''}`}
              onClick={() => setFilter('all')}
            >
              全部攻略
            </button>
            {userId && (
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

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      {loading ? (
        <div className="loading-state">加载中...</div>
      ) : guides.length === 0 ? (
        <div className="empty-state">
          <p>{filter === 'my' ? '你还没有创建攻略' : '暂无攻略'}</p>
          {token && (
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
                <h3>{guide.title}</h3>
                <div className="guide-tags">
                  {guide.tags.slice(0, 3).map((tag) => (
                    <span key={tag} className="guide-tag">
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
              <p className="guide-preview">
                {guide.content.length > 150
                  ? `${guide.content.substring(0, 150)}...`
                  : guide.content}
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
