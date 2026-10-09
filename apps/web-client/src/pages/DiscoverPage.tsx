import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { fetchHomeFeed, type HomeFeed } from '../api/client'
// 复用 community.css 的 community-card/post-meta/secondary-btn/error-banner 通用样式。
import './community.css'
import './discover.css'

const FEED_LIMIT = 8

// 发现页（BFF 聚合端点 GET /home/feed 的 web 消费点，对等 mobile 发现 Tab）：
// 一次请求渲染精选游戏/热帖/话题/攻略四区块；单组上游故障时网关降级为空组
// 并记入 degraded，此处顶部给降级提示条，不整页报错。
const DEGRADED_LABELS: Record<string, string> = {
  games: '精选游戏',
  posts: '热帖',
  topics: '话题',
  guides: '攻略',
}

const emptyFeed = (): HomeFeed => ({
  featured_games: [],
  hot_posts: [],
  topics: [],
  guides: [],
  degraded: [],
})

const DiscoverPage = () => {
  const [feed, setFeed] = useState<HomeFeed | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchHomeFeed(FEED_LIMIT)
      setFeed(data)
    } catch (err) {
      setError((err as Error).message || '加载发现页失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(load)
  }, [load])

  const data = feed ?? emptyFeed()
  const degradedNames = data.degraded
    .map((key) => DEGRADED_LABELS[key] ?? key)
    .join('、')

  return (
    <div className="discover-page">
      <div className="discover-header">
        <div>
          <h1>发现</h1>
          <p className="secondary-text">精选游戏 / 热帖 / 话题 / 攻略，一次请求聚合（api-gateway）</p>
        </div>
        <button type="button" className="secondary-btn" onClick={load} disabled={loading}>
          {loading ? '刷新中...' : '刷新'}
        </button>
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : null}

      {degradedNames ? (
        <div className="degraded-banner" role="status">
          「{degradedNames}」分组暂时不可用（上游服务降级），稍后刷新重试
        </div>
      ) : null}

      <div className="discover-grid">
        <section className="community-card">
          <h2>精选游戏</h2>
          {data.featured_games.length === 0 ? (
            <div className="secondary-text">暂无精选</div>
          ) : (
            <div className="feed-game-list">
              {data.featured_games.map((game) => (
                <Link key={game.id} to={`/games/${game.id}`} className="feed-game">
                  <div
                    className="feed-game-cover"
                    style={{
                      backgroundImage: game.cover_image ? `url(${game.cover_image})` : undefined,
                    }}
                    aria-hidden="true"
                  />
                  <div className="feed-game-main">
                    <div className="feed-game-title">{game.title}</div>
                    <div className="feed-game-meta">
                      <span>{game.genres.join(' / ') || '未知类型'}</span>
                      <span className="feed-score">{game.score > 0 ? game.score.toFixed(1) : '—'}</span>
                    </div>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="community-card">
          <h2>热帖</h2>
          {data.hot_posts.length === 0 ? (
            <div className="secondary-text">暂无热帖</div>
          ) : (
            <div className="feed-list">
              {data.hot_posts.map((post) => (
                <Link key={post.id} to={`/community/posts/${post.id}`} className="feed-item">
                  <div className="feed-item-title">{post.title}</div>
                  <div className="feed-item-summary">{post.summary}</div>
                  <div className="post-meta">
                    <span>👤 {post.author_name || `用户${post.author_id}`}</span>
                    <span>💬 {post.comment_count}</span>
                    <span>👍 {post.like_count}</span>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="community-card">
          <h2>话题圈子</h2>
          {data.topics.length === 0 ? (
            <div className="secondary-text">暂无话题</div>
          ) : (
            <div className="feed-list">
              {data.topics.map((topic) => (
                <Link key={topic.id} to="/community" className="feed-item">
                  <div className="feed-item-title">
                    {topic.name} {topic.is_official ? '✅' : ''}
                  </div>
                  <div className="feed-item-summary">
                    📝 {topic.post_count} 帖 · ⭐ {topic.follower_count} 关注
                  </div>
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="community-card">
          <h2>最新攻略</h2>
          {data.guides.length === 0 ? (
            <div className="secondary-text">暂无攻略</div>
          ) : (
            <div className="feed-list">
              {data.guides.map((guide) => (
                <Link key={guide.id} to={`/guides/${guide.id}`} className="feed-item">
                  <div className="feed-item-title">{guide.title}</div>
                  <div className="feed-item-summary">
                    🎮 {guide.game_title} · 👤 {guide.author_name || '匿名'}
                  </div>
                  <div className="post-meta">
                    <span>👁️ {guide.views}</span>
                    <span>👍 {guide.likes}</span>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

export default DiscoverPage
