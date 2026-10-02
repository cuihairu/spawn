import { useCallback, useEffect, useState } from 'react'
import type { Comment } from '../../api/client'
import { fetchComments, createComment, deleteComment, likeComment } from '../../api/client'
import CommentItem from './CommentItem'
import './comments.css'

interface Props {
  targetType: 'guide' | 'game' | 'comment'
  targetId: number
  currentUserId?: number
  token?: string
  onCountChange?: (count: number) => void
}

function countComments(items: Comment[]): number {
  return items.reduce((total, item) => total + 1 + countComments(item.replies ?? []), 0)
}

const CommentList = ({ targetType, targetId, currentUserId, token, onCountChange }: Props) => {
  const [comments, setComments] = useState<Comment[]>([])
  const [loading, setLoading] = useState(false)
  const [newComment, setNewComment] = useState('')
  const [replyToId, setReplyToId] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  // When switching targets, reset composing state during render to avoid replying
  // to the wrong entity (adjust-state-during-render, 同步 setState 不进 effect).
  const targetKey = `${targetType}:${targetId}`
  const [prevTargetKey, setPrevTargetKey] = useState(targetKey)
  if (targetKey !== prevTargetKey) {
    setPrevTargetKey(targetKey)
    setNewComment('')
    setReplyToId(null)
  }

  const loadComments = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchComments({ targetType, targetId, limit: 100 })
      setComments(data)
      onCountChange?.(countComments(data))
    } catch (err) {
      setError((err as Error).message || '加载评论失败')
    } finally {
      setLoading(false)
    }
  }, [onCountChange, targetType, targetId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(loadComments)
  }, [loadComments])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newComment.trim() || !token) return

    setError(null)
    try {
      await createComment(
        {
          targetType,
          targetId,
          content: newComment.trim(),
          parentId: replyToId || undefined,
        },
        token,
      )
      setNewComment('')
      setReplyToId(null)
      await loadComments()
    } catch (err) {
      setError((err as Error).message || '发表评论失败')
    }
  }

  const handleLike = async (commentId: number) => {
    if (!token) {
      setError('请先登录')
      return
    }
    try {
      await likeComment(commentId, token)
      await loadComments()
    } catch (err) {
      setError((err as Error).message || '点赞失败')
    }
  }

  const handleDelete = async (commentId: number) => {
    if (!token) return
    if (!confirm('确定要删除这条评论吗?')) return

    try {
      await deleteComment(commentId, token)
      await loadComments()
    } catch (err) {
      setError((err as Error).message || '删除失败')
    }
  }

  const handleReply = (parentId: number) => {
    setReplyToId(parentId)
  }

  return (
    <div className="comment-list">
      <h3>评论 ({comments.length})</h3>

      {error && (
        <div className="comment-error" role="alert">
          {error}
        </div>
      )}

      {token ? (
        <form className="comment-form" onSubmit={handleSubmit}>
          {replyToId && (
            <div className="comment-reply-hint">
              回复评论 #{replyToId}
              <button
                type="button"
                className="cancel-reply-btn"
                onClick={() => setReplyToId(null)}
              >
                取消
              </button>
            </div>
          )}
          <textarea
            className="comment-textarea"
            value={newComment}
            onChange={(e) => setNewComment(e.target.value)}
            placeholder={replyToId ? '写下你的回复...' : '写下你的评论...'}
            rows={3}
          />
          <button type="submit" className="comment-submit-btn" disabled={!newComment.trim()}>
            发表
          </button>
        </form>
      ) : (
        <div className="comment-login-hint">登录后即可发表评论</div>
      )}

      {loading ? (
        <div className="comment-loading">加载中...</div>
      ) : comments.length === 0 ? (
        <div className="comment-empty">暂无评论，来抢沙发吧！</div>
      ) : (
        <div className="comment-items">
          {comments.map((comment) => (
            <CommentItem
              key={comment.id}
              comment={comment}
              currentUserId={currentUserId}
              onReply={handleReply}
              onLike={handleLike}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export default CommentList
