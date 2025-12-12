import { useState } from 'react'
import type { Comment } from '../../api/client'
import './comments.css'

interface Props {
  comment: Comment
  currentUserId?: number
  onReply?: (parentId: number) => void
  onLike?: (commentId: number) => void
  onDelete?: (commentId: number) => void
}

const CommentItem = ({ comment, currentUserId, onReply, onLike, onDelete }: Props) => {
  const [showReplies, setShowReplies] = useState(false)
  const isAuthor = currentUserId === comment.authorId

  const formattedDate = new Date(comment.createdAt).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })

  return (
    <div className="comment-item">
      <div className="comment-header">
        <span className="comment-author">{comment.authorName || `用户${comment.authorId}`}</span>
        <span className="comment-date">{formattedDate}</span>
      </div>
      <div className="comment-content">{comment.content}</div>
      <div className="comment-actions">
        <button
          type="button"
          className="comment-action-btn"
          onClick={() => onLike?.(comment.id)}
        >
          👍 {comment.likeCount > 0 ? comment.likeCount : '赞'}
        </button>
        <button
          type="button"
          className="comment-action-btn"
          onClick={() => onReply?.(comment.id)}
        >
          💬 回复
        </button>
        {isAuthor && (
          <button
            type="button"
            className="comment-action-btn comment-delete-btn"
            onClick={() => onDelete?.(comment.id)}
          >
            🗑️ 删除
          </button>
        )}
        {comment.replies && comment.replies.length > 0 && (
          <button
            type="button"
            className="comment-action-btn"
            onClick={() => setShowReplies(!showReplies)}
          >
            {showReplies ? '收起回复' : `查看 ${comment.replies.length} 条回复`}
          </button>
        )}
      </div>
      {showReplies && comment.replies && (
        <div className="comment-replies">
          {comment.replies.map((reply) => (
            <CommentItem
              key={reply.id}
              comment={reply}
              currentUserId={currentUserId}
              onReply={onReply}
              onLike={onLike}
              onDelete={onDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export default CommentItem
