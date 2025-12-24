import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { fetchPostById, likePost, sharePost, type Post } from '../api/client'
import './community.css'

interface Props {
  token?: string
}

const PostDetailPage = ({ token }: Props) => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [post, setPost] = useState<Post | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (postId: number) => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchPostById(postId)
      setPost(data)
    } catch (err) {
      setError((err as Error).message || '加载帖子失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!id) return
    load(Number(id))
  }, [id, load])

  const formatDate = (dateStr: string) =>
    new Date(dateStr).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })

  const handleLike = async () => {
    if (!token || !post) {
      setError('请先登录')
      return
    }
    try {
      await likePost(post.id, token)
      setPost({ ...post, likeCount: post.likeCount + 1 })
    } catch (err) {
      setError((err as Error).message || '点赞失败')
    }
  }

  const handleShare = async () => {
    if (!token || !post) {
      setError('请先登录')
      return
    }
    try {
      await sharePost(post.id, token)
      setPost({ ...post, shareCount: post.shareCount + 1 })
    } catch (err) {
      setError((err as Error).message || '分享失败')
    }
  }

  if (loading) return <div className="community-page">加载中...</div>

  if (error && !post) {
    return (
      <div className="community-page">
        <div className="error-banner" role="alert">
          {error}
        </div>
        <button type="button" className="secondary-btn" onClick={() => navigate('/community')}>
          返回社区
        </button>
      </div>
    )
  }

  if (!post) return null

  return (
    <div className="community-page">
      <div className="community-header">
        <div>
          <h1>{post.title}</h1>
          <p className="secondary-text">
            👤 {post.authorName || `用户${post.authorId}`} · 🕒 {formatDate(post.createdAt)} · 👁️ {post.viewCount}
          </p>
        </div>
        <div className="inline-actions">
          <Link to="/community" className="secondary-btn">
            ← 返回社区
          </Link>
          <button type="button" className="secondary-btn" onClick={handleLike}>
            👍 点赞 ({post.likeCount})
          </button>
          <button type="button" className="secondary-btn" onClick={handleShare}>
            🔁 分享 ({post.shareCount})
          </button>
        </div>
      </div>

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      <article className="community-card">
        <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{post.content}</p>
      </article>
    </div>
  )
}

export default PostDetailPage

