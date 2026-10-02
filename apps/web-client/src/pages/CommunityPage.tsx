import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  createPost,
  createTopic,
  fetchFollowingTopics,
  fetchHotPosts,
  fetchPosts,
  fetchTopics,
  followTopic,
  unfollowTopic,
  type Post,
  type Topic,
} from '../api/client'
import './community.css'

interface Props {
  token?: string
  userId?: number
}

const CommunityPage = ({ token, userId }: Props) => {
  const [topics, setTopics] = useState<Topic[]>([])
  const [activeTopicId, setActiveTopicId] = useState<number | null>(null)
  const [posts, setPosts] = useState<Post[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedMode, setSelectedMode] = useState<'latest' | 'hot' | 'mine'>('latest')
  // 未登录时「我的」不可用，渲染期推导回退到最新（替代原 effect 内的同步 setState）.
  const mode = selectedMode === 'mine' && (!token || !userId) ? 'latest' : selectedMode
  const [following, setFollowing] = useState<Set<number>>(new Set())

  const [topicDraft, setTopicDraft] = useState({ name: '', description: '' })
  const [postDraft, setPostDraft] = useState({ title: '', content: '', tags: '' })

  const updateTopicStats = useCallback((topicId: number, updater: (topic: Topic) => Topic) => {
    setTopics((prev) => prev.map((topic) => (topic.id === topicId ? updater(topic) : topic)))
  }, [])

  const activeTopic = useMemo(() => {
    if (!activeTopicId) return null
    return topics.find((t) => t.id === activeTopicId) ?? null
  }, [activeTopicId, topics])

  const loadTopics = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetchTopics({ limit: 50 })
      setTopics(res.topics)
      if (!activeTopicId && res.topics.length > 0) {
        setActiveTopicId(res.topics[0].id)
      }
    } catch (err) {
      setError((err as Error).message || '加载话题失败')
    } finally {
      setLoading(false)
    }
  }, [activeTopicId])

  const loadFollowing = useCallback(async () => {
    if (!token) {
      setFollowing(new Set())
      return
    }
    try {
      const list = await fetchFollowingTopics(token)
      setFollowing(new Set(list.map((t) => t.id)))
    } catch {
      // best-effort
    }
  }, [token])

  const loadPosts = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      if (mode === 'hot') {
        const list = await fetchHotPosts(20)
        setPosts(list)
        return
      }
      const res = await fetchPosts({
        topicId: mode === 'mine' ? undefined : activeTopicId ?? undefined,
        authorId: mode === 'mine' && userId ? userId : undefined,
        limit: 50,
      })
      setPosts(res.posts)
    } catch (err) {
      setError((err as Error).message || '加载帖子失败')
    } finally {
      setLoading(false)
    }
  }, [activeTopicId, mode, userId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => {
      loadTopics()
      loadFollowing()
    })
  }, [loadTopics, loadFollowing])

  useEffect(() => {
    queueMicrotask(loadPosts)
  }, [loadPosts])

  const handleCreateTopic = async () => {
    if (!token) {
      setError('请先登录')
      return
    }
    if (!topicDraft.name.trim()) {
      setError('请输入话题名称')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const created = await createTopic(
        { name: topicDraft.name.trim(), description: topicDraft.description.trim() },
        token,
      )
      setTopics((prev) => [created, ...prev])
      setActiveTopicId(created.id)
      setTopicDraft({ name: '', description: '' })
      setSelectedMode('latest')
      setPosts([])
    } catch (err) {
      setError((err as Error).message || '创建话题失败')
    } finally {
      setLoading(false)
    }
  }

  const handleToggleFollow = async () => {
    if (!token || !activeTopicId) {
      setError('请先登录并选择话题')
      return
    }
    setLoading(true)
    setError(null)
    try {
      if (following.has(activeTopicId)) {
        await unfollowTopic(activeTopicId, token)
        setFollowing((prev) => {
          const next = new Set(prev)
          next.delete(activeTopicId)
          return next
        })
        updateTopicStats(activeTopicId, (topic) => ({
          ...topic,
          followerCount: Math.max(0, topic.followerCount - 1),
        }))
      } else {
        await followTopic(activeTopicId, token)
        setFollowing((prev) => new Set(prev).add(activeTopicId))
        updateTopicStats(activeTopicId, (topic) => ({
          ...topic,
          followerCount: topic.followerCount + 1,
        }))
      }
    } catch (err) {
      setError((err as Error).message || '操作失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreatePost = async () => {
    if (!token) {
      setError('请先登录')
      return
    }
    if (!activeTopicId) {
      setError('请先选择话题')
      return
    }
    if (!postDraft.title.trim() || !postDraft.content.trim()) {
      setError('标题和内容不能为空')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const tags = postDraft.tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
        .slice(0, 8)
      const created = await createPost(
        { topicId: activeTopicId, title: postDraft.title.trim(), content: postDraft.content.trim(), tags },
        token,
      )
      setPostDraft({ title: '', content: '', tags: '' })
      setSelectedMode('mine')
      setPosts((prev) => [created, ...prev.filter((item) => item.id !== created.id)])
      updateTopicStats(activeTopicId, (topic) => ({
        ...topic,
        postCount: topic.postCount + 1,
      }))
    } catch (err) {
      setError((err as Error).message || '发帖失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="community-page">
      <div className="community-header">
        <div>
          <h1>社区</h1>
          <p className="secondary-text">话题圈子 + 帖子流（Community Service）</p>
        </div>
        <div className="inline-actions">
          <button type="button" className="secondary-btn" onClick={loadTopics} disabled={loading}>
            刷新话题
          </button>
          <button type="button" className="secondary-btn" onClick={loadPosts} disabled={loading}>
            刷新帖子
          </button>
        </div>
      </div>

      {error && (
        <div className="error-banner" role="alert">
          {error}
        </div>
      )}

      <div className="community-grid">
        <section className="community-card">
          <h2>话题圈子</h2>
          <div className="topic-list">
            {topics.length === 0 ? (
              <div className="secondary-text">暂无话题</div>
            ) : (
              topics.map((topic) => (
                <div
                  key={topic.id}
                  className={`topic-item ${topic.id === activeTopicId ? 'active' : ''}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => {
                    setActiveTopicId(topic.id)
                    setSelectedMode('latest')
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      setActiveTopicId(topic.id)
                      setSelectedMode('latest')
                    }
                  }}
                >
                  <div className="topic-title">
                    {topic.name} {topic.isOfficial ? '✅' : ''}
                  </div>
                  <div className="topic-desc">{topic.description || '暂无描述'}</div>
                  <div className="topic-meta">
                    <span>📝 {topic.postCount}</span>
                    <span>⭐ {topic.followerCount}</span>
                  </div>
                </div>
              ))
            )}
          </div>

          {token ? (
            <div className="post-form">
              <h2>创建话题</h2>
              <input
                value={topicDraft.name}
                onChange={(e) => setTopicDraft((p) => ({ ...p, name: e.target.value }))}
                placeholder="话题名称"
              />
              <input
                value={topicDraft.description}
                onChange={(e) => setTopicDraft((p) => ({ ...p, description: e.target.value }))}
                placeholder="话题描述（可选）"
              />
              <button type="button" className="primary-btn" onClick={handleCreateTopic} disabled={loading}>
                创建话题
              </button>
            </div>
          ) : (
            <div className="secondary-text" style={{ marginTop: '1rem' }}>
              登录后可创建话题与发帖
            </div>
          )}
        </section>

        <section className="community-card">
          <div className="posts-header">
            <div>
              <h2>帖子列表</h2>
              <div className="secondary-text">
                {mode === 'hot'
                  ? '热门帖子（全站）'
                  : mode === 'mine'
                    ? '我的帖子（按最近更新排序）'
                  : activeTopic
                    ? `当前话题：${activeTopic.name}`
                    : '未选择话题'}
              </div>
            </div>
            <div className="inline-actions">
              <select
                value={mode}
                onChange={(e) => setSelectedMode(e.target.value as 'latest' | 'hot' | 'mine')}
              >
                <option value="latest">最新</option>
                <option value="hot">热门</option>
                {token ? <option value="mine">我的帖子</option> : null}
              </select>
              <button type="button" className="secondary-btn" onClick={handleToggleFollow} disabled={!token || loading}>
                {activeTopicId && following.has(activeTopicId) ? '取消关注' : '关注话题'}
              </button>
            </div>
          </div>

          {loading ? (
            <div className="secondary-text">加载中...</div>
          ) : posts.length === 0 ? (
            <div className="community-empty">
              <div className="secondary-text">
                {mode === 'mine' ? '你还没有发布帖子' : '暂无帖子'}
              </div>
              {mode === 'mine' ? (
                <button type="button" className="secondary-btn" onClick={() => setSelectedMode('latest')}>
                  去当前话题看看
                </button>
              ) : null}
            </div>
          ) : (
            <div className="post-list">
              {posts.map((p) => (
                <Link key={p.id} to={`/community/posts/${p.id}`} className="post-item">
                  <div className="post-item-head">
                    <div className="post-title">{p.title}</div>
                    {userId && p.authorId === userId ? <span className="post-badge">我的</span> : null}
                  </div>
                  <div className="post-preview">
                    {p.content.length > 120 ? `${p.content.slice(0, 120)}...` : p.content}
                  </div>
                  <div className="post-meta">
                    <span>👤 {p.authorName || `用户${p.authorId}`}</span>
                    <span>👁️ {p.viewCount}</span>
                    <span>💬 {p.commentCount}</span>
                    <span>👍 {p.likeCount}</span>
                    <span>🔁 {p.shareCount}</span>
                  </div>
                  {userId && p.authorId === userId ? (
                    <div className="post-manage-hint">进入详情可编辑或删除</div>
                  ) : null}
                </Link>
              ))}
            </div>
          )}

          {token ? (
            <div className="post-form">
              <h2>{mode === 'mine' ? '继续发帖' : '在当前话题发帖'}</h2>
              <input
                value={postDraft.title}
                onChange={(e) => setPostDraft((p) => ({ ...p, title: e.target.value }))}
                placeholder="标题"
              />
              <textarea
                value={postDraft.content}
                onChange={(e) => setPostDraft((p) => ({ ...p, content: e.target.value }))}
                placeholder="正文内容"
              />
              <input
                value={postDraft.tags}
                onChange={(e) => setPostDraft((p) => ({ ...p, tags: e.target.value }))}
                placeholder="标签（逗号分隔，可选）"
              />
              <button type="button" className="primary-btn" onClick={handleCreatePost} disabled={loading}>
                发布帖子
              </button>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  )
}

export default CommunityPage
