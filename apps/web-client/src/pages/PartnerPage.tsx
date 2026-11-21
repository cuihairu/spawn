import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchPartnerAnalyticsOverview,
  fetchPartnerGames,
  fetchPartnerStatementDifferences,
  fetchPartnerStatements,
  fetchPartnerStatementExport,
  fetchPartnerVersions,
  partnerConfirmStatement,
  partnerCreateVersion,
  partnerDisputeStatement,
  partnerSubmitReview,
  type AdminGame,
  type GameVersionInfo,
  type PartnerAnalyticsOverview,
  type PartnerStatement,
} from '../api/client'
import './admin.css'

interface Props {
  token: string
  partnerId: number
}

const STATUS_LABELS: Record<string, string> = {
  draft: '草稿',
  pending: '待审核',
  approved: '已过审',
  rejected: '已驳回',
  offline: '已下架',
  confirmed: '已确认',
  disputed: '有异议',
}

function StatusBadge({ status }: { status: string }) {
  const cls = status in STATUS_LABELS ? `status-${status}` : 'status-draft'
  return <span className={`status-badge ${cls}`}>{STATUS_LABELS[status] ?? status}</span>
}

// 合作方自助台（GP-103）：自有游戏列表 + 资料提审 + 版本创建/提审。
// 数据经网关反代 game-catalog（/partner/*），按合作方归属服务端过滤。
const PartnerPage = ({ token, partnerId }: Props) => {
  const [games, setGames] = useState<AdminGame[]>([])
  const [versions, setVersions] = useState<GameVersionInfo[]>([])
  const [selectedGame, setSelectedGame] = useState('')
  const [versionName, setVersionName] = useState('')
  const [versionChangelog, setVersionChangelog] = useState('')
  // GP-104 渠道包字段
  const [packageName, setPackageName] = useState('')
  const [versionCode, setVersionCode] = useState('')
  const [signFingerprint, setSignFingerprint] = useState('')
  const [sizeBytes, setSizeBytes] = useState('')
  const [hash, setHash] = useState('')
  const [storageUrl, setStorageUrl] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  // GP-404 合作方自助：对账单确认/异议/导出 + 名下经营数据
  const [statements, setStatements] = useState<PartnerStatement[]>([])
  const [differences, setDifferences] = useState<PartnerStatement[]>([])
  const [disputeId, setDisputeId] = useState<number | null>(null)
  const [disputeReason, setDisputeReason] = useState('')
  const [partAn, setPartAn] = useState<PartnerAnalyticsOverview | null>(null)
  const [partFrom, setPartFrom] = useState('')
  const [partTo, setPartTo] = useState('')

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

  const loadGames = useCallback(() => {
    run(async () => {
      setGames(await fetchPartnerGames(token))
    })
  }, [run, token])

  const loadVersions = useCallback(
    (gameId: string) => {
      if (!gameId) {
        setVersions([])
        return
      }
      run(async () => {
        setVersions(await fetchPartnerVersions(gameId, token))
      })
    },
    [run, token],
  )

  const loadStatements = useCallback(() => {
    run(async () => {
      const [items, diffs] = await Promise.all([
        fetchPartnerStatements(undefined, token),
        fetchPartnerStatementDifferences(undefined, token),
      ])
      setStatements(items)
      setDifferences(diffs)
    })
  }, [run, token])

  const loadPartnerAnalytics = useCallback(() => {
    run(async () => {
      setPartAn(
        await fetchPartnerAnalyticsOverview(
          { from: partFrom.trim() || undefined, to: partTo.trim() || undefined },
          token,
        ),
      )
    })
  }, [partFrom, partTo, run, token])

  useEffect(() => {
    queueMicrotask(loadGames)
  }, [loadGames])

  useEffect(() => {
    queueMicrotask(loadStatements)
  }, [loadStatements])

  return (
    <div className="admin-page">
      <div className="admin-header">
        <div>
          <h1>合作方后台</h1>
          <p className="admin-meta">合作方 #{partnerId} · 管理自有游戏资料与版本提审</p>
        </div>
        <button type="button" className="secondary-btn" onClick={loadGames} disabled={loading}>
          {loading ? '刷新中...' : '刷新'}
        </button>
      </div>

      {error ? (
        <div className="error-banner" role="alert">
          {error}
        </div>
      ) : null}
      {notice ? <div className="notice-banner">{notice}</div> : null}

      {games.length === 0 && !loading && !error ? (
        <section className="community-card">
          <div className="admin-meta">
            暂无归属游戏，请联系运营在后台指派；游戏库见
            <Link to="/games">游戏库</Link>
          </div>
        </section>
      ) : null}

      <section className="community-card">
        <h2>自有游戏（{games.length}）</h2>
        {games.length === 0 ? null : (
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>游戏</th>
                  <th>状态</th>
                  <th>审核原因</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {games.map((g) => (
                  <tr key={g.id}>
                    <td>{g.title}</td>
                    <td>
                      <StatusBadge status={g.status} />
                    </td>
                    <td>{g.reviewReason || '—'}</td>
                    <td>
                      <div className="inline-actions">
                        <button
                          type="button"
                          className="secondary-btn"
                          disabled={loading || g.status === 'pending'}
                          onClick={() =>
                            run(async () => {
                              const review = await partnerSubmitReview(g.id, undefined, token)
                              setNotice(`「${g.title}」已提审（#${review.id}）`)
                              loadGames()
                            })
                          }
                        >
                          资料提审
                        </button>
                        <button
                          type="button"
                          className="secondary-btn"
                          onClick={() => {
                            setSelectedGame(g.id)
                            loadVersions(g.id)
                          }}
                        >
                          版本管理
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {selectedGame ? (
        <>
          <section className="community-card">
            <h2>新建版本（{games.find((g) => g.id === selectedGame)?.title ?? selectedGame}）</h2>
            <div className="admin-form">
              <input placeholder="版本号（如 1.2.0）" value={versionName} onChange={(e) => setVersionName(e.target.value)} />
              <input className="reason-input" placeholder="更新说明（可选）" value={versionChangelog} onChange={(e) => setVersionChangelog(e.target.value)} />
            </div>
            <div className="admin-form" style={{ marginTop: '0.5rem' }}>
              <input placeholder="包名（如 com.demo.game）" value={packageName} onChange={(e) => setPackageName(e.target.value)} />
              <input type="number" placeholder="versionCode" value={versionCode} onChange={(e) => setVersionCode(e.target.value)} />
              <input placeholder="签名指纹" value={signFingerprint} onChange={(e) => setSignFingerprint(e.target.value)} />
              <input type="number" placeholder="大小(字节)" value={sizeBytes} onChange={(e) => setSizeBytes(e.target.value)} />
            </div>
            <div className="admin-form" style={{ marginTop: '0.5rem' }}>
              <input placeholder="哈希（如 sha256:...）" value={hash} onChange={(e) => setHash(e.target.value)} />
              <input className="reason-input" placeholder="存储地址（如 s3://...）" value={storageUrl} onChange={(e) => setStorageUrl(e.target.value)} />
              <button
                type="button"
                className="secondary-btn"
                disabled={loading || !versionName.trim()}
                onClick={() =>
                  run(async () => {
                    const v = await partnerCreateVersion(
                      selectedGame,
                      {
                        version: versionName.trim(),
                        changelog: versionChangelog.trim() || undefined,
                        packageName: packageName.trim() || undefined,
                        versionCode: versionCode ? Number(versionCode) : undefined,
                        signFingerprint: signFingerprint.trim() || undefined,
                        sizeBytes: sizeBytes ? Number(sizeBytes) : undefined,
                        hash: hash.trim() || undefined,
                        storageUrl: storageUrl.trim() || undefined,
                      },
                      token,
                    )
                    setNotice(`版本 ${v.version} 已创建（草稿）`)
                    setVersionName('')
                    setVersionChangelog('')
                    setPackageName('')
                    setVersionCode('')
                    setSignFingerprint('')
                    setSizeBytes('')
                    setHash('')
                    setStorageUrl('')
                    loadVersions(selectedGame)
                  })
                }
              >
                创建
              </button>
            </div>
          </section>

          <section className="community-card">
            <h2>版本列表（{versions.length}）</h2>
            {versions.length === 0 ? (
              <div className="admin-meta">暂无版本</div>
            ) : (
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>版本</th>
                      <th>包名 / code</th>
                      <th>大小</th>
                      <th>哈希</th>
                      <th>更新说明</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {versions.map((v) => (
                      <tr key={v.id}>
                        <td>{v.version}</td>
                        <td>
                          {v.packageName || '—'}
                          {v.versionCode ? <div className="admin-meta">code {v.versionCode}</div> : null}
                        </td>
                        <td>{v.sizeBytes > 0 ? `${(v.sizeBytes / 1024 / 1024).toFixed(1)} MB` : '—'}</td>
                        <td>{v.hash || '—'}</td>
                        <td>{v.changelog || '—'}</td>
                        <td>
                          <StatusBadge status={v.status} />
                          {v.reviewReason ? <div className="admin-meta">{v.reviewReason}</div> : null}
                        </td>
                        <td>{v.createdAt ? new Date(v.createdAt).toLocaleString('zh-CN', { hour12: false }) : '—'}</td>
                        <td>
                          <button
                            type="button"
                            className="secondary-btn"
                            disabled={loading || v.status !== 'draft'}
                            onClick={() =>
                              run(async () => {
                                await partnerSubmitReview(selectedGame, v.id, token)
                                setNotice(`版本 ${v.version} 已提审`)
                                loadVersions(selectedGame)
                              })
                            }
                          >
                            提审
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

      <section className="community-card">
        <h2>经营数据（{partAn ? partAn.items.length : 0} 类）</h2>
        <p className="admin-meta">只统计本人名下游戏事件（game-catalog 归属为单一事实源）；金额单位为分。</p>
        <div className="admin-form">
          <input value={partFrom} onChange={(e) => setPartFrom(e.target.value)} placeholder="起（YYYY-MM-DD / RFC3339）" />
          <input value={partTo} onChange={(e) => setPartTo(e.target.value)} placeholder="止" />
          <button type="button" className="primary-btn" onClick={loadPartnerAnalytics} disabled={loading}>
            查询
          </button>
        </div>
        {partAn ? (
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>事件</th>
                  <th>次数</th>
                  <th>去重用户</th>
                  <th>金额（分）</th>
                </tr>
              </thead>
              <tbody>
                {partAn.items.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="admin-meta">
                      区间内暂无事件
                    </td>
                  </tr>
                ) : (
                  partAn.items.map((row) => (
                    <tr key={row.eventType}>
                      <td>{row.eventType}</td>
                      <td>{row.events}</td>
                      <td>{row.users}</td>
                      <td>{row.amountCents}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="admin-meta">选择区间后查询名下事件计数</div>
        )}
      </section>

      <section className="community-card">
        <h2>对账单（{statements.length}）</h2>
        {statements.length === 0 ? (
          <div className="admin-meta">暂无对账单</div>
        ) : (
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>周期</th>
                  <th>渠道</th>
                  <th>订单</th>
                  <th>实付</th>
                  <th>优惠承担</th>
                  <th>退款冲减</th>
                  <th>分成基数</th>
                  <th>分成金额</th>
                  <th>结算</th>
                  <th>差异</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {statements.map((s) => (
                  <tr key={s.id}>
                    <td>{s.id}</td>
                    <td>
                      {s.periodStart} ~ {s.periodEnd}
                    </td>
                    <td>{s.vendor}</td>
                    <td>{s.orderCount}</td>
                    <td>¥{(s.paidTotal / 100).toFixed(2)}</td>
                    <td>¥{(s.discountTotal / 100).toFixed(2)}</td>
                    <td>¥{(s.refundTotal / 100).toFixed(2)}</td>
                    <td>¥{(s.splitBase / 100).toFixed(2)}</td>
                    <td>¥{(s.splitAmount / 100).toFixed(2)}</td>
                    <td>¥{(s.settlement / 100).toFixed(2)}</td>
                    <td>
                      <span className={s.diff !== 0 ? 'status-rejected' : ''}>{s.diff !== 0 ? `¥${(s.diff / 100).toFixed(2)}` : '—'}</span>
                    </td>
                    <td>
                      <StatusBadge status={s.status} />
                      {s.disputeReason ? <div className="admin-meta">异议：{s.disputeReason}</div> : null}
                    </td>
                    <td>
                      <div className="admin-form">
                        <button
                          type="button"
                          className="secondary-btn"
                          disabled={loading || s.status !== 'draft'}
                          onClick={() =>
                            run(async () => {
                              await partnerConfirmStatement(s.id, token)
                              setNotice(`对账单 ${s.id} 已确认`)
                              loadStatements()
                            })
                          }
                        >
                          确认
                        </button>
                        {disputeId === s.id ? (
                          <div className="admin-form">
                            <input
                              value={disputeReason}
                              onChange={(e) => setDisputeReason(e.target.value)}
                              placeholder="异议理由（必填）"
                            />
                            <button
                              type="button"
                              className="secondary-btn"
                              disabled={loading || !disputeReason.trim()}
                              onClick={() =>
                                run(async () => {
                                  await partnerDisputeStatement(s.id, disputeReason.trim(), token)
                                  setNotice(`对账单 ${s.id} 已提异议`)
                                  setDisputeId(null)
                                  setDisputeReason('')
                                  loadStatements()
                                })
                              }
                            >
                              提交异议
                            </button>
                          </div>
                        ) : (
                          <button
                            type="button"
                            className="secondary-btn"
                            disabled={loading || s.diff === 0 || s.status !== 'draft'}
                            onClick={() => {
                              setDisputeId(s.id)
                              setDisputeReason('')
                            }}
                          >
                            提异议
                          </button>
                        )}
                        <button
                          type="button"
                          className="secondary-btn"
                          disabled={loading}
                          onClick={() =>
                            run(async () => {
                              const blob = await fetchPartnerStatementExport(s.id, token)
                              const url = URL.createObjectURL(blob)
                              const a = document.createElement('a')
                              a.href = url
                              a.download = `statement-${s.id}.csv`
                              a.click()
                              URL.revokeObjectURL(url)
                            })
                          }
                        >
                          导出
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {differences.length ? (
          <>
            <h3>差异清单（diff ≠ 0，按 |diff| 降序）</h3>
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>周期</th>
                    <th>差异</th>
                    <th>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {differences.map((s) => (
                    <tr key={s.id}>
                      <td>{s.id}</td>
                      <td>
                        {s.periodStart} ~ {s.periodEnd}
                      </td>
                      <td>¥{(s.diff / 100).toFixed(2)}</td>
                      <td>
                        <StatusBadge status={s.status} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </section>
    </div>
  )
}

export default PartnerPage
