import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  DESIGN_SECTIONS,
  createDesignComment,
  deleteDesignComment,
  fetchDesignBreakdown,
  fetchDesignComments,
  type DesignBreakdown,
  type DesignComment,
} from '../api/design'
import './design.css'

interface Props {
  auth?: { token: string; user: { id: number; nickname: string; username: string } } | null
}

// DesignBreakdownPage 拆解详情：模板分节全文渲染 + 讨论区（复用 community
// 基建：登录发言、删自己评论）。
const DesignBreakdownPage = ({ auth }: Props) => {
  const { id } = useParams()
  const breakdownId = Number(id)

  const [breakdown, setBreakdown] = useState<DesignBreakdown | null>(null)
  const [comments, setComments] = useState<DesignComment[]>([])
  const [draft, setDraft] = useState('')
  const [posting, setPosting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!Number.isFinite(breakdownId) || breakdownId <= 0) {
      setError('拆解不存在')
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [b, cs] = await Promise.all([
        fetchDesignBreakdown(breakdownId),
        fetchDesignComments(breakdownId),
      ])
      setBreakdown(b)
      setComments(cs)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [breakdownId])

  useEffect(() => {
    void load()
  }, [load])

  const submitComment = async () => {
    const content = draft.trim()
    if (!auth || !content || posting) return
    setPosting(true)
    try {
      const comment = await createDesignComment(breakdownId, content, auth.token)
      setComments((prev) => [...prev, comment])
      setDraft('')
    } catch (err) {
      setError(err instanceof Error ? err.message : '评论失败')
    } finally {
      setPosting(false)
    }
  }

  const removeComment = async (commentId: number) => {
    if (!auth) return
    try {
      await deleteDesignComment(breakdownId, commentId, auth.token)
      setComments((prev) => prev.filter((c) => c.id !== commentId))
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  if (loading) return <div className="design-page"><p className="design-state">加载中…</p></div>
  if (error && !breakdown) {
    return (
      <div className="design-page">
        <p className="design-state design-error">{error}</p>
        <Link to="/design" className="design-back">← 返回拆解库</Link>
      </div>
    )
  }
  if (!breakdown) return null

  return (
    <div className="design-page">
      <Link to="/design" className="design-back">← 返回拆解库</Link>
      <header className="design-detail-head">
        <div className="design-card-head">
          <span className="design-card-cat">{breakdown.category}</span>
          {breakdown.featured ? <span className="design-card-featured">精选</span> : null}
        </div>
        <h1>{breakdown.game_title}</h1>
        <p className="design-detail-summary">{breakdown.summary}</p>
        <p className="design-detail-meta">
          {breakdown.author_name} · 更新于 {breakdown.updated_at.slice(0, 10)}
        </p>
      </header>

      {DESIGN_SECTIONS.map(({ key, title }) => {
        const text = breakdown[key]
        if (!text) return null
        return (
          <section key={key} className="design-section">
            <h2>{title}</h2>
            <p>{text}</p>
          </section>
        )
      })}

      {breakdown.sources ? (
        <section className="design-section design-sources">
          <h2>来源</h2>
          <p>{breakdown.sources}</p>
        </section>
      ) : null}

      <section className="design-discussion">
        <h2>讨论区（{comments.length}）</h2>
        {error ? <p className="design-state design-error">{error}</p> : null}
        <ul className="design-comments">
          {comments.map((c) => (
            <li key={c.id} className="design-comment">
              <div className="design-comment-head">
                <span className="design-comment-author">{c.author_name}</span>
                <span className="design-comment-time">{c.created_at.slice(0, 16).replace('T', ' ')}</span>
                {auth && auth.user.id === c.author_id ? (
                  <button
                    type="button"
                    className="design-comment-delete"
                    onClick={() => void removeComment(c.id)}
                  >
                    删除
                  </button>
                ) : null}
              </div>
              <p>{c.content}</p>
            </li>
          ))}
          {comments.length === 0 ? <li className="design-state">还没有讨论，来补充你的视角。</li> : null}
        </ul>
        {auth ? (
          <div className="design-comment-form">
            <textarea
              value={draft}
              maxLength={500}
              placeholder="补充视角、提出质疑、分享同类对比（1-500 字）"
              onChange={(e) => setDraft(e.target.value)}
            />
            <button
              type="button"
              disabled={!draft.trim() || posting}
              onClick={() => void submitComment()}
            >
              {posting ? '发布中…' : '发言'}
            </button>
          </div>
        ) : (
          <p className="design-state">登录后参与讨论。</p>
        )}
      </section>
    </div>
  )
}

export default DesignBreakdownPage
