// GP-401 长评举报管理域 API 契约测试（vitest）：锁 URL/方法/请求体/鉴权头与
// 响应映射，fetch 全程 mock 不打真实上游。http.ts 在模块加载时读取
// import.meta.env 决定网关地址，故每个用例前 resetModules + stubEnv 后动态 import。
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

const importAdmin = async () => import('./admin')

const callUrl = (): string => String(fetchMock.mock.calls[0][0])
const callInit = (): RequestInit | undefined => fetchMock.mock.calls[0][1]
const callHeaders = (): Record<string, string> => callInit()?.headers as Record<string, string>

describe('admin review report api', () => {
  it('fetchReviewReports 举报队列查询（GET /admin/review-reports + 状态过滤）', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        items: [{ id: 1, review_id: 9, game_id: 'game-a', reporter_id: 7, reason: '广告', status: 'pending' }],
        total: 1,
      }),
    )
    const { fetchReviewReports } = await importAdmin()
    const list = await fetchReviewReports({ status: 'pending', limit: 50 }, 'tok')
    expect(list.total).toBe(1)
    expect(list.items).toEqual([
      {
        id: 1,
        reviewId: 9,
        gameId: 'game-a',
        reporterId: 7,
        reason: '广告',
        status: 'pending',
        handledBy: 0,
        createdAt: '',
        handledAt: '',
      },
    ])
    expect(callUrl()).toBe(`${GW}/admin/review-reports?status=pending&limit=50&offset=0`)
    expect(callHeaders().Authorization).toBe('Bearer tok')
  })

  it('fetchReviewReports 空状态不打 status 参数；缺字段回落空集', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({}))
    const { fetchReviewReports } = await importAdmin()
    const list = await fetchReviewReports({}, 'tok')
    expect(list.items).toEqual([])
    expect(list.total).toBe(0)
    expect(callUrl()).toBe(`${GW}/admin/review-reports?limit=50&offset=0`)
  })

  it('handleReviewReport 处置举报（POST /handle + action 体）', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ id: 1, review_id: 9, game_id: 'game-a', reporter_id: 7, reason: '广告', status: 'resolved' }),
    )
    const { handleReviewReport } = await importAdmin()
    const report = await handleReviewReport(1, 'remove', 'adm')
    expect(report.status).toBe('resolved')
    expect(report.reviewId).toBe(9)
    expect(callUrl()).toBe(`${GW}/admin/review-reports/1/handle`)
    expect(callInit()?.method).toBe('POST')
    expect(callHeaders().Authorization).toBe('Bearer adm')
    expect(JSON.parse(String(callInit()?.body))).toEqual({ action: 'remove' })
  })

  it('restoreReview 恢复软删评价（POST /admin/reviews/:id/restore）', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ id: 9, game_id: 'game-a', user_id: 5, rating: 4, content: '很好', created_at: '2026-11-12T09:00:00Z' }),
    )
    const { restoreReview } = await importAdmin()
    const review = await restoreReview(9, 'adm')
    expect(review.id).toBe(9)
    expect(review.rating).toBe(4)
    expect(review.deletedAt).toBe('')
    expect(callUrl()).toBe(`${GW}/admin/reviews/9/restore`)
    expect(callInit()?.method).toBe('POST')
    expect(callHeaders().Authorization).toBe('Bearer adm')
  })
})