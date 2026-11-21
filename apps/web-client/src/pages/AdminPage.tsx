import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  assignOwnership,
  createPartner,
  createPromoChannel,
  decideReview,
  fetchAnalyticsActivityEffect,
  fetchAnalyticsChannels,
  fetchAnalyticsFunnel,
  fetchAnalyticsOverview,
  fetchAnalyticsReconciliation,
  fetchAnalyticsRetention,
  fetchAuditLogs,
  fetchGameVersions,
  fetchGames,
  fetchPartners,
  fetchPromoChannels,
  fetchPromoChannelStats,
  fetchReviewReports,
  fetchReviews,
  grantRole,
  handleReviewReport,
  publishGame,
  publishVersion,
  restoreReview,
  revokeVersion,
  setPartnerStatus,
  setPromoChannelStatus,
  unpublishGame,
  updatePromoChannel,
  type AdminGame,
  type AnalyticsChannelRow,
  type AnalyticsFunnel,
  type AnalyticsOverview,
  type AnalyticsReconciliationRow,
  type AnalyticsRetention,
  type AuditLogInfo,
  type GameReviewInfo,
  type GameVersionInfo,
  type PartnerInfo,
  type PromoChannel,
  type PromoChannelStats,
  type ReviewReportInfo,
} from '../api/client'
import './admin.css'

type Tab = 'reviews' | 'reports' | 'partners' | 'games' | 'audit' | 'analytics' | 'promo'

interface Props {
  token: string
  isAdmin: boolean // platform_admin 可授予角色；operator 仅日常审核/合作方
}

const STATUS_LABELS: Record<string, string> = {
  draft: '草稿',
  pending: '待审核',
  approved: '已过审',
  published: '已发布',
  revoked: '已撤回',
  rejected: '已驳回',
  offline: '已下架',
  active: '正常',
  suspended: '已挂起',
}

function StatusBadge({ status }: { status: string }) {
  const cls = status in STATUS_LABELS ? `status-${status}` : 'status-draft'
  return <span className={`status-badge ${cls}`}>{STATUS_LABELS[status] ?? status}</span>
}

// GP-401 长评举报状态：pending 待处置 / dismissed 已驳回（评价保留）/
// resolved 已移除（评价软删，可恢复）。与提审状态共用 badge 样式但语义不同，
// 单独一张映射避免文案混淆。
const REPORT_STATUS_LABELS: Record<string, string> = {
  pending: '待处置',
  dismissed: '已驳回',
  resolved: '已移除',
}

const REPORT_STATUS_CLASS: Record<string, string> = {
  pending: 'status-pending',
  dismissed: 'status-rejected',
  resolved: 'status-offline',
}

function ReportStatusBadge({ status }: { status: string }) {
  const cls = REPORT_STATUS_CLASS[status] ?? 'status-draft'
  return <span className={`status-badge ${cls}`}>{REPORT_STATUS_LABELS[status] ?? status}</span>
}

