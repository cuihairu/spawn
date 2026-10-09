import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  createPostComment,
  deletePostComment,
  deletePost,
  fetchPostById,
  fetchPostComments,
  likePost,
  resolveImageUrl,
  sharePost,
  updatePost,
  type Post,
  type PostComment,
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
  // 评论（community 域帖子评论）：列表 + 发评/删评；回复带 parent_id。
  const [comments, setComments] = useState<PostComment[]>([])
  const [commentTotal, setCommentTotal] = useState(0)
  const [commentDraft, setCommentDraft] = useState('')
  const [replyTo, setReplyTo] = useState<PostComment | null>(null)
  const [commentBusy, setCommentBusy] = useState(false)

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

  const loadComments = useCallback(async (postId: number) => {
    try {
      const res = await fetchPostComments(postId)
      setComments(res.comments)
      setCommentTotal(res.total)
    } catch {
      // 评论加载失败不阻塞帖子本体，置空即可
      setComments([])
      setCommentTotal(0)
    }
  }, [])

  useEffect(() => {
    if (!id) return
    const postId = Number(id)
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => {
      load(postId)
      loadComments(postId)
    })
  }, [id, load, loadComments])

  // post 到达或变化时在渲染期同步编辑草稿（adjust-state-during-render）.
  const [draftFor, setDraftFor] = useState<Post | null>(null)
  if (post && post !== draftFor) {
    setDraftFor(post)
    setDraft({
      title: post.title,
      content: post.content,
      tags: (post.tags ?? []).join(', '),
    })
  }

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

  const handleSubmitComment = async () => {
    if (!post) return
    if (!token) {
      setError('请先登录')
      return
    }
    const content = commentDraft.trim()
    if (!content) {
      setError('评论内容不能为空')
      return
    }
    setCommentBusy(true)
    setError(null)
    try {
      const created = await createPostComment(
        post.id,
        content,
        replyTo ? replyTo.id : undefined,
        token ?? '',
      )
      setComments((prev) => [...prev, created])
      setCommentTotal((n) => n + 1)
      // 帖子评论计数单调累加，同步顶栏展示
      setPost((prev) => (prev ? { ...prev, commentCount: prev.commentCount + 1 } : prev))
      setCommentDraft('')
      setReplyTo(null)
    } catch (err) {
      setError((err as Error).message || '评论失败')
    } finally {
      setCommentBusy(false)
    }
  }

  const handleDeleteComment = async (comment: PostComment) => {
    if (!post || !token) return
    setCommentBusy(true)
    setError(null)
    try {
      await deletePostComment(post.id, comment.id, token)
      setComments((prev) => prev.filter((c) => c.id !== comment.id))
      setCommentTotal((n) => Math.max(0, n - 1))
      // 计数契约单调不回退（与点赞同款），顶栏 commentCount 保持不动
    } catch (err) {
      setError((err as Error).message || '删除评论失败')
    } finally {
      setCommentBusy(false)
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
            {post.images && post.images.length > 0 ? (
              <div className="post-images">
                {post.images.map((src) => (
                  <img
                    key={src}
                    src={resolveImageUrl(src)}
                    alt={`${post.title} 附图`}
                    loading="lazy"
                  />
                ))}
              </div>
            ) : null}

            <div className="post-comments">
              <h2 className="post-comments-title">评论（{commentTotal}）</h2>
              {comments.length === 0 ? (
                <div className="secondary-text">还没有评论，来抢沙发</div>
              ) : (
                <div className="post-comments-list">
                  {comments.map((comment) => (
                    <div key={comment.id} className="post-comment">
                      <div className="post-comment-head">
                        <span className="post-comment-author">
                          👤 {comment.author_name || `用户${comment.author_id}`}
                        </span>
                        <span className="post-comment-time">{formatDate(comment.created_at)}</span>
                        {token && userId === comment.author_id ? (
                          <button
                            type="button"
                            className="post-comment-delete"
                            onClick={() => handleDeleteComment(comment)}
                            disabled={commentBusy}
                          >
                            删除
                          </button>
                        ) : null}
                      </div>
                      <div className="post-comment-content">
                        {comment.reply_to_author_name ? (
                          <span className="post-comment-reply">回复 @{comment.reply_to_author_name}：</span>
                        ) : null}
                        {comment.content}
                      </div>
                      {token ? (
                        <button
                          type="button"
                          className="post-comment-reply-btn"
                          onClick={() => {
                            setReplyTo(comment)
                            setCommentDraft('')
                          }}
                          disabled={commentBusy}
                        >
                          回复
                        </button>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}

              {token ? (
                <div className="post-comment-form">
                  {replyTo ? (
                    <div className="post-comment-replying">
                      回复 @{replyTo.author_name || `用户${replyTo.author_id}`}
                      <button type="button" className="post-comment-cancel" onClick={() => setReplyTo(null)}>
                        取消回复
                      </button>
                    </div>
                  ) : null}
                  <textarea
                    value={commentDraft}
                    onChange={(e) => setCommentDraft(e.target.value)}
                    placeholder={replyTo ? `回复 @${replyTo.author_name || `用户${replyTo.author_id}`}...` : '写下你的评论...'}
                    disabled={commentBusy}
                  />
                  <button
                    type="button"
                    className="primary-btn"
                    onClick={handleSubmitComment}
                    disabled={commentBusy}
                  >
                    {commentBusy ? '发送中...' : '发表评论'}
                  </button>
                </div>
              ) : (
                <div className="secondary-text">登录后可评论</div>
              )}
            </div>
          </>
        )}
      </article>
    </div>
  )
}

export default PostDetailPage
