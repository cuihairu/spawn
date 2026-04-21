import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  deletePost,
  fetchPostById,
  likePost,
  sharePost,
  updatePost,
  type Post,
} from '../api/client'
import './community.css'

interface Props {
  token?: string
  userId?: number
}

const PostDetailPage = ({ token, userId }: Props) => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [post, setPost] = useState<Post | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState({ title: '', content: '', tags: '' })

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

  useEffect(() => {
    if (!post) return
    setDraft({
      title: post.title,
      content: post.content,
      tags: (post.tags ?? []).join(', '),
    })
  }, [post])

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

  const isAuthor = Boolean(post && userId && post.authorId === userId)

  const handleStartEdit = () => {
    if (!post) return
    setDraft({
      title: post.title,
      content: post.content,
      tags: (post.tags ?? []).join(', '),
    })
    setEditing(true)
    setError(null)
  }

  const handleCancelEdit = () => {
    setEditing(false)
    setError(null)
    if (!post) return
    setDraft({
      title: post.title,
      content: post.content,
      tags: (post.tags ?? []).join(', '),
    })
  }

  const handleSave = async () => {
    if (!token || !post) {
      setError('请先登录')
      return
    }
    if (!draft.title.trim() || !draft.content.trim()) {
      setError('标题和内容不能为空')
      return
    }

    setSaving(true)
    setError(null)
    try {
      const updated = await updatePost(
        post.id,
        {
          title: draft.title.trim(),
          content: draft.content.trim(),
          tags: draft.tags
            .split(',')
            .map((tag) => tag.trim())
            .filter(Boolean)
            .slice(0, 8),
        },
        token,
      )
      setPost(updated)
      setEditing(false)
    } catch (err) {
      setError((err as Error).message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!token || !post) {
      setError('请先登录')
      return
    }

    const confirmed = window.confirm('删除后不可恢复，确认删除这篇帖子？')
    if (!confirmed) return

    setSaving(true)
    setError(null)
    try {
      await deletePost(post.id, token)
      navigate('/community')
    } catch (err) {
      setError((err as Error).message || '删除失败')
      setSaving(false)
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
          {isAuthor ? (
            <>
              {editing ? (
                <>
                  <button type="button" className="secondary-btn" onClick={handleCancelEdit} disabled={saving}>
                    取消编辑
                  </button>
                  <button type="button" className="primary-btn" onClick={handleSave} disabled={saving}>
                    {saving ? '保存中...' : '保存修改'}
                  </button>
                </>
              ) : (
                <>
                  <button type="button" className="secondary-btn" onClick={handleStartEdit}>
                    编辑帖子
                  </button>
                  <button type="button" className="secondary-btn danger-btn" onClick={handleDelete} disabled={saving}>
                    {saving ? '删除中...' : '删除帖子'}
                  </button>
                </>
              )}
            </>
          ) : null}
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
        {editing ? (
          <div className="post-form">
            <input
              value={draft.title}
              onChange={(e) => setDraft((prev) => ({ ...prev, title: e.target.value }))}
              placeholder="标题"
              disabled={saving}
            />
            <textarea
              value={draft.content}
              onChange={(e) => setDraft((prev) => ({ ...prev, content: e.target.value }))}
              placeholder="正文内容"
              disabled={saving}
            />
            <input
              value={draft.tags}
              onChange={(e) => setDraft((prev) => ({ ...prev, tags: e.target.value }))}
              placeholder="标签（逗号分隔，可选）"
              disabled={saving}
            />
          </div>
        ) : (
          <>
            {post.tags && post.tags.length > 0 ? (
              <div className="post-tags">
                {post.tags.map((tag) => (
                  <span key={tag} className="post-tag">
                    #{tag}
                  </span>
                ))}
              </div>
            ) : null}
            <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{post.content}</p>
          </>
        )}
      </article>
    </div>
  )
}

export default PostDetailPage
