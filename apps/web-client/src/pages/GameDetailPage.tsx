import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  addFavorite,
  cancelReservation,
  createGameReview,
  fetchGameById,
  fetchGameReviews,
  fetchGuides,
  fetchGameServers,
  fetchMyFavoriteGameIds,
  fetchMyReservations,
  removeFavorite,
  reportGameReview,
  reserveServer,
  submitGameRating,
  updateGameReview,
  updateGameTags,
  type GameDetail,
  type GameReview,
  type GameServerView,
  type Guide,
} from '../api/client'
import './games.css'

// 游戏详情（/games/:id）：hero + 简介/类型/平台/标签 + 相关攻略，对等 mobile game/[id]。
// token 传入时提供「联运」标签运营维护入口（登录用户按现有写接口口径，角色细化随 GP-103）；
// GP-401 玩家评分/长评/举报/收藏与 GP-306 开服预约按登录态启用。
const GameDetailPage = ({ token, userId }: { token?: string; userId?: number }) => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [savingTag, setSavingTag] = useState(false)
  const [game, setGame] = useState<GameDetail | null>(null)
  const [guides, setGuides] = useState<Guide[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // GP-401：我的评分/收藏与长评列表
  const [myRating, setMyRating] = useState(0)
  const [ratingBusy, setRatingBusy] = useState(false)
  const [ratingError, setRatingError] = useState<string | null>(null)
  const [favorited, setFavorited] = useState(false)
  const [favBusy, setFavBusy] = useState(false)
  const [reviews, setReviews] = useState<GameReview[]>([])
  const [reviewTotal, setReviewTotal] = useState(0)
  const [reviewText, setReviewText] = useState('')
  const [editingReviewId, setEditingReviewId] = useState<number | null>(null)
  const [reviewBusy, setReviewBusy] = useState(false)
  const [reviewError, setReviewError] = useState<string | null>(null)

  // GP-306：开服预约
  const [servers, setServers] = useState<GameServerView[]>([])
  const [reservedIds, setReservedIds] = useState<Set<number>>(new Set())
  const [reserveBusyId, setReserveBusyId] = useState<number | null>(null)
  const [reserveError, setReserveError] = useState<string | null>(null)

  const load = useCallback(async (gameId: string) => {
    setLoading(true)
    setError(null)
    try {
      const detail = await fetchGameById(gameId)
      setGame(detail)
    } catch (err) {
      setError((err as Error).message || '加载游戏详情失败')
    } finally {
      setLoading(false)
    }
  }, [])

  // 攻略区独立加载，失败静默不阻塞主内容（与 mobile 同款策略）
  const loadGuides = useCallback(async (gameId: string) => {
    try {
      const result = await fetchGuides({ gameId, limit: 5 })
      setGuides(result)
    } catch {
      setGuides([])
    }
  }, [])

  // 运营维护：整体覆盖标签，仅切换「联运」一枚，其余标签原样保留
  const [tagError, setTagError] = useState<string | null>(null)
  const toggleCoPublish = useCallback(async () => {
    if (!game || savingTag || !token) return
    const tags = game.tags ?? []
    const next = tags.includes('联运') ? tags.filter((t) => t !== '联运') : [...tags, '联运']
    setSavingTag(true)
    setTagError(null)
    try {
      setGame(await updateGameTags(game.id, next, token))
    } catch (err) {
      setTagError((err as Error).message || '更新标签失败')
    } finally {
      setSavingTag(false)
    }
  }, [game, savingTag, token])

  // GP-401 长评列表（失败静默，与攻略区同策略）
  const loadReviews = useCallback(async (gameId: string) => {
    try {
      const list = await fetchGameReviews(gameId)
      setReviews(list.items)
      setReviewTotal(list.total)
    } catch {
      setReviews([])
      setReviewTotal(0)
    }
  }, [])

  // GP-306 公开开服表
  const loadServers = useCallback(async (gameId: string) => {
    try {
      setServers(await fetchGameServers(gameId))
    } catch {
      setServers([])
    }
  }, [])

  // GP-401 收藏态：经我的收藏列表判定（上限 100，与后端口径一致）
  const loadFavoriteState = useCallback(
    async (gameId: string) => {
      if (!token) return
      try {
        setFavorited((await fetchMyFavoriteGameIds(token)).has(gameId))
      } catch {
        // 静默：收藏态未知时按钮仍可用，操作以服务端为准
      }
    },
    [token],
  )

  // GP-306 我的预约（reservedIds 驱动预约/取消按钮切换）
  const loadMyReservations = useCallback(
    async (gameId: string) => {
      if (!token) return
      try {
        const mine = await fetchMyReservations(gameId, token)
        setReservedIds(new Set(mine.map((r) => r.serverId)))
      } catch {
        setReservedIds(new Set())
      }
    },
    [token],
  )

  const submitMyRating = useCallback(
    async (score: number) => {
      if (!game || ratingBusy || !token) return
      setRatingBusy(true)
      setRatingError(null)
      try {
        const summary = await submitGameRating(game.id, score, token)
        setGame({ ...game, rating: summary })
        setMyRating(score)
      } catch (err) {
        setRatingError((err as Error).message || '评分失败')
      } finally {
        setRatingBusy(false)
      }
    },
    [game, ratingBusy, token],
  )

  const toggleFavorite = useCallback(async () => {
    if (!game || favBusy || !token) return
    setFavBusy(true)
    try {
      if (favorited) {
        await removeFavorite(game.id, token)
        setFavorited(false)
      } else {
        await addFavorite(game.id, token)
        setFavorited(true)
      }
    } catch (err) {
      setReviewError((err as Error).message || '收藏操作失败')
    } finally {
      setFavBusy(false)
    }
  }, [game, favBusy, favorited, token])

  const toggleReserve = useCallback(
    async (serverId: number) => {
      if (!game || reserveBusyId !== null || !token) return
      setReserveBusyId(serverId)
      setReserveError(null)
      try {
        if (reservedIds.has(serverId)) {
          await cancelReservation(game.id, serverId, token)
          setReservedIds((prev) => {
            const next = new Set(prev)
            next.delete(serverId)
            return next
          })
        } else {
          const result = await reserveServer(game.id, serverId, token)
          setReservedIds((prev) => new Set(prev).add(serverId))
          if (!result.created) {
            setReserveError('已预约过该区服')
          }
        }
      } catch (err) {
        setReserveError((err as Error).message || '预约操作失败')
      } finally {
        setReserveBusyId(null)
      }
    },
    [game, reserveBusyId, reservedIds, token],
  )

  // 提交/改评：editingReviewId 非空走 PUT（作者覆盖式），否则 POST（重复 409 提示改评）
  const submitReview = useCallback(async () => {
    if (!game || reviewBusy || !token) return
    const content = reviewText.trim()
    setReviewBusy(true)
    setReviewError(null)
    try {
      if (editingReviewId !== null) {
        const updated = await updateGameReview(game.id, editingReviewId, content, token)
        setReviews((prev) => prev.map((r) => (r.id === updated.id ? updated : r)))
        setEditingReviewId(null)
      } else {
        const created = await createGameReview(game.id, content, token)
        setReviews((prev) => [created, ...prev])
        setReviewTotal((t) => t + 1)
      }
      setReviewText('')
    } catch (err) {
      setReviewError((err as Error).message || '发布长评失败')
    } finally {
      setReviewBusy(false)
    }
  }, [editingReviewId, game, reviewBusy, reviewText, token])

  const reportReview = useCallback(
    async (reviewId: number) => {
      if (!game || !token) return
      const reason = window.prompt('举报理由（1-500 字）')?.trim()
      if (!reason) return
      try {
        await reportGameReview(game.id, reviewId, reason, token)
        window.alert('举报已提交')
      } catch (err) {
        window.alert((err as Error).message || '举报失败')
      }
    },
    [game, token],
  )

  useEffect(() => {
    if (!id) return
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState.
    queueMicrotask(() => {
      load(id)
      loadGuides(id)
      loadReviews(id)
      loadServers(id)
      if (token) {
        loadFavoriteState(id)
        loadMyReservations(id)
      } else {
        setFavorited(false)
        setReservedIds(new Set())
      }
    })
  }, [id, load, loadGuides, loadReviews, loadServers, loadFavoriteState, loadMyReservations, token])

  if (loading && !game) return <div className="games-page">加载中...</div>

  if (error && !game) {
    return (
      <div className="error-page">
        <h1>游戏详情</h1>
        <p>{error}</p>
        <button type="button" onClick={() => id && load(id)}>
          重试
        </button>
        <Link to="/games">返回游戏库</Link>
      </div>
    )
  }

  if (!game) return null

  return (
    <div className="games-page">
      <div className="games-header">
        <div>
          <h1>{game.title}</h1>
          <p className="secondary-text">
            {game.developer || '未知工作室'}
            {game.releaseDate ? ` · ${game.releaseDate}` : ''}
            {game.publisher ? ` · 发行：${game.publisher}` : ''}
          </p>
        </div>
        <div className="inline-actions">
          {token ? (
            <button type="button" className="secondary-btn" disabled={favBusy} onClick={toggleFavorite}>
              {favBusy ? '处理中...' : favorited ? '★ 已收藏' : '☆ 收藏'}
            </button>
          ) : null}
          <Link to="/games" className="secondary-btn">
            ← 返回游戏库
          </Link>
        </div>
      </div>

      <section className="game-detail-hero">
        <div className="game-detail-score">
          <span className="game-detail-score-value">{game.score ? game.score.toFixed(1) : '—'}</span>
          <span className="game-detail-score-label">资料评分</span>
        </div>
        <div className="game-detail-score">
          <span className="game-detail-score-value">
            {game.rating && game.rating.count > 0 ? game.rating.average.toFixed(1) : '—'}
          </span>
          <span className="game-detail-score-label">玩家评分{game.rating ? `（${game.rating.count} 人）` : ''}</span>
        </div>
        {game.coverImage ? (
          <div
            className="game-detail-cover"
            style={{ backgroundImage: `url(${game.coverImage})` }}
            aria-hidden="true"
          />
        ) : null}
      </section>

      {game.description ? (
        <section className="game-detail-section">
          <h2>简介</h2>
          <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{game.description}</p>
        </section>
      ) : null}

      {game.genres.length ? (
        <section className="game-detail-section">
          <h2>类型</h2>
          <div className="game-detail-chips">
            {game.genres.map((genre) => (
              <button key={genre} type="button" className="genre-chip" onClick={() => navigate(`/games?genre=${encodeURIComponent(genre)}`)}>
                {genre}
              </button>
            ))}
          </div>
        </section>
      ) : null}

      {game.platforms.length ? (
        <section className="game-detail-section">
          <h2>平台</h2>
          <div className="game-detail-chips">
            {game.platforms.map((platform) => (
              <span key={platform} className="game-detail-chip game-detail-chip--muted">
                {platform}
              </span>
            ))}
          </div>
        </section>
      ) : null}

      {game.tags?.length || token ? (
        <section className="game-detail-section">
          <h2>标签</h2>
          <div className="game-detail-chips">
            {(game.tags ?? []).map((tag) => (
              <span key={tag} className="game-detail-chip game-detail-chip--accent">
                {tag}
              </span>
            ))}
          </div>
          {token ? (
            <div className="game-detail-tag-admin">
              <button
                type="button"
                className="secondary-btn"
                disabled={savingTag}
                onClick={toggleCoPublish}
              >
                {savingTag ? '保存中...' : game.tags?.includes('联运') ? '取消联运标记' : '标记为联运'}
              </button>
              {tagError ? (
                <span className="secondary-text" role="alert">
                  {tagError}
                </span>
              ) : null}
            </div>
          ) : null}
        </section>
      ) : null}

      {token ? (
        <section className="game-detail-section">
          <h2>我的评分</h2>
          <div className="game-detail-stars" role="radiogroup" aria-label="玩家评分">
            {[1, 2, 3, 4, 5].map((star) => (
              <button
                key={star}
                type="button"
                className={`game-detail-star${myRating >= star ? ' game-detail-star--on' : ''}`}
                disabled={ratingBusy}
                onClick={() => submitMyRating(star)}
              >
                ★
              </button>
            ))}
          </div>
          {ratingError ? (
            <p className="secondary-text" role="alert">
              {ratingError}
            </p>
          ) : null}
        </section>
      ) : null}

      {servers.length ? (
        <section className="game-detail-section">
          <h2>区服与预约</h2>
          <ul className="game-detail-servers">
            {servers.map((server) => (
              <li key={server.id} className="game-detail-server-row">
                <span className="game-detail-server-name">{server.name}</span>
                <span className="secondary-text">
                  {server.region || '—'}
                  {server.status === 'closed' && server.plannedOpenAt ? ` · 排期 ${server.plannedOpenAt.slice(0, 10)}` : ''}
                </span>
                {token ? (
                  <button
                    type="button"
                    className="secondary-btn"
                    disabled={reserveBusyId === server.id}
                    onClick={() => toggleReserve(server.id)}
                  >
                    {reserveBusyId === server.id
                      ? '处理中...'
                      : reservedIds.has(server.id)
                        ? '取消预约'
                        : server.status === 'closed'
                          ? '预约开服'
                          : '预约提醒'}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
          {reserveError ? (
            <p className="secondary-text" role="alert">
              {reserveError}
            </p>
          ) : null}
          {!token ? <p className="secondary-text">登录后可预约未开服区服，开服时收到站内提醒</p> : null}
        </section>
      ) : null}

      {game.playUrl || game.downloadUrl || game.storeLink ? (
        <section className="game-detail-section">
          <h2>入口</h2>
          <div className="game-detail-actions">
            {game.playUrl ? (
              <a className="primary-btn" href={game.playUrl} target="_blank" rel="noopener noreferrer">
                进入游戏
              </a>
            ) : null}
            {game.downloadUrl ? (
              <a className="primary-btn" href={game.downloadUrl} target="_blank" rel="noopener noreferrer">
                下载 Android 包
              </a>
            ) : null}
            {game.storeLink ? (
              <a className="secondary-btn" href={game.storeLink} target="_blank" rel="noopener noreferrer">
                应用商店
              </a>
            ) : null}
          </div>
        </section>
      ) : null}

      {game.screenshots?.length ? (
        <section className="game-detail-section">
          <h2>截图</h2>
          <div className="game-detail-screenshots">
            {game.screenshots.map((src) => (
              <a key={src} href={src} target="_blank" rel="noopener noreferrer" className="game-detail-shot">
                <img src={src} alt="" loading="lazy" />
              </a>
            ))}
          </div>
        </section>
      ) : null}

      {game.videoUrl ? (
        <section className="game-detail-section">
          <h2>宣传视频</h2>
          <a className="secondary-btn" href={game.videoUrl} target="_blank" rel="noopener noreferrer">
            播放视频
          </a>
        </section>
      ) : null}

      {game.changelog ? (
        <section className="game-detail-section">
          <h2>更新日志</h2>
          <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{game.changelog}</p>
        </section>
      ) : null}

      {game.developerWebsite ? (
        <section className="game-detail-section">
          <h2>厂商资料</h2>
          <p className="secondary-text">
            <a href={game.developerWebsite} target="_blank" rel="noopener noreferrer">
              {game.developer || '厂商主页'} →
            </a>
          </p>
        </section>
      ) : null}

      <section className="game-detail-section">
        <div className="game-detail-section-head">
          <h2>攻略</h2>
          <Link
            to={`/guides?gameId=${encodeURIComponent(game.id)}&title=${encodeURIComponent(game.title)}`}
            className="secondary-btn"
          >
            全部攻略 →
          </Link>
        </div>
        {guides.length ? (
          <ul className="game-detail-guides">
            {guides.map((guide) => (
              <li key={guide.id}>
                <Link to={`/guides/${guide.id}`} className="game-detail-guide-row">
                  <span className="game-detail-guide-title">{guide.title}</span>
                  <span className="secondary-text">
                    {guide.authorName || '匿名'} · {guide.viewCount} 次阅读
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="secondary-text">暂无攻略，去社区看看大家的讨论</p>
        )}
      </section>

      <section className="game-detail-section">
        <div className="game-detail-section-head">
          <h2>玩家长评{reviewTotal ? `（${reviewTotal}）` : ''}</h2>
        </div>
        {token ? (
          <div className="game-detail-review-form">
            <textarea
              value={reviewText}
              onChange={(e) => setReviewText(e.target.value)}
              placeholder={editingReviewId !== null ? '修改你的长评（10-4000 字）' : '写下你的长评（10-4000 字）'}
              rows={4}
            />
            <div className="inline-actions">
              <button
                type="button"
                className="primary-btn"
                disabled={reviewBusy || reviewText.trim().length < 10}
                onClick={submitReview}
              >
                {reviewBusy ? '提交中...' : editingReviewId !== null ? '保存修改' : '发布长评'}
              </button>
              {editingReviewId !== null ? (
                <button
                  type="button"
                  className="secondary-btn"
                  onClick={() => {
                    setEditingReviewId(null)
                    setReviewText('')
                  }}
                >
                  取消编辑
                </button>
              ) : null}
            </div>
            {reviewError ? (
              <p className="secondary-text" role="alert">
                {reviewError}
              </p>
            ) : null}
          </div>
        ) : (
          <p className="secondary-text">登录后可写长评</p>
        )}
        {reviews.length ? (
          <ul className="game-detail-reviews">
            {reviews.map((review) => (
              <li key={review.id} className="game-detail-review">
                <div className="game-detail-review-head">
                  <span className="game-detail-review-author">玩家 #{review.userId}</span>
                  {review.rating > 0 ? <span className="game-detail-review-rating">★ {review.rating}/5</span> : null}
                  <span className="secondary-text">{review.updatedAt?.slice(0, 10)}</span>
                </div>
                <p style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7, margin: 0 }}>{review.content}</p>
                {review.devReply ? (
                  <blockquote className="game-detail-review-reply">
                    <span className="secondary-text">开发者回复：</span>
                    {review.devReply}
                  </blockquote>
                ) : null}
                <div className="inline-actions">
                  {userId !== undefined && review.userId === userId ? (
                    <button
                      type="button"
                      className="secondary-btn"
                      onClick={() => {
                        setEditingReviewId(review.id)
                        setReviewText(review.content)
                      }}
                    >
                      编辑
                    </button>
                  ) : null}
                  {token ? (
                    <button type="button" className="secondary-btn" onClick={() => reportReview(review.id)}>
                      举报
                    </button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p className="secondary-text">暂无长评，来写第一条</p>
        )}
      </section>

      <section className="game-detail-section">
        <h2>热度</h2>
        <p className="secondary-text">🔥 trending score {game.trendingScore ?? 0}</p>
      </section>
    </div>
  )
}

export default GameDetailPage
