import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import type { Guide } from '../api/client'
import { fetchGuides, fetchUserInfo } from '../api/client'
import { markdownToPlainText } from '../lib/markdown'
import './guides.css'
import './profile.css'

const UserProfilePage = () => {
  const { id } = useParams()
  const userId = Number(id)
  const [userInfo, setUserInfo] = useState<{ nickname: string; username: string } | null>(null)
  const [guides, setGuides] = useState<Guide[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadProfile = useCallback(async () => {
    if (!Number.isInteger(userId) || userId <= 0) {
      setError('无效的用户')
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [info, userGuides] = await Promise.all([
        fetchUserInfo(userId),
        // 不带 token：仅返回该作者已发布攻略
        fetchGuides({ authorId: userId, limit: 50 }),
      ])
      setUserInfo(info)
      setGuides(userGuides)
    } catch (err) {
      setError((err as Error).message || '加载用户信息失败')
    } finally {
      setLoading(false)
    }
  }, [userId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(loadProfile)
  }, [loadProfile])

  const displayName = userInfo?.nickname || `用户${userId}`
  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  }

  return (
    <div className="guides-page">
      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : loading ? (
        <div className="loading-state">加载中...</div>
      ) : (
        <>
          <div className="profile-header user-profile-header">
            <div className="user-profile-id">
              <span className="user-profile-avatar" aria-hidden="true">
                {displayName.charAt(0).toUpperCase()}
              </span>
              <div>
                <h1>{displayName}</h1>
                <p>@{userInfo?.username || userId}</p>
              </div>
            </div>
          </div>

          <h2 className="user-profile-section-title">发布的攻略（{guides.length}）</h2>
          {guides.length === 0 ? (
            <div className="empty-state">
              <p>TA 还没有发布攻略</p>
            </div>
          ) : (
            <div className="guides-grid">
              {guides.map((guide) => (
                <Link key={guide.id} to={`/guides/${guide.id}`} className="guide-card">
                  <div className="guide-card-header">
                    <div className="guide-title-row">
                      <h3>{guide.title}</h3>
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
                        guide.format === 'markdown'
                          ? markdownToPlainText(guide.content)
                          : guide.content
                      return excerpt.length > 150 ? `${excerpt.substring(0, 150)}...` : excerpt
                    })()}
                  </p>
                  <div className="guide-meta">
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
        </>
      )}
    </div>
  )
}

export default UserProfilePage
