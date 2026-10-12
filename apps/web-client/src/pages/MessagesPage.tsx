import { useCallback, useEffect, useRef, useState } from 'react'
import {
  fetchDMConversations,
  fetchDMThread,
  markDMThreadRead,
  sendDirectMessage,
  type DMConversation,
  type DirectMessage,
} from '../api/client'
import './messages.css'

interface Props {
  token: string
  /** App 层实时帧计数（message/message.read 帧）：值变化即重拉会话与当前线程 */
  refreshSignal?: number
  /** 未读私信总数回传 App 导航徽标 */
  onUnreadChange?: (count: number) => void
}

function formatTime(value: string): string {
  if (!value) return '—'
  return value.slice(0, 16).replace('T', ' ')
}

// 私信中心（community 站内私信 web 消费面）：会话列表 + 当前线程 + 发送框；
// 打开会话即置已读（剩余未读数回传徽标），WS message 帧驱动即时刷新。
const MessagesPage = ({ token, refreshSignal = 0, onUnreadChange }: Props) => {
  const [conversations, setConversations] = useState<DMConversation[]>([])
  const [activePeer, setActivePeer] = useState<number | null>(null)
  const [messages, setMessages] = useState<DirectMessage[]>([])
  const [total, setTotal] = useState(0)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setConversations(await fetchDMConversations(token))
    } catch {
      setError('会话列表加载失败')
    }
    setLoading(false)
  }, [token])

  const openThread = useCallback(
    async (peerId: number) => {
      setActivePeer(peerId)
      setError(null)
      try {
        const [threadRes, remaining] = await Promise.all([
          fetchDMThread(token, peerId, 50, 0),
          markDMThreadRead(token, peerId),
        ])
        // 倒序取回，展示层反转为时间正序
        setMessages([...threadRes.messages].reverse())
        setTotal(threadRes.total)
        onUnreadChange?.(remaining)
        setConversations((prev) =>
          prev.map((c) => (c.peer_id === peerId ? { ...c, unread: 0 } : c)),
        )
      } catch {
        setError('消息加载失败')
      }
    },
    [onUnreadChange, token],
  )

  // 首次进入 + 实时帧（新私信/已读回执）即重拉
  useEffect(() => {
    queueMicrotask(load)
  }, [load, refreshSignal])

  // 实时帧也可能补进当前打开的线程：帧到达时若已选中会话则重拉线程
  useEffect(() => {
    if (refreshSignal > 0 && activePeer !== null) {
      queueMicrotask(() => {
        fetchDMThread(token, activePeer, 50, 0)
          .then((threadRes) => {
            setMessages([...threadRes.messages].reverse())
            setTotal(threadRes.total)
          })
          .catch(() => {
            // 静默：线程刷新失败保留现有内容
          })
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshSignal])

  // 新消息进屏：线程滚动到底
  useEffect(() => {
    const el = listRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages])

  const handleSend = async () => {
    const content = draft.trim()
    if (!content || activePeer === null || sending) return
    setSending(true)
    try {
      const msg = await sendDirectMessage(token, activePeer, content)
      setMessages((prev) => [...prev, msg])
      setTotal((t) => t + 1)
      setDraft('')
      setConversations((prev) => {
        const others = prev.filter((c) => c.peer_id !== activePeer)
        return [
          {
            peer_id: activePeer,
            last_message: msg.content,
            last_at: msg.created_at,
            unread: 0,
          },
          ...others,
        ]
      })
    } catch {
      setError('发送失败，请重试')
    }
    setSending(false)
  }

  if (!token) {
    return (
      <div className="error-page">
        <p>请先登录</p>
      </div>
    )
  }

  return (
    <div className="messages-page">
      <h2>私信</h2>
      {error && <div className="messages-error">{error}</div>}
      <div className="messages-layout">
        <aside className="dm-conversations">
          {conversations.length === 0 && !loading && (
            <p className="dm-empty">还没有会话，去社区认识朋友吧</p>
          )}
          {conversations.map((c) => (
            <button
              key={c.peer_id}
              className={`dm-conversation${c.peer_id === activePeer ? ' active' : ''}`}
              onClick={() => {
                void openThread(c.peer_id)
              }}
            >
              <span className="dm-peer">用户 #{c.peer_id}</span>
              <span className="dm-preview">{c.last_message}</span>
              <span className="dm-meta">
                {formatTime(c.last_at)}
                {c.unread > 0 && <em className="dm-unread">{c.unread > 99 ? '99+' : c.unread}</em>}
              </span>
            </button>
          ))}
        </aside>
        <section className="dm-thread">
          {activePeer === null ? (
            <p className="dm-empty">选择左侧会话开始聊天</p>
          ) : (
            <>
              <div className="dm-thread-head">
                与用户 #{activePeer} 的对话（{total} 条）
              </div>
              <div className="dm-thread-list" ref={listRef}>
                {messages.map((m) => (
                  <div
                    key={m.id}
                    className={`dm-bubble ${m.sender_id === activePeer ? 'from-peer' : 'from-me'}`}
                  >
                    <p className="dm-content">{m.content}</p>
                    <span className="dm-time">{formatTime(m.created_at)}</span>
                  </div>
                ))}
              </div>
              <div className="dm-composer">
                <textarea
                  value={draft}
                  placeholder="输入消息（Enter 发送，Shift+Enter 换行）"
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault()
                      void handleSend()
                    }
                  }}
                />
                <button type="button" disabled={sending || !draft.trim()} onClick={() => void handleSend()}>
                  发送
                </button>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  )
}

export default MessagesPage
