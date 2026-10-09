import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  fetchNotifications,
  fetchUnreadNotificationCount,
  markAllNotificationsRead,
  type AppNotification,
} from '../api/client'
import './notifications.css'

interface Props {
  token: string
  onUnreadChange?: (count: number) => void
}

const TYPE_LABELS: Record<string, string> = {
  like_post: '点赞',
  comment_post: '评论',
  reply_comment: '回复',
  follow_user: '关注',
}

function formatTime(value: string): string {
  if (!value) return '—'
  return value.slice(0, 10)
}

// 通知中心（community 站内通知 web 消费面）：列表 + 未读数 + 全部已读；
// 点赞/评论/回复类点进原帖，关注类不跳转。
const NotificationsPage = ({ token, onUnreadChange }: Props) => {
  const [notifications, setNotifications] = useState<AppNotification[]>([])
  const [total, setTotal] = useState(0)
  const [unread, setUnread] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    const [listRes, countRes] = await Promise.allSettled([
      fetchNotifications(token, 20, 0),
      fetchUnreadNotificationCount(token),
    ])
    if (listRes.status === 'fulfilled') {
      setNotifications(listRes.value.notifications)
      setTotal(listRes.value.total)
    } else {
      setError('通知列表加载失败')
    }
    if (countRes.status === 'fulfilled') {
      setUnread(countRes.value)
      onUnreadChange?.(countRes.value)
    }
    setLoading(false)
  }, [onUnreadChange, token])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(load)
  }, [load])

  const handleMarkAllRead = useCallback(async () => {
    try {
      await markAllNotificationsRead(token)
      setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })))
      setUnread(0)
      onUnreadChange?.(0)
    } catch {
      setError('标记已读失败')
    }
  }, [onUnreadChange, token])

  const openNotification = useCallback(
    (n: AppNotification) => {
      if (n.type === 'follow_user') return
      navigate(`/community/posts/${n.target_id}`)
    },
    [navigate],
  )

  return (
    <div className="notifications-page">
      <div className="notifications-header">
        <div>
          <h1>通知中心</h1>
          <p className="secondary-text">
            共 {total} 条{unread > 0 ? ` · 未读 ${unread} 条` : ''}
          </p>
        </div>
        <div className="notifications-actions">
          <button type="button" className="secondary-btn" onClick={load} disabled={loading}>
            {loading ? '刷新中...' : '刷新'}
          </button>
          <button
            type="button"
            className="secondary-btn"
            onClick={handleMarkAllRead}
            disabled={loading || unread === 0}
          >
            全部已读
          </button>
        </div>
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : null}

      {notifications.length === 0 && !loading ? (
        <div className="notifications-empty">
          <p>还没有通知</p>
          <p className="secondary-text">被点赞、评论或关注时会在这里提醒你</p>
        </div>
      ) : (
        <ul className="notification-list">
          {notifications.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                className={`notification-item${n.is_read ? '' : ' is-unread'}`}
                onClick={() => openNotification(n)}
              >
                <span className={`notification-type type-${n.type}`}>
                  {TYPE_LABELS[n.type] ?? '通知'}
                </span>
                <span className="notification-body">
                  <strong className="notification-actor">{n.actor_name || `用户${n.actor_id}`}</strong>
                  <span className="notification-content">{n.content}</span>
                  <span className="notification-time">{formatTime(n.created_at)}</span>
                </span>
                {!n.is_read ? <span className="notification-dot" aria-label="未读" /> : null}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export default NotificationsPage
