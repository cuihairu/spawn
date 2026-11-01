import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchPartnerGames,
  fetchPartnerVersions,
  partnerCreateVersion,
  partnerSubmitReview,
  type AdminGame,
  type GameVersionInfo,
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

  useEffect(() => {
    queueMicrotask(loadGames)
  }, [loadGames])

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
    </div>
  )
}

export default PartnerPage