function formatDate(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

// 运营后台（GP-103 最小面）：审核队列 / 合作方与归属 / 游戏上下架 / 审计。
// 数据经网关反代 game-catalog（/admin/*），角色守卫在服务端（player 403）。
const AdminPage = ({ token, isAdmin }: Props) => {
  const [tab, setTab] = useState<Tab>('reviews')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  // 审核队列
  const [reviews, setReviews] = useState<GameReviewInfo[]>([])
  const [reviewTotal, setReviewTotal] = useState(0)
  const [reviewFilter, setReviewFilter] = useState('')
  const [rejecting, setRejecting] = useState<number | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  // GP-401 长评举报队列：status 过滤（''=全部），remove/restore 破坏性操作两步确认
  const [reports, setReports] = useState<ReviewReportInfo[]>([])
  const [reportTotal, setReportTotal] = useState(0)
  const [reportFilter, setReportFilter] = useState('')
  const [reportConfirm, setReportConfirm] = useState<{ id: number; action: 'remove' | 'restore' } | null>(null)
  // 本次会话内已恢复的评价 id（举报状态仍为 resolved，据此隐藏恢复按钮）
  const [restoredReviewIds, setRestoredReviewIds] = useState<Set<number>>(new Set())

  // 合作方
  const [partners, setPartners] = useState<PartnerInfo[]>([])
  const [partnerName, setPartnerName] = useState('')
  const [partnerEmail, setPartnerEmail] = useState('')

  // 角色授予
  const [roleUserId, setRoleUserId] = useState('')
  const [roleName, setRoleName] = useState('operator')
  const [rolePartnerId, setRolePartnerId] = useState('')

  // 归属指派
  const [ownGameId, setOwnGameId] = useState('')
  const [ownPartnerId, setOwnPartnerId] = useState('')

  // 游戏上下架
  const [games, setGames] = useState<AdminGame[]>([])
  const [gameKeyword, setGameKeyword] = useState('')
  const [actionGameId, setActionGameId] = useState('')

  // 版本管理（GP-104）：按游戏查版本，发布/撤回
  const [versionGameId, setVersionGameId] = useState('')
  const [adminVersions, setAdminVersions] = useState<GameVersionInfo[]>([])

  // 审计
  const [logs, setLogs] = useState<AuditLogInfo[]>([])
  const [logTotal, setLogTotal] = useState(0)

  const run = useCallback(async (fn: () => Promise<void>) => {
    setLoading(true)
    setError(null)
    setNotice(null)
    try {
      await fn()
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const loadReviews = useCallback(
    (status: string) => {
      run(async () => {
        const res = await fetchReviews({ status: status || undefined, limit: 50 }, token)
        setReviews(res.reviews)
        setReviewTotal(res.total)
      })
    },
    [run, token],
  )

  // GP-401 举报队列（含已处置视图，resolved 行提供恢复评价入口）
  const loadReports = useCallback(
    (status: string) => {
      run(async () => {
        const res = await fetchReviewReports({ status: status || undefined, limit: 50 }, token)
        setReports(res.items)
        setReportTotal(res.total)
      })
    },
    [run, token],
  )

  const loadPartners = useCallback(() => {
    run(async () => {
      setPartners(await fetchPartners(token))
    })
  }, [run, token])

  const loadGames = useCallback(
    (keyword: string) => {
      run(async () => {
        const res = await fetchGames({ keyword: keyword || undefined, limit: 50 })
        setGames(
          res.games.map((g) => ({
            ...g,
            status: 'approved',
            reviewReason: '',
          })),
        )
      })
    },
    [run],
  )

  const loadAudit = useCallback(() => {
    run(async () => {
      const res = await fetchAuditLogs({ limit: 50 }, token)
      setLogs(res.logs)
      setLogTotal(res.total)
    })
  }, [run, token])

  // GP-402 经营分析：一次拉取五张面（总览/漏斗/留存/对账/渠道），窗口与游戏过滤共用
  const [anFrom, setAnFrom] = useState('')
  const [anTo, setAnTo] = useState('')
  const [anGameId, setAnGameId] = useState('')
  const [overview, setOverview] = useState<AnalyticsOverview | null>(null)
  const [funnel, setFunnel] = useState<AnalyticsFunnel | null>(null)
  const [retention, setRetention] = useState<AnalyticsRetention | null>(null)
  const [reconciliation, setReconciliation] = useState<AnalyticsReconciliationRow[]>([])
  const [channels, setChannels] = useState<AnalyticsChannelRow[]>([])
  const [anActivityId, setAnActivityId] = useState('')
  const [activityEffect, setActivityEffect] = useState<string | null>(null)
  // GP-403 渠道推广（F22）：渠道登记/启停/效果计数 + 推广链接生成器
  const [promoChannels, setPromoChannels] = useState<PromoChannel[]>([])
  const [promoCode, setPromoCode] = useState('')
  const [promoName, setPromoName] = useState('')
  const [promoWindow, setPromoWindow] = useState('72')
  const [promoEditId, setPromoEditId] = useState<number | null>(null)
  const [promoEditName, setPromoEditName] = useState('')
  const [promoEditWindow, setPromoEditWindow] = useState('')
  const [promoStatsCode, setPromoStatsCode] = useState('')
  const [promoStats, setPromoStats] = useState<PromoChannelStats | null>(null)
  const [promoLinkCode, setPromoLinkCode] = useState('')
  const [promoLinkGameId, setPromoLinkGameId] = useState('')
  const [promoLink, setPromoLink] = useState('')

  const loadAnalytics = useCallback(() => {
    run(async () => {
      const window = {
        from: anFrom.trim() || undefined,
        to: anTo.trim() || undefined,
        gameId: anGameId.trim() || undefined,
      }
      const [ov, fn, rt, rc, ch] = await Promise.all([
        fetchAnalyticsOverview(window, token),
        fetchAnalyticsFunnel(window, token),
        fetchAnalyticsRetention({ from: window.from, to: window.to }, token),
        fetchAnalyticsReconciliation({ from: window.from, to: window.to }, token),
        fetchAnalyticsChannels(window, token),
      ])
      setOverview(ov)
      setFunnel(fn)
      setRetention(rt)
      setReconciliation(rc)
      setChannels(ch)
    })
  }, [anFrom, anGameId, anTo, run, token])

  const loadActivityEffect = useCallback(() => {
    const id = anActivityId.trim()
    if (!id) return
    run(async () => {
      const effect = await fetchAnalyticsActivityEffect(id, token)
      setActivityEffect(
        `活动 ${effect.activityId}：发放 ${effect.grants} 笔 · 参与 ${effect.participants} 人 · 带动 GMV ¥${(effect.rewardCents / 100).toFixed(2)}`,
      )
    })
  }, [anActivityId, run, token])

  const loadAdminVersions = useCallback(
    (gameId: string) => {
      if (!gameId.trim()) {
        setAdminVersions([])
        return
      }
      run(async () => {
        setAdminVersions(await fetchGameVersions(gameId.trim(), token))
      })
    },
    [run, token],
  )

  const loadPromoChannels = useCallback(() => {
    run(async () => {
      const { items } = await fetchPromoChannels({ limit: 50 }, token)
      setPromoChannels(items)
    })
  }, [run, token])

  const submitPromoChannel = useCallback(() => {
    if (!promoCode.trim() || !promoName.trim()) return
    run(async () => {
      const created = await createPromoChannel(
        { code: promoCode.trim(), name: promoName.trim(), windowHours: Number(promoWindow) || 0 },
        token,
      )
      const { items } = await fetchPromoChannels({ limit: 50 }, token)
      setPromoChannels(items)
      setPromoCode('')
      setPromoName('')
      setNotice(`渠道 ${created.code} 已登记`)
    })
  }, [promoCode, promoName, promoWindow, run, token])

  const togglePromoChannel = useCallback(
    (channel: PromoChannel) => {
      const next = channel.status === 'disabled' ? 'active' : 'disabled'
      run(async () => {
        await setPromoChannelStatus(channel.id, next, token)
        const { items } = await fetchPromoChannels({ limit: 50 }, token)
        setPromoChannels(items)
      })
    },
    [run, token],
  )

  const submitPromoEdit = useCallback(
    (channel: PromoChannel) => {
      run(async () => {
        await updatePromoChannel(
          channel.id,
          { name: promoEditName.trim(), windowHours: Number(promoEditWindow) || 0 },
          token,
        )
        const { items } = await fetchPromoChannels({ limit: 50 }, token)
        setPromoChannels(items)
        setPromoEditId(null)
      })
    },
    [promoEditName, promoEditWindow, run, token],
  )

  const queryPromoStats = useCallback(() => {
    const code = promoStatsCode.trim()
    if (!code) return
    run(async () => {
      const stats = await fetchPromoChannelStats(
        code,
        { from: anFrom.trim() || undefined, to: anTo.trim() || undefined },
        token,
      )
      setPromoStats(stats)
    })
  }, [anFrom, anTo, promoStatsCode, run, token])

  const generatePromoLink = useCallback(() => {
    const code = promoLinkCode.trim()
    if (!code) return
    const clickId =
      typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`
    const query = new URLSearchParams({ channel: code, click_id: clickId })
    const gameId = promoLinkGameId.trim()
    setPromoLink(`${window.location.origin}${gameId ? `/games/${encodeURIComponent(gameId)}` : ''}?${query.toString()}`)
  }, [promoLinkCode, promoLinkGameId])

  useEffect(() => {
    queueMicrotask(() => {
      if (tab === 'reviews') loadReviews(reviewFilter)
      else if (tab === 'reports') loadReports(reportFilter)
      else if (tab === 'partners') loadPartners()
      else if (tab === 'games') loadGames(gameKeyword)
      else if (tab === 'analytics') loadAnalytics()
      else if (tab === 'promo') loadPromoChannels()
      else loadAudit()
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  return (
    <div className="admin-page">
      <div className="admin-header">
        <div>
          <h1>运营后台</h1>
          <p className="admin-meta">审核 / 长评举报 / 合作方 / 上下架 / 审计（{isAdmin ? '管理员' : '运营'}）</p>
        </div>
        <div className="admin-tabs">
          <button type="button" className={`admin-tab${tab === 'reviews' ? ' is-active' : ''}`} onClick={() => setTab('reviews')}>
            审核队列（{reviewTotal}）
          </button>
          <button type="button" className={`admin-tab${tab === 'reports' ? ' is-active' : ''}`} onClick={() => setTab('reports')}>
            长评举报（{reportTotal}）
          </button>
          <button type="button" className={`admin-tab${tab === 'partners' ? ' is-active' : ''}`} onClick={() => setTab('partners')}>
            合作方与角色
          </button>
          <button type="button" className={`admin-tab${tab === 'games' ? ' is-active' : ''}`} onClick={() => setTab('games')}>
            游戏上下架
          </button>
          <button type="button" className={`admin-tab${tab === 'audit' ? ' is-active' : ''}`} onClick={() => setTab('audit')}>
            审计日志（{logTotal}）
          </button>
          <button type="button" className={`admin-tab${tab === 'analytics' ? ' is-active' : ''}`} onClick={() => setTab('analytics')}>
            经营分析
          </button>
          <button type="button" className={`admin-tab${tab === 'promo' ? ' is-active' : ''}`} onClick={() => setTab('promo')}>
            渠道推广（{promoChannels.length}）
          </button>
        </div>
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : null}
      {notice ? <div className="notice-banner">{notice}</div> : null}

      {tab === 'reviews' ? (
        <section className="community-card">
          <h2>提审队列</h2>
          <div className="admin-form" style={{ marginBottom: '0.75rem' }}>
            <select value={reviewFilter} onChange={(e) => setReviewFilter(e.target.value)}>
              <option value="">全部状态</option>
              <option value="pending">待审核</option>
              <option value="approved">已过审</option>
              <option value="rejected">已驳回</option>
            </select>
            <button type="button" className="secondary-btn" onClick={() => loadReviews(reviewFilter)} disabled={loading}>
              筛选
            </button>
          </div>
          {reviews.length === 0 ? (
            <div className="admin-meta">暂无提审记录</div>
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>类型</th>
                    <th>游戏</th>
                    <th>提审人</th>
                    <th>状态</th>
                    <th>审核原因</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {reviews.map((r) => (
                    <tr key={r.id}>
                      <td>{r.id}</td>
                      <td>{r.versionId ? '版本提审' : '资料提审'}</td>
                      <td>
                        {r.gameId}
                        {r.versionId ? ` / ${r.versionId}` : ''}
                      </td>
                      <td>#{r.submittedBy}</td>
                      <td>
                        <StatusBadge status={r.status} />
                      </td>
                      <td>{r.reviewReason || '—'}</td>
                      <td>
                        {r.status === 'pending' ? (
                          <div className="inline-actions">
                            <button
                              type="button"
                              className="secondary-btn"
                              disabled={loading}
                              onClick={() =>
                                run(async () => {
                                  await decideReview(r.id, 'approve', '', token)
                                  setNotice(`已通过 #${r.id}`)
                                  loadReviews(reviewFilter)
                                })
                              }
                            >
                              通过
                            </button>
                            {rejecting === r.id ? (
                              <>
                                <input
                                  className="reason-input"
                                  placeholder="驳回原因（必填）"
                                  value={rejectReason}
                                  onChange={(e) => setRejectReason(e.target.value)}
                                />
                                <button
                                  type="button"
                                  className="secondary-btn danger-btn"
                                  disabled={loading || !rejectReason.trim()}
                                  onClick={() =>
                                    run(async () => {
                                      await decideReview(r.id, 'reject', rejectReason.trim(), token)
                                      setRejecting(null)
                                      setRejectReason('')
                                      setNotice(`已驳回 #${r.id}`)
                                      loadReviews(reviewFilter)
                                    })
                                  }
                                >
                                  确认驳回
                                </button>
                              </>
                            ) : (
                              <button
                                type="button"
                                className="secondary-btn danger-btn"
                                onClick={() => {
                                  setRejecting(r.id)
                                  setRejectReason('')
                                }}
                              >
                                驳回
                              </button>
                            )}
                          </div>
                        ) : (
                          <span className="admin-meta">{formatDate(r.reviewedAt)}</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      ) : null}

      {tab === 'reports' ? (
        <section className="community-card">
          <h2>长评举报</h2>
          <div className="admin-form" style={{ marginBottom: '0.75rem' }}>
            <select value={reportFilter} onChange={(e) => setReportFilter(e.target.value)}>
              <option value="">全部状态</option>
              <option value="pending">待处置</option>
              <option value="dismissed">已驳回</option>
              <option value="resolved">已移除</option>
            </select>
            <button type="button" className="secondary-btn" onClick={() => loadReports(reportFilter)} disabled={loading}>
              筛选
            </button>
          </div>
          {reports.length === 0 ? (
            <div className="admin-meta">暂无举报记录</div>
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>游戏</th>
                    <th>评价</th>
                    <th>举报人</th>
                    <th>理由</th>
                    <th>状态</th>
                    <th>处置</th>
                  </tr>
                </thead>
                <tbody>
                  {reports.map((r) => (
                    <tr key={r.id}>
                      <td>{r.id}</td>
                      <td>{r.gameId}</td>
                      <td>
                        <Link to={`/games/${encodeURIComponent(r.gameId)}`}>评价 #{r.reviewId}</Link>
                      </td>
                      <td>#{r.reporterId}</td>
                      <td>{r.reason}</td>
                      <td>
                        <ReportStatusBadge status={r.status} />
                      </td>
                      <td>
                        {r.status === 'pending' ? (
                          <div className="inline-actions">
                            <button
                              type="button"
                              className="secondary-btn"
                              disabled={loading}
                              onClick={() =>
                                run(async () => {
                                  await handleReviewReport(r.id, 'dismiss', token)
                                  setNotice(`已驳回举报 #${r.id}（评价保留）`)
                                  loadReports(reportFilter)
                                })
                              }
                            >
                              驳回
                            </button>
                            {reportConfirm?.id === r.id && reportConfirm.action === 'remove' ? (
                              <>
                                <button
                                  type="button"
                                  className="secondary-btn danger-btn"
                                  disabled={loading}
                                  onClick={() =>
                                    run(async () => {
                                      await handleReviewReport(r.id, 'remove', token)
                                      setReportConfirm(null)
                                      setRestoredReviewIds((prev) => {
                                        const next = new Set(prev)
                                        next.delete(r.reviewId)
                                        return next
                                      })
                                      setNotice(`已移除评价 #${r.reviewId}（同评其余举报一并结案）`)
                                      loadReports(reportFilter)
                                    })
                                  }
                                >
                                  确认移除
                                </button>
                                <button type="button" className="secondary-btn" onClick={() => setReportConfirm(null)}>
                                  取消
                                </button>
                              </>
                            ) : (
                              <button
                                type="button"
                                className="secondary-btn danger-btn"
                                onClick={() => setReportConfirm({ id: r.id, action: 'remove' })}
                              >
                                移除评价
                              </button>
                            )}
                          </div>
                        ) : r.status === 'resolved' && !restoredReviewIds.has(r.reviewId) ? (
                          <div className="inline-actions">
                            {reportConfirm?.id === r.id && reportConfirm.action === 'restore' ? (
                              <>
                                <button
                                  type="button"
                                  className="secondary-btn"
                                  disabled={loading}
                                  onClick={() =>
                                    run(async () => {
                                      const review = await restoreReview(r.reviewId, token)
                                      setReportConfirm(null)
                                      setRestoredReviewIds((prev) => new Set(prev).add(review.id))
                                      setNotice(`评价 #${review.id} 已恢复（已通知作者）`)
                                      loadReports(reportFilter)
                                    })
                                  }
                                >
                                  确认恢复
                                </button>
                                <button type="button" className="secondary-btn" onClick={() => setReportConfirm(null)}>
                                  取消
                                </button>
                              </>
                            ) : (
                              <button
                                type="button"
                                className="secondary-btn"
                                onClick={() => setReportConfirm({ id: r.id, action: 'restore' })}
                              >
                                恢复评价
                              </button>
                            )}
                            <span className="admin-meta">{r.handledBy ? `处置 #${r.handledBy}` : ''}</span>
                          </div>
                        ) : (
                          <span className="admin-meta">
                            {restoredReviewIds.has(r.reviewId) ? '已恢复' : r.handledBy ? `处置 #${r.handledBy}` : ''}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      ) : null}

      {tab === 'partners' ? (
        <>
          <section className="community-card">
            <h2>新建合作方</h2>
            <div className="admin-form">
              <input placeholder="合作方名称" value={partnerName} onChange={(e) => setPartnerName(e.target.value)} />
              <input placeholder="联系邮箱（可选）" value={partnerEmail} onChange={(e) => setPartnerEmail(e.target.value)} />
              <button
                type="button"
                className="secondary-btn"
                disabled={loading || !partnerName.trim()}
                onClick={() =>
                  run(async () => {
                    const p = await createPartner({ name: partnerName.trim(), contactEmail: partnerEmail.trim() || undefined }, token)
                    setNotice(`已创建合作方「${p.name}」(#${p.id})`)
                    setPartnerName('')
                    setPartnerEmail('')
                    loadPartners()
                  })
                }
              >
                创建
              </button>
            </div>
          </section>

          {isAdmin ? (
            <section className="community-card">
              <h2>授予平台角色</h2>
              <div className="admin-form">
                <input type="number" placeholder="用户 ID" value={roleUserId} onChange={(e) => setRoleUserId(e.target.value)} />
                <select value={roleName} onChange={(e) => setRoleName(e.target.value)}>
                  <option value="operator">operator（运营）</option>
                  <option value="partner">partner（合作方）</option>
                  <option value="player">player（玩家）</option>
                </select>
                {roleName === 'partner' ? (
                  <input type="number" placeholder="合作方 ID" value={rolePartnerId} onChange={(e) => setRolePartnerId(e.target.value)} />
                ) : null}
                <button
                  type="button"
                  className="secondary-btn"
                  disabled={loading || !roleUserId}
                  onClick={() =>
                    run(async () => {
                      await grantRole(
                        {
                          userId: Number(roleUserId),
                          role: roleName,
                          partnerId: roleName === 'partner' ? Number(rolePartnerId) || 0 : 0,
                        },
                        token,
                      )
                      setNotice(`已授予用户 #${roleUserId} 角色 ${roleName}`)
                      setRoleUserId('')
                      setRolePartnerId('')
                    })
                  }
                >
                  授予
                </button>
              </div>
            </section>
          ) : null}

          <section className="community-card">
            <h2>指派游戏归属</h2>
            <div className="admin-form">
              <input placeholder="游戏 ID（如 game-elden-ring）" value={ownGameId} onChange={(e) => setOwnGameId(e.target.value)} />
              <input type="number" placeholder="合作方 ID" value={ownPartnerId} onChange={(e) => setOwnPartnerId(e.target.value)} />
              <button
                type="button"
                className="secondary-btn"
                disabled={loading || !ownGameId.trim() || !ownPartnerId}
                onClick={() =>
                  run(async () => {
                    await assignOwnership(ownGameId.trim(), Number(ownPartnerId), token)
                    setNotice(`已将 ${ownGameId.trim()} 指派给合作方 #${ownPartnerId}`)
                    setOwnGameId('')
                  })
                }
              >
                指派
              </button>
            </div>
          </section>

          <section className="community-card">
            <h2>合作方列表（{partners.length}）</h2>
            {partners.length === 0 ? (
              <div className="admin-meta">暂无合作方</div>
            ) : (
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>名称</th>
                      <th>联系邮箱</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {partners.map((p) => (
                      <tr key={p.id}>
                        <td>{p.id}</td>
                        <td>{p.name}</td>
                        <td>{p.contactEmail || '—'}</td>
                        <td>
                          <StatusBadge status={p.status} />
                        </td>
                        <td>{formatDate(p.createdAt)}</td>
                        <td>
                          <button
                            type="button"
                            className="secondary-btn"
                            disabled={loading}
                            onClick={() =>
                              run(async () => {
                                const next = await setPartnerStatus(p.id, p.status === 'active' ? 'suspended' : 'active', token)
                                setNotice(`合作方「${next.name}」已${next.status === 'active' ? '启用' : '挂起'}`)
                                loadPartners()
                              })
                            }
                          >
                            {p.status === 'active' ? '挂起' : '启用'}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      ) : null}

      {tab === 'games' ? (
        <>
          <section className="community-card">
            <h2>版本发布/撤回（GP-104）</h2>
            <div className="admin-form">
              <input placeholder="游戏 ID" value={versionGameId} onChange={(e) => setVersionGameId(e.target.value)} />
              <button type="button" className="secondary-btn" onClick={() => loadAdminVersions(versionGameId)} disabled={loading}>
                查询版本
              </button>
            </div>
            {adminVersions.length > 0 ? (
              <div className="admin-table-wrap" style={{ marginTop: '0.75rem' }}>
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>版本</th>
                      <th>包名 / code</th>
                      <th>大小</th>
                      <th>哈希</th>
                      <th>状态</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {adminVersions.map((v) => (
                      <tr key={v.id}>
                        <td>{v.version}</td>
                        <td>
                          {v.packageName || '—'}
                          {v.versionCode ? <div className="admin-meta">code {v.versionCode}</div> : null}
                        </td>
                        <td>{v.sizeBytes > 0 ? `${(v.sizeBytes / 1024 / 1024).toFixed(1)} MB` : '—'}</td>
                        <td>{v.hash || '—'}</td>
                        <td>
                          <StatusBadge status={v.status} />
                        </td>
                        <td>
                          <div className="inline-actions">
                            <button
                              type="button"
                              className="secondary-btn"
                              disabled={loading || v.status !== 'approved'}
                              onClick={() =>
                                run(async () => {
                                  await publishVersion(versionGameId.trim(), v.id, token)
                                  setNotice(`版本 ${v.version} 已发布`)
                                  loadAdminVersions(versionGameId)
                                })
                              }
                            >
                              发布
                            </button>
                            <button
                              type="button"
                              className="secondary-btn danger-btn"
                              disabled={loading || v.status !== 'published'}
                              onClick={() =>
                                run(async () => {
                                  await revokeVersion(versionGameId.trim(), v.id, token)
                                  setNotice(`版本 ${v.version} 已撤回`)
                                  loadAdminVersions(versionGameId)
                                })
                              }
                            >
                              撤回
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : null}
          </section>

          <section className="community-card">
            <h2>按游戏 ID 操作</h2>
            <div className="admin-form">
              <input placeholder="游戏 ID" value={actionGameId} onChange={(e) => setActionGameId(e.target.value)} />
              <button
                type="button"
                className="secondary-btn"
                disabled={loading || !actionGameId.trim()}
                onClick={() =>
                  run(async () => {
                    await publishGame(actionGameId.trim(), token)
                    setNotice(`已上架 ${actionGameId.trim()}`)
                  })
                }
              >
                上架
              </button>
              <button
                type="button"
                className="secondary-btn danger-btn"
                disabled={loading || !actionGameId.trim()}
                onClick={() =>
                  run(async () => {
                    await unpublishGame(actionGameId.trim(), token)
                    setNotice(`已下架 ${actionGameId.trim()}`)
                  })
                }
              >
                下架
              </button>
            </div>
            <p className="admin-meta" style={{ marginTop: '0.5rem' }}>
              公开目录仅展示已过审游戏；下架/待审游戏可通过 ID 直接操作。
            </p>
          </section>

          <section className="community-card">
            <h2>已上架游戏（搜索）</h2>
            <div className="admin-form" style={{ marginBottom: '0.75rem' }}>
              <input placeholder="关键词" value={gameKeyword} onChange={(e) => setGameKeyword(e.target.value)} />
              <button type="button" className="secondary-btn" onClick={() => loadGames(gameKeyword)} disabled={loading}>
                搜索
              </button>
            </div>
            {games.length === 0 ? (
              <div className="admin-meta">
                暂无结果，可到<Link to="/games">游戏库</Link>查看
              </div>
            ) : (
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>游戏</th>
                      <th>平台</th>
                      <th>状态</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {games.map((g) => (
                      <tr key={g.id}>
                        <td>{g.title}</td>
                        <td>{g.platforms.join(' / ')}</td>
                        <td>
                          <StatusBadge status={g.status} />
                        </td>
                        <td>
                          <button
                            type="button"
                            className="secondary-btn danger-btn"
                            disabled={loading}
                            onClick={() =>
                              run(async () => {
                                await unpublishGame(g.id, token)
                                setNotice(`已下架「${g.title}」`)
                                loadGames(gameKeyword)
                              })
                            }
                          >
                            下架
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      ) : null}

      {tab === 'audit' ? (
        <section className="community-card">
          <h2>审计日志</h2>
          {logs.length === 0 ? (
            <div className="admin-meta">暂无记录</div>
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>操作者</th>
                    <th>角色</th>
                    <th>动作</th>
                    <th>资源</th>
                    <th>IP</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map((log) => (
                    <tr key={log.id}>
                      <td>{log.id}</td>
                      <td>#{log.actorId}</td>
                      <td>{log.actorRole}</td>
                      <td>{log.action}</td>
                      <td>
                        {log.resourceType}
                        {log.resourceId ? ` / ${log.resourceId}` : ''}
                        {log.afterJson ? <span className="audit-detail" title={log.afterJson}>{` ${log.afterJson}`}</span> : null}
                      </td>
                      <td>{log.ip || '—'}</td>
                      <td>{formatDate(log.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      ) : null}

      {tab === 'analytics' ? (
        <>
          <section className="community-card">
            <h2>经营分析（GP-402）</h2>
            <div className="admin-form" style={{ marginBottom: '0.75rem' }}>
              <input type="date" value={anFrom} onChange={(e) => setAnFrom(e.target.value)} aria-label="起始日期" />
              <input type="date" value={anTo} onChange={(e) => setAnTo(e.target.value)} aria-label="结束日期" />
              <input
                value={anGameId}
                onChange={(e) => setAnGameId(e.target.value)}
                placeholder="game_id（可空）"
              />
              <button type="button" className="primary-btn" onClick={loadAnalytics}>
                查询
              </button>
            </div>
            <div className="admin-meta">窗口空 = 缺省近 30 天；日期为 10 位（YYYY-MM-DD，含全天边界）</div>
          </section>

          {overview ? (
            <section className="community-card">
              <h2>总览</h2>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>事件类型</th>
                      <th>次数</th>
                      <th>人数</th>
                      <th>金额</th>
                    </tr>
                  </thead>
                  <tbody>
                    {overview.items.map((row) => (
                      <tr key={row.eventType}>
                        <td>{row.eventType}</td>
                        <td>{row.events}</td>
                        <td>{row.users}</td>
                        <td>¥{(row.amountCents / 100).toFixed(2)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {funnel ? (
            <section className="community-card">
              <h2>注册→活跃→下载→付费 漏斗</h2>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>阶段</th>
                      <th>人数</th>
                      <th>相对注册</th>
                      <th>相对上一阶段</th>
                    </tr>
                  </thead>
                  <tbody>
                    {funnel.stages.map((stage) => (
                      <tr key={stage.stage}>
                        <td>{stage.stage}</td>
                        <td>{stage.users}</td>
                        <td>{(stage.overallConv * 100).toFixed(1)}%</td>
                        <td>{(stage.stepConv * 100).toFixed(1)}%</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {retention ? (
            <section className="community-card">
              <h2>留存（{retention.eventType}，窗口 {retention.window} 天）</h2>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>首活日</th>
                      <th>规模</th>
                      <th>D0</th>
                      <th>D1</th>
                      <th>D3</th>
                      <th>D7</th>
                    </tr>
                  </thead>
                  <tbody>
                    {retention.cohorts.map((cohort) => (
                      <tr key={cohort.day}>
                        <td>{cohort.day}</td>
                        <td>{cohort.size}</td>
                        {[0, 1, 3, 7].map((d) => {
                          const cell = cohort.retention[String(d)]
                          return (
                            <td key={d}>
                              {cell && cohort.size > 0
                                ? `${(cell.retention * 100).toFixed(1)}%（${cell.users}）`
                                : '—'}
                            </td>
                          )
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {reconciliation.length ? (
            <section className="community-card">
              <h2>账单核对（按游戏）</h2>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>游戏</th>
                      <th>付费笔数</th>
                      <th>付费金额</th>
                      <th>退款笔数</th>
                      <th>退款金额</th>
                    </tr>
                  </thead>
                  <tbody>
                    {reconciliation.map((row) => (
                      <tr key={row.gameId}>
                        <td>{row.gameId}</td>
                        <td>{row.paidCount}</td>
                        <td>¥{(row.paidCents / 100).toFixed(2)}</td>
                        <td>{row.refundCount}</td>
                        <td>¥{(row.refundCents / 100).toFixed(2)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {channels.length ? (
            <section className="community-card">
              <h2>渠道效果</h2>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>渠道</th>
                      <th>注册数</th>
                      <th>去重用户</th>
                      <th>付费金额</th>
                    </tr>
                  </thead>
                  <tbody>
                    {channels.map((row) => (
                      <tr key={row.channel || '(自然流量)'}>
                        <td>{row.channel || '（自然流量）'}</td>
                        <td>{row.registrations}</td>
                        <td>{row.users}</td>
                        <td>¥{(row.paidCents / 100).toFixed(2)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          <section className="community-card">
            <h2>活动效果</h2>
            <div className="admin-form" style={{ marginBottom: '0.5rem' }}>
              <input
                value={anActivityId}
                onChange={(e) => setAnActivityId(e.target.value)}
                placeholder="活动 id"
              />
              <button type="button" className="primary-btn" onClick={loadActivityEffect}>
                查询
              </button>
            </div>
            {activityEffect ? <div className="admin-meta">{activityEffect}</div> : null}
          </section>
        </>
      ) : null}

      {tab === 'promo' ? (
        <>
          <section className="community-card">
            <h2>登记渠道</h2>
            <p className="admin-meta">
              code 为推广链接 ?channel= 标识（全局唯一）；归因窗口小时数缺省 72（0-720，显式 0 = 不开放归因）。变更历史见审计日志（resource_type=promo_channel）。
            </p>
            <div className="admin-form">
              <input
                value={promoCode}
                onChange={(e) => setPromoCode(e.target.value)}
                placeholder="渠道标识，如 kol-li"
              />
              <input value={promoName} onChange={(e) => setPromoName(e.target.value)} placeholder="渠道名称" />
              <input
                value={promoWindow}
                onChange={(e) => setPromoWindow(e.target.value)}
                placeholder="归因窗口（小时）"
                inputMode="numeric"
              />
              <button
                type="button"
                className="primary-btn"
                onClick={submitPromoChannel}
                disabled={loading || !promoCode.trim() || !promoName.trim()}
              >
                登记
              </button>
            </div>
          </section>

          <section className="community-card">
            <h2>渠道列表</h2>
            {promoChannels.length === 0 ? (
              <div className="admin-meta">暂无渠道</div>
            ) : (
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>标识</th>
                      <th>名称</th>
                      <th>归因窗口</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {promoChannels.map((channel) => (
                      <tr key={channel.id}>
                        <td>{channel.id}</td>
                        <td>{channel.code}</td>
                        <td>
                          {promoEditId === channel.id ? (
                            <div className="admin-form">
                              <input
                                value={promoEditName}
                                onChange={(e) => setPromoEditName(e.target.value)}
                                placeholder="名称"
                              />
                              <input
                                value={promoEditWindow}
                                onChange={(e) => setPromoEditWindow(e.target.value)}
                                placeholder="窗口（小时）"
                                inputMode="numeric"
                              />
                            </div>
                          ) : (
                            channel.name
                          )}
                        </td>
                        <td>
                          {promoEditId === channel.id
                            ? `${promoEditWindow || channel.attributionWindowHours} 小时`
                            : `${channel.attributionWindowHours} 小时`}
                        </td>
                        <td>
                          <StatusBadge status={channel.status} />
                        </td>
                        <td>{formatDate(channel.createdAt)}</td>
                        <td>
                          {promoEditId === channel.id ? (
                            <div className="admin-form">
                              <button type="button" className="primary-btn" onClick={() => submitPromoEdit(channel)}>
                                保存
                              </button>
                              <button type="button" className="secondary-btn" onClick={() => setPromoEditId(null)}>
                                取消
                              </button>
                            </div>
                          ) : (
                            <div className="admin-form">
                              <button
                                type="button"
                                className="secondary-btn"
                                onClick={() => {
                                  setPromoEditId(channel.id)
                                  setPromoEditName(channel.name)
                                  setPromoEditWindow(String(channel.attributionWindowHours))
                                }}
                              >
                                改名/窗口
                              </button>
                              <button type="button" className="secondary-btn" onClick={() => togglePromoChannel(channel)}>
                                {channel.status === 'disabled' ? '启用' : '停用'}
                              </button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className="community-card">
            <h2>渠道效果</h2>
            <div className="admin-form">
              <select value={promoStatsCode} onChange={(e) => setPromoStatsCode(e.target.value)}>
                <option value="">选择渠道</option>
                {promoChannels.map((channel) => (
                  <option key={channel.id} value={channel.code}>
                    {channel.code}（{channel.name}）
                  </option>
                ))}
              </select>
              <button type="button" className="primary-btn" onClick={queryPromoStats} disabled={!promoStatsCode}>
                查询计数
              </button>
            </div>
            {promoStats ? (
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>渠道</th>
                      <th>点击</th>
                      <th>下载</th>
                      <th>归因注册</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>{promoStats.channel}</td>
                      <td>{promoStats.clicks}</td>
                      <td>{promoStats.downloads}</td>
                      <td>{promoStats.attributedUsers}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="admin-meta">选择渠道后查询点击/下载/归因人数（时间区间同经营分析窗口）</div>
            )}
          </section>

          <section className="community-card">
            <h2>推广链接生成器</h2>
            <p className="admin-meta">
              链接形如 /games/:id?channel=&lt;code&gt;&amp;click_id=&lt;随机&gt;；落地页在归因窗口内注册即计归因。
            </p>
            <div className="admin-form">
              <select value={promoLinkCode} onChange={(e) => setPromoLinkCode(e.target.value)}>
                <option value="">选择渠道</option>
                {promoChannels
                  .filter((channel) => channel.status !== 'disabled')
                  .map((channel) => (
                    <option key={channel.id} value={channel.code}>
                      {channel.code}（{channel.name}）
                    </option>
                  ))}
              </select>
              <select value={promoLinkGameId} onChange={(e) => setPromoLinkGameId(e.target.value)}>
                <option value="">（不指定游戏）</option>
                {games.map((game) => (
                  <option key={game.id} value={game.id}>
                    {game.title || game.id}
                  </option>
                ))}
              </select>
              <button type="button" className="primary-btn" onClick={generatePromoLink} disabled={!promoLinkCode}>
                生成
              </button>
            </div>
            {promoLink ? (
              <div className="admin-form">
                <input readOnly value={promoLink} onFocus={(e) => e.currentTarget.select()} />
                <button
                  type="button"
                  className="secondary-btn"
                  onClick={() => navigator.clipboard?.writeText(promoLink).catch(() => {})}
                >
                  复制
                </button>
              </div>
            ) : null}
          </section>
        </>
      ) : null}
    </div>
  )
}

export default AdminPage
