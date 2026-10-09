import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import type { Guide } from '../api/client'
import { createGuide, updateGuide, fetchGuideById, publishGuide } from '../api/client'
import { renderMarkdown } from '../lib/markdown'
import './guides.css'

interface Props {
  token: string
  userId: number
}

const GuideEditorPage = ({ token, userId }: Props) => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const isEditMode = !!id

  const [guide, setGuide] = useState<Guide | null>(null)
  const [form, setForm] = useState({
    gameId: '',
    title: '',
    content: '',
    format: 'text' as 'text' | 'markdown',
    tags: '',
  })
  const [preview, setPreview] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadGuide = useCallback(
    async (guideId: number) => {
      setLoading(true)
      setError(null)
      try {
        const data = await fetchGuideById(guideId, token)
        if (data.authorId !== userId) {
          setError('你没有权限编辑此攻略')
          return
        }
        setGuide(data)
        setForm({
          gameId: data.gameId,
          title: data.title,
          content: data.content,
          format: data.format,
          tags: data.tags.join(', '),
        })
      } catch (err) {
        setError((err as Error).message || '加载攻略失败')
      } finally {
        setLoading(false)
      }
    },
    [token, userId],
  )

  useEffect(() => {
    if (!isEditMode || !id) return
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => loadGuide(Number(id)))
  }, [id, isEditMode, loadGuide])

  const handleSubmit = async (e: React.FormEvent, shouldPublish = false) => {
    e.preventDefault()
    if (!form.title.trim() || !form.content.trim()) {
      setError('标题和内容不能为空')
      return
    }

    setSaving(true)
    setError(null)

    try {
      const tags = form.tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)

      let savedGuide: Guide
      if (isEditMode && id) {
        savedGuide = await updateGuide(
          Number(id),
          {
            gameId: form.gameId,
            title: form.title,
            content: form.content,
            format: form.format,
            tags,
          },
          token,
        )
      } else {
        savedGuide = await createGuide(
          {
            gameId: form.gameId || 'default',
            title: form.title,
            content: form.content,
            format: form.format,
            tags,
          },
          token,
        )
      }

      if (shouldPublish && savedGuide.status === 'draft') {
        await publishGuide(savedGuide.id, token)
      }

      navigate(`/guides/${savedGuide.id}`)
    } catch (err) {
      setError((err as Error).message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  if (!token) {
    return (
      <div className="error-page">
        <p>请先登录才能创建攻略</p>
        <button type="button" onClick={() => navigate('/')}>
          返回首页
        </button>
      </div>
    )
  }

  if (loading) {
    return <div className="loading-state">加载中...</div>
  }

  if (error && isEditMode && !guide) {
    return (
      <div className="error-page">
        <p>{error}</p>
        <button type="button" onClick={() => navigate('/guides')}>
          返回攻略列表
        </button>
      </div>
    )
  }

  return (
    <div className="guide-editor-page">
      <h1>{isEditMode ? '编辑攻略' : '创建攻略'}</h1>

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      <form className="guide-form" onSubmit={(e) => handleSubmit(e, false)}>
        <div className="form-group">
          <label htmlFor="gameId">
            游戏 ID <span className="optional">(可选)</span>
          </label>
          <input
            id="gameId"
            type="text"
            value={form.gameId}
            onChange={(e) => setForm({ ...form, gameId: e.target.value })}
            placeholder="例如: game-001"
          />
        </div>

        <div className="form-group">
          <label htmlFor="title">
            标题 <span className="required">*</span>
          </label>
          <input
            id="title"
            type="text"
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
            placeholder="为你的攻略取个标题"
            required
          />
        </div>

        <div className="form-group">
          <label htmlFor="content">
            内容 <span className="required">*</span>
          </label>
          {form.format === 'markdown' && (
            <div className="editor-toolbar">
              <button
                type="button"
                className={`toolbar-btn ${preview ? '' : 'active'}`}
                onClick={() => setPreview(false)}
              >
                编辑
              </button>
              <button
                type="button"
                className={`toolbar-btn ${preview ? 'active' : ''}`}
                onClick={() => setPreview(true)}
              >
                预览
              </button>
            </div>
          )}
          {form.format === 'markdown' && preview ? (
            <div className="markdown-body markdown-preview">
              {form.content.trim() ? (
                <div dangerouslySetInnerHTML={{ __html: renderMarkdown(form.content) }} />
              ) : (
                <span className="preview-empty">暂无内容可预览</span>
              )}
            </div>
          ) : (
            <textarea
              id="content"
              value={form.content}
              onChange={(e) => setForm({ ...form, content: e.target.value })}
              placeholder={
                form.format === 'markdown'
                  ? '支持 Markdown 语法：# 标题、**加粗**、- 列表、```代码块``` 等'
                  : '分享你的游戏心得和技巧...'
              }
              rows={15}
              required
            />
          )}
        </div>

        <div className="form-group">
          <label htmlFor="format">内容格式</label>
          <select
            id="format"
            value={form.format}
            onChange={(e) => setForm({ ...form, format: e.target.value as 'text' | 'markdown' })}
          >
            <option value="text">纯文本</option>
            <option value="markdown">Markdown</option>
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="tags">
            标签 <span className="optional">(可选，用逗号分隔)</span>
          </label>
          <input
            id="tags"
            type="text"
            value={form.tags}
            onChange={(e) => setForm({ ...form, tags: e.target.value })}
            placeholder="新手, 进阶, 装备推荐"
          />
        </div>

        <div className="form-actions">
          <button type="button" className="cancel-btn" onClick={() => navigate(-1)}>
            取消
          </button>
          <button type="submit" className="save-draft-btn" disabled={saving}>
            {saving ? '保存中...' : '保存草稿'}
          </button>
          <button
            type="button"
            className="publish-btn"
            onClick={(e) => handleSubmit(e, true)}
            disabled={saving}
          >
            {saving ? '发布中...' : '保存并发布'}
          </button>
        </div>
      </form>
    </div>
  )
}

export default GuideEditorPage
