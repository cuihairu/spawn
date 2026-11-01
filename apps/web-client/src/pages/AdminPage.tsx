import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  assignOwnership,
  createPartner,
  decideReview,
  fetchAuditLogs,
  fetchGames,
  fetchPartners,
  fetchReviews,
  grantRole,
  publishGame,
  setPartnerStatus,
  unpublishGame,
  type AdminGame,
  type AuditLogInfo,
  type GameReviewInfo,
  type PartnerInfo,
} from '../api/client'
import './admin.css'

type Tab = 'reviews' | 'partners' | 'games' | 'audit'

interface Props {
  token: string
  isAdmin: boolean // platform_admin 可授予角色；operator 仅日常审核/合作方
}

const STATUS_LABELS: Record<string, string> = {
  draft: '草稿',
  pending: '待审核',
  approved: '已过审',
  rejected: '已驳回',
  offline: '已下架',
  active: '正常',
  suspended: '已挂起',
}

function StatusBadge({ status }: { status: string }) {
  const cls = status in STATUS_LABELS ? `status-${status}` : 'status-draft'
  return <span className={`status-badge ${cls}`}>{STATUS_LABELS[status] ?? status}</span>
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

  useEffect(() => {
    queueMicrotask(() => {
      if (tab === 'reviews') loadReviews(reviewFilter)
      else if (tab === 'partners') loadPartners()
      else if (tab === 'games') loadGames(gameKeyword)
      else loadAudit()
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  return (
    <div className="admin-page">
      <div className="admin-header">
        <div>
          <h1>运营后台</h1>
          <p className="admin-meta">审核 / 合作方 / 上下架 / 审计（{isAdmin ? '管理员' : '运营'}）</p>
        </div>
        <div className="admin-tabs">
          <button type="button" className={`admin-tab${tab === 'reviews' ? ' is-active' : ''}`} onClick={() => setTab('reviews')}>
            审核队列（{reviewTotal}）
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
    </div>
  )
}

export default AdminPage
