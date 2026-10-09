import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchGuides,
  fetchLikedPosts,
  fetchMyFavorites,
  fetchPosts,
  type Guide,
  type Post,
} from '../api/client'
import './community.css'
import './profile.css'

interface Props {
  token: string
  userId: number
  nickname?: string
}

// 个人中心（M3 对 mobile 的 web 对等）：我的帖子（community author_id 过滤）、
// 我的攻略（content-service author_id 过滤，带 Bearer 可见自己草稿）、
// 我的收藏（GET /guides/favorites 收藏时间倒序）、
// 我的点赞（GET /users/likes 点赞时间倒序）。
const ProfilePage = ({ token, userId, nickname }: Props) => {
  const [myPosts, setMyPosts] = useState<Post[]>([])
  const [myGuides, setMyGuides] = useState<Guide[]>([])
  const [myFavorites, setMyFavorites] = useState<Guide[]>([])
  const [likedPosts, setLikedPosts] = useState<Post[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    const [postsRes, guidesRes, favsRes, likesRes] = await Promise.allSettled([
      fetchPosts({ authorId: userId, limit: 20 }),
      fetchGuides({ authorId: userId, token, limit: 20 }),
      fetchMyFavorites(token, 20),
      fetchLikedPosts(token, 20),
    ])
    const failures: string[] = []
    if (postsRes.status === 'fulfilled') setMyPosts(postsRes.value.posts)
    else failures.push('我的帖子')
    if (guidesRes.status === 'fulfilled') setMyGuides(guidesRes.value)
    else failures.push('我的攻略')
    if (favsRes.status === 'fulfilled') setMyFavorites(favsRes.value)
    else failures.push('我的收藏')
    if (likesRes.status === 'fulfilled') setLikedPosts(likesRes.value.posts)
    else failures.push('我的点赞')
    setError(failures.length ? `部分内容加载失败：${failures.join('、')}` : null)
    setLoading(false)
  }, [token, userId])

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(load)
  }, [load])

  return (
    <div className="profile-page">
      <div className="profile-header">
        <div>
          <h1>个人中心</h1>
          <p className="secondary-text">👤 {nickname || `用户${userId}`} · 我发布与点赞的内容</p>
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

      <section className="community-card">
        <h2>我的帖子（{myPosts.length}）</h2>
        {loading && myPosts.length === 0 ? (
          <div className="secondary-text">加载中...</div>
        ) : myPosts.length === 0 ? (
          <div className="secondary-text">
            还没有发过帖子，去<Link to="/community">社区</Link>发一帖
          </div>
        ) : (
          <div className="feed-list">
            {myPosts.map((post) => (
              <Link key={post.id} to={`/community/posts/${post.id}`} className="feed-item">
                <div className="feed-item-title">
                  {post.title}
                  {post.status && post.status !== 'published' ? `（${post.status}）` : ''}
                </div>
                <div className="post-meta">
                  <span>👁️ {post.viewCount}</span>
                  <span>💬 {post.commentCount}</span>
                  <span>👍 {post.likeCount}</span>
                  <span>🕒 {new Date(post.updatedAt).toLocaleDateString('zh-CN')}</span>
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="community-card">
        <h2>我的攻略（{myGuides.length}）</h2>
        {loading && myGuides.length === 0 ? (
          <div className="secondary-text">加载中...</div>
        ) : myGuides.length === 0 ? (
          <div className="secondary-text">
            还没有写过攻略，去<Link to="/guides/new">写一篇</Link>
          </div>
        ) : (
          <div className="feed-list">
            {myGuides.map((guide) => (
              <Link key={guide.id} to={`/guides/${guide.id}`} className="feed-item">
                <div className="feed-item-title">
                  {guide.title}
                  {guide.status === 'draft' ? <span className="post-badge">草稿</span> : null}
                </div>
                <div className="post-meta">
                  <span>👁️ {guide.viewCount}</span>
                  <span>👍 {guide.likeCount}</span>
                  <span>🕒 {new Date(guide.updatedAt).toLocaleDateString('zh-CN')}</span>
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="community-card">
        <h2>我的收藏（{myFavorites.length}）</h2>
        {loading && myFavorites.length === 0 ? (
          <div className="secondary-text">加载中...</div>
        ) : myFavorites.length === 0 ? (
          <div className="secondary-text">
            还没有收藏攻略，去<Link to="/guides">攻略区</Link>逛逛
          </div>
        ) : (
          <div className="feed-list">
            {myFavorites.map((guide) => (
              <Link key={guide.id} to={`/guides/${guide.id}`} className="feed-item">
                <div className="feed-item-title">{guide.title}</div>
                <div className="feed-item-summary">
                  👤 {guide.authorName || `用户${guide.authorId}`} · 👍 {guide.likeCount}
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="community-card">
        <h2>我的点赞（{likedPosts.length}）</h2>
        {loading && likedPosts.length === 0 ? (
          <div className="secondary-text">加载中...</div>
        ) : likedPosts.length === 0 ? (
          <div className="secondary-text">还没有点过赞</div>
        ) : (
          <div className="feed-list">
            {likedPosts.map((post) => (
              <Link key={post.id} to={`/community/posts/${post.id}`} className="feed-item">
                <div className="feed-item-title">{post.title}</div>
                <div className="feed-item-summary">
                  👤 {post.authorName || `用户${post.authorId}`} · 👍 {post.likeCount}
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

export default ProfilePage
