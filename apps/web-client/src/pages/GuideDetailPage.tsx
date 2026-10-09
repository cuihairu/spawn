import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import type { Guide } from '../api/client'
import { fetchGuideById, likeGuide, publishGuide } from '../api/client'
import { renderMarkdown } from '../lib/markdown'
import CommentList from '../components/comments/CommentList'
import './guides.css'

interface Props {
  token?: string
  userId?: number
}

const GuideDetailPage = ({ token, userId }: Props) => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [guide, setGuide] = useState<Guide | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [liked, setLiked] = useState(false)
  const [commentCount, setCommentCount] = useState(0)

  const loadGuide = useCallback(
    async (guideId: number) => {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchGuideById(guideId, token)
        setGuide(data)
        setCommentCount(0)
      } catch (err) {
        setError((err as Error).message || '加载攻略失败')
      } finally {
        setLoading(false)
      }
    },
    [token],
  )

  // 切换攻略时在渲染期重置点赞态（adjust-state-during-render）.
  const [likedForId, setLikedForId] = useState<string | undefined>(id)
  if (id !== likedForId) {
    setLikedForId(id)
    setLiked(false)
  }

  useEffect(() => {
    if (!id) return
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => loadGuide(Number(id)))
  }, [id, loadGuide])

  const handleLike = async () => {
    if (!token || !guide) {
      setError('请先登录')
      return
    }
    try {
      await likeGuide(guide.id, token)
      setLiked(true)
      setGuide({ ...guide, likeCount: guide.likeCount + 1 })
    } catch (err) {
      setError((err as Error).message || '点赞失败')
    }
  }

  const handlePublish = async () => {
    if (!token || !guide) {
      setError('请先登录')
      return
    }
    try {
      await publishGuide(guide.id, token)
      await loadGuide(guide.id)
    } catch (err) {
      setError((err as Error).message || '发布失败')
    }
  }

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  if (loading) {
    return <div className="loading-state">加载中...</div>
  }

  if (error && !guide) {
    return (
      <div className="error-page">
        <p>{error}</p>
        <button type="button" onClick={() => navigate('/guides')}>
          返回攻略列表
        </button>
      </div>
    )
  }

  if (!guide) {
    return null
  }

  const isAuthor = userId === guide.authorId

  return (
    <div className="guide-detail-page">
      <div className="guide-nav">
        <Link to="/guides" className="back-link">
          ← 返回列表
        </Link>
        {isAuthor && (
          <div className="inline-actions">
            <Link to={`/guides/${guide.id}/edit`} className="edit-link">
              ✏️ 编辑
            </Link>
            {guide.status === 'draft' && (
              <button type="button" className="secondary-btn" onClick={handlePublish}>
                🚀 发布
              </button>
            )}
          </div>
        )}
      </div>

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      <article className="guide-detail">
        <header className="guide-detail-header">
          <div className="guide-title-row">
            <h1>{guide.title}</h1>
            <span className={`guide-status ${guide.status}`}>
              {guide.status === 'draft' ? '草稿' : '已发布'}
            </span>
          </div>
          <div className="guide-meta-info">
            <span className="guide-author">
              作者: {guide.authorName || `用户${guide.authorId}`}
            </span>
            <span className="guide-date">
              {guide.status === 'draft' ? '创建于' : '最近更新'}: {formatDate(guide.updatedAt)}
            </span>
            {guide.updatedAt !== guide.createdAt && guide.status === 'draft' && (
              <span className="guide-updated">更新于: {formatDate(guide.updatedAt)}</span>
            )}
          </div>
          <div className="guide-tags-section">
            {guide.tags.map((tag) => (
              <span key={tag} className="guide-tag">
                {tag}
              </span>
            ))}
          </div>
        </header>

        {guide.format === 'markdown' ? (
          <div
            className="guide-content markdown-body"
            dangerouslySetInnerHTML={{ __html: renderMarkdown(guide.content) }}
          />
        ) : (
          <div className="guide-content">{guide.content}</div>
        )}

        <div className="guide-actions-bar">
          <button
            type="button"
            className={`action-btn like-btn ${liked ? 'liked' : ''}`}
            onClick={handleLike}
            disabled={!token || liked || guide.status !== 'published'}
          >
            👍 {liked ? '已赞' : '点赞'} ({guide.likeCount})
          </button>
          <div className="guide-stats-bar">
            <span>👁️ {guide.viewCount} 阅读</span>
            <span>💬 {commentCount} 评论</span>
          </div>
        </div>
      </article>

      {guide.status === 'published' ? (
        <CommentList
          targetType="guide"
          targetId={guide.id}
          currentUserId={userId}
          token={token}
          onCountChange={setCommentCount}
        />
      ) : (
        <div className="comment-login-hint">草稿未发布，评论暂不可用</div>
      )}
    </div>
  )
}

export default GuideDetailPage
