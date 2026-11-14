import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchDesignBreakdown,
  fetchDesignBreakdowns,
  fetchDesignCategories,
  type DesignBreakdown,
  type DesignCategoryRow,
} from '../api/design'
import './design.css'

// compareClip 对比表格单元格截断（全文在详情页）。
const compareClip = (text: string, max = 90): string =>
  text.length > max ? `${text.slice(0, max)}…` : text

// DesignHubPage 设计拆解库入口：品类聚合（同类游戏集中）+ 拆解列表 +
// 品类横向对比表（选中品类后按详情拉全文对比核心维度）。
const DesignHubPage = () => {
  const [categories, setCategories] = useState<DesignCategoryRow[]>([])
  const [items, setItems] = useState<DesignBreakdown[]>([])
  const [total, setTotal] = useState(0)
  const [category, setCategory] = useState('')
  const [featured, setFeatured] = useState(false)
  const [details, setDetails] = useState<Map<number, DesignBreakdown>>(new Map())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [cats, list] = await Promise.all([
        fetchDesignCategories(),
        fetchDesignBreakdowns({
          category: category || undefined,
          featured: featured || undefined,
          limit: 50,
        }),
      ])
      setCategories(cats)
      setItems(list.items)
      setTotal(list.total)
      // 对比表需要全文分节：列表条目只回摘要，选中品类后逐篇补拉详情
      // （品类内数量小，批量拉取成本可控）。
      if (category) {
        const full = await Promise.all(list.items.map((b) => fetchDesignBreakdown(b.id)))
        setDetails(new Map(full.map((b) => [b.id, b])))
      } else {
        setDetails(new Map())
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [category, featured])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="design-page">
      <header className="design-header">
        <h1>设计拆解</h1>
        <p className="design-tagline">
          策划视角的游戏拆解库：核心循环 / 进度 / 经济 / 战斗 / 养成 / 社交 / 付费 / 公平性，
          按品类聚合横向对比。内容基于公开设计事实并标注来源，欢迎在拆解页讨论区补充视角。
        </p>
      </header>

      <div className="design-filters">
        <div className="design-cats">
          <button
            type="button"
            className={`design-cat ${category === '' ? 'active' : ''}`}
            onClick={() => setCategory('')}
          >
            全部
          </button>
          {categories.map((c) => (
            <button
              key={c.category}
              type="button"
              className={`design-cat ${category === c.category ? 'active' : ''}`}
              onClick={() => setCategory(category === c.category ? '' : c.category)}
            >
              {c.category}
              <span className="design-cat-count">{c.count}</span>
            </button>
          ))}
        </div>
        <label className="design-featured-toggle">
          <input
            type="checkbox"
            checked={featured}
            onChange={(e) => setFeatured(e.target.checked)}
          />
          只看精选
        </label>
      </div>

      {loading ? (
        <p className="design-state">加载中…</p>
      ) : error ? (
        <p className="design-state design-error">{error}</p>
      ) : items.length === 0 ? (
        <p className="design-state">暂无拆解，换个筛选条件试试。</p>
      ) : (
        <>
          <div className="design-grid">
            {items.map((b) => (
              <Link key={b.id} to={`/design/${b.id}`} className="design-card">
                <div className="design-card-head">
                  <span className="design-card-cat">{b.category}</span>
                  {b.featured ? <span className="design-card-featured">精选</span> : null}
                </div>
                <h3>{b.game_title}</h3>
                <p className="design-card-summary">{b.summary}</p>
                <div className="design-card-meta">
                  <span>{b.author_name}</span>
                  <span>{b.updated_at.slice(0, 10)}</span>
                </div>
              </Link>
            ))}
          </div>

          {category && details.size > 0 ? (
            <section className="design-compare">
              <h2>{category} · 横向对比</h2>
              <table className="design-compare-table">
                <thead>
                  <tr>
                    <th>游戏</th>
                    <th>核心循环</th>
                    <th>付费主线</th>
                    <th>公平性要点</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((b) => {
                    const full = details.get(b.id)
                    return (
                      <tr key={b.id}>
                        <td>
                          <Link to={`/design/${b.id}`}>{b.game_title}</Link>
                        </td>
                        <td>{full ? compareClip(full.core_loop) : '—'}</td>
                        <td>{full ? compareClip(full.monetization) : '—'}</td>
                        <td>{full ? compareClip(full.fairness) : '—'}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </section>
          ) : null}

          <p className="design-total">共 {total} 篇拆解</p>
        </>
      )}
    </div>
  )
}

export default DesignHubPage
