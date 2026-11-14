// 设计拆解域 API 契约测试（vitest）：锁 URL/方法/请求体/鉴权头与响应映射，
// fetch 全程 mock 不打真实上游。http.ts 在模块加载时读取 import.meta.env
// 决定网关地址，故每个用例前 resetModules + stubEnv 后动态 import。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const GW = 'http://gw.test'

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.resetModules()
  vi.stubEnv('VITE_API_GATEWAY_URL', GW)
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})

const importDesign = async () => import('./design')

const callUrl = (): string => String(fetchMock.mock.calls[0][0])
const callInit = (): RequestInit | undefined => fetchMock.mock.calls[0][1]
const callHeaders = (): Record<string, string> => callInit()?.headers as Record<string, string>

describe('design api', () => {
  it('fetchDesignCategories 品类聚合（GET /design/categories）', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ categories: [{ category: 'MMORPG', count: 3 }, { category: 'MOBA', count: 2 }] }),
    )
    const { fetchDesignCategories } = await importDesign()
    const cats = await fetchDesignCategories()
    expect(cats).toEqual([
      { category: 'MMORPG', count: 3 },
      { category: 'MOBA', count: 2 },
    ])
    expect(callUrl()).toBe(`${GW}/api/v1/design/categories`)
    expect(fetchDesignCategories).toBeTruthy()
  })

  it('fetchDesignBreakdowns 品类/精选/分页查询串', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ items: [{ id: 1, game_title: '原神' }], total: 12 }),
    )
    const { fetchDesignBreakdowns } = await importDesign()
    const list = await fetchDesignBreakdowns({ category: '卡牌', featured: true, limit: 5, offset: 10 })
    expect(list.total).toBe(12)
    expect(list.items).toEqual([{ id: 1, game_title: '原神' }])
    expect(callUrl()).toBe(
      `${GW}/api/v1/design/breakdowns?category=%E5%8D%A1%E7%89%8C&featured=true&limit=5&offset=10`,
    )
  })

  it('fetchDesignBreakdowns 空参数不打查询串；缺字段回落空集', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({}))
    const { fetchDesignBreakdowns } = await importDesign()
    const list = await fetchDesignBreakdowns()
    expect(list.items).toEqual([])
    expect(list.total).toBe(0)
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns`)
  })

  it('fetchDesignBreakdown 详情全文', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ breakdown: { id: 7, game_title: '炉石传说', core_loop: '构筑循环', sources: '公开' } }),
    )
    const { fetchDesignBreakdown } = await importDesign()
    const b = await fetchDesignBreakdown(7)
    expect(b.game_title).toBe('炉石传说')
    expect(b.core_loop).toBe('构筑循环')
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/7`)
  })

  it('fetchDesignComments 评论列表（升序回传即升序展示）', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ comments: [{ id: 1, content: '很有参考价值' }] }),
    )
    const { fetchDesignComments } = await importDesign()
    const cs = await fetchDesignComments(7)
    expect(cs).toEqual([{ id: 1, content: '很有参考价值' }])
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/7/comments?limit=50&offset=0`)
  })

  it('createDesignBreakdown 登录发布（POST + Bearer + 草稿体）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ breakdown: { id: 13 } }))
    const { createDesignBreakdown } = await importDesign()
    const id = await createDesignBreakdown(
      { game_title: '测试拆解', category: 'MMORPG', summary: '摘要' },
      'tok',
    )
    expect(id).toBe(13)
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns`)
    expect(callInit()?.method).toBe('POST')
    expect(callHeaders().Authorization).toBe('Bearer tok')
    expect(JSON.parse(String(callInit()?.body))).toEqual({
      game_title: '测试拆解',
      category: 'MMORPG',
      summary: '摘要',
    })
  })

  it('updateDesignBreakdown 仅作者编辑（PUT /breakdowns/:id）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ breakdown: { id: 3 } }))
    const { updateDesignBreakdown } = await importDesign()
    await updateDesignBreakdown(3, { game_title: '改', category: 'MOBA', summary: '' }, 'tok')
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/3`)
    expect(callInit()?.method).toBe('PUT')
    expect(callHeaders().Authorization).toBe('Bearer tok')
  })

  it('setDesignFeatured 精选运营位（POST feature 体）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ code: 200, message: 'ok' }))
    const { setDesignFeatured } = await importDesign()
    await setDesignFeatured(3, true, 'adm')
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/3/feature`)
    expect(callInit()?.method).toBe('POST')
    expect(JSON.parse(String(callInit()?.body))).toEqual({ featured: true })
  })

  it('createDesignComment 讨论区发言', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ comment: { id: 9, breakdown_id: 7, content: '补充视角' } }),
    )
    const { createDesignComment } = await importDesign()
    const c = await createDesignComment(7, '补充视角', 'tok')
    expect(c.id).toBe(9)
    expect(c.content).toBe('补充视角')
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/7/comments`)
    expect(JSON.parse(String(callInit()?.body))).toEqual({ content: '补充视角' })
  })

  it('deleteDesignComment 删自己评论（DELETE /comments/:cid）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ code: 200, message: 'ok' }))
    const { deleteDesignComment } = await importDesign()
    await deleteDesignComment(7, 9, 'tok')
    expect(callUrl()).toBe(`${GW}/api/v1/design/breakdowns/7/comments/9`)
    expect(callInit()?.method).toBe('DELETE')
    expect(callHeaders().Authorization).toBe('Bearer tok')
  })
})
