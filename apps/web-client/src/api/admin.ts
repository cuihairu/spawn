// 运营后台与合作方自助（game-catalog 域，经网关反代，GP-103）。
// ========== 运营后台（GP-103，game-catalog 域，经网关反代） ==========
// game-catalog 返回裸 JSON（非信封），错误经 HTTP 状态码 + {code,message}，
// request() 统一在 !response.ok 时抛 message。
import { apiGatewayUrl, gameServiceUrl, gatewayBaseUrl, request } from './http'
import { mapGameDetail } from './games'
import type { GameDetail, GameDto } from './games'

export type PlatformRole = 'platform_admin' | 'operator' | 'partner' | 'player'

export interface MeInfo {
  userId: number
  username: string
  role: PlatformRole
  partnerId: number
}

export interface PartnerInfo {
  id: number
  name: string
  contactEmail: string
  status: 'active' | 'suspended' | string
  createdAt: string
}

export interface GameOwnershipInfo {
  gameId: string
  partnerId: number
  grantedBy: number
  grantedAt: string
  status: string
}

export interface GameReviewInfo {
  id: number
  gameId: string
  versionId: string
  submittedBy: number
  submittedAt: string
  status: string
  reviewerId: number
  reviewReason: string
  reviewedAt: string
}

export interface GameVersionInfo {
  id: string
  gameId: string
  version: string
  changelog: string
  packageName: string
  versionCode: number
  signFingerprint: string
  sizeBytes: number
  hash: string
  storageUrl: string
  status: string
  reviewReason: string
  createdAt: string
}

export interface AuditLogInfo {
  id: number
  actorId: number
  actorRole: string
  action: string
  resourceType: string
  resourceId: string
  beforeJson: string
  afterJson: string
  ip: string
  createdAt: string
}

// 后台/合作方列表里的游戏：含审核状态字段（详情字段复用 GameDetail）
export interface AdminGame extends GameDetail {
  status: string
  reviewReason: string
}

type MeDto = { user_id: number; username: string; role: PlatformRole; partner_id: number }
type PartnerDto = { id: number; name: string; contact_email?: string; status?: string; created_at?: string }
type OwnershipDto = { game_id: string; partner_id: number; granted_by: number; granted_at: string; status: string }
type ReviewDto = {
  id: number
  game_id: string
  version_id?: string
  submitted_by: number
  submitted_at: string
  status: string
  reviewer_id: number
  review_reason: string
  reviewed_at: string
}
type VersionDto = {
  id: string
  game_id: string
  version: string
  changelog?: string
  package_name?: string
  version_code?: number
  sign_fingerprint?: string
  size_bytes?: number
  hash?: string
  storage_url?: string
  status: string
  review_reason?: string
  created_at: string
}
type AuditDto = {
  id: number
  actor_id: number
  actor_role: string
  action: string
  resource_type: string
  resource_id: string
  before_json: string
  after_json: string
  ip: string
  created_at: string
}

function adminHeaders(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` }
}

function mapAdminGame(dto: GameDto & { status?: string; review_reason?: string }): AdminGame {
  const detail = mapGameDetail(dto)
  return { ...detail, status: dto.status ?? 'approved', reviewReason: dto.review_reason ?? '' }
}

// 当前账号平台角色（导航/入口权限用）
export async function fetchMe(token: string): Promise<MeInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<MeDto>(baseUrl, '/me', { headers: adminHeaders(token) })
  return { userId: payload.user_id, username: payload.username, role: payload.role, partnerId: payload.partner_id ?? 0 }
}

export async function grantRole(params: { userId: number; role: string; partnerId?: number }, token: string): Promise<MeInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<MeDto>(baseUrl, '/admin/roles', {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ user_id: params.userId, role: params.role, partner_id: params.partnerId ?? 0 }),
  })
  return { userId: payload.user_id, username: payload.username ?? '', role: payload.role, partnerId: payload.partner_id ?? 0 }
}

export async function createPartner(params: { name: string; contactEmail?: string }, token: string): Promise<PartnerInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<PartnerDto>(baseUrl, '/admin/partners', {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ name: params.name, contact_email: params.contactEmail ?? '' }),
  })
  return mapPartner(payload)
}

export async function fetchPartners(token: string): Promise<PartnerInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ partners: PartnerDto[] }>(baseUrl, '/admin/partners', { headers: adminHeaders(token) })
  return (payload.partners ?? []).map(mapPartner)
}

export async function setPartnerStatus(id: number, status: string, token: string): Promise<PartnerInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<PartnerDto>(baseUrl, `/admin/partners/${id}`, {
    method: 'PATCH',
    headers: adminHeaders(token),
    body: JSON.stringify({ status }),
  })
  return mapPartner(payload)
}

export async function assignOwnership(gameId: string, partnerId: number, token: string): Promise<GameOwnershipInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<OwnershipDto>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/ownership`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ partner_id: partnerId }),
  })
  return mapOwnership(payload)
}

export async function fetchReviews(params: { status?: string; limit?: number; offset?: number }, token: string): Promise<{ reviews: GameReviewInfo[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params.status) searchParams.set('status', params.status)
  searchParams.set('limit', String(params.limit ?? 20))
  searchParams.set('offset', String(params.offset ?? 0))
  const payload = await request<{ reviews: ReviewDto[]; total: number }>(baseUrl, `/admin/reviews?${searchParams.toString()}`, { headers: adminHeaders(token) })
  return { reviews: (payload.reviews ?? []).map(mapReview), total: payload.total ?? 0 }
}

export async function decideReview(id: number, decision: 'approve' | 'reject', reason: string, token: string): Promise<GameReviewInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<ReviewDto>(baseUrl, `/admin/reviews/${id}/decision`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ decision, reason }),
  })
  return mapReview(payload)
}

export async function publishGame(id: string, token: string): Promise<AdminGame> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<GameDto>(baseUrl, `/admin/games/${encodeURIComponent(id)}/publish`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapAdminGame(payload)
}

export async function unpublishGame(id: string, token: string): Promise<AdminGame> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<GameDto>(baseUrl, `/admin/games/${encodeURIComponent(id)}/unpublish`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapAdminGame(payload)
}

export async function fetchGameVersions(gameId: string, token: string): Promise<GameVersionInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ versions: VersionDto[] }>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/versions`, { headers: adminHeaders(token) })
  return (payload.versions ?? []).map(mapVersion)
}

export async function fetchAuditLogs(params: { resourceType?: string; limit?: number; offset?: number }, token: string): Promise<{ logs: AuditLogInfo[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const searchParams = new URLSearchParams()
  if (params.resourceType) searchParams.set('resource_type', params.resourceType)
  searchParams.set('limit', String(params.limit ?? 50))
  searchParams.set('offset', String(params.offset ?? 0))
  const payload = await request<{ logs: AuditDto[]; total: number }>(baseUrl, `/admin/audit?${searchParams.toString()}`, { headers: adminHeaders(token) })
  return { logs: (payload.logs ?? []).map(mapAudit), total: payload.total ?? 0 }
}

// ===== 合作方自助（partner 角色） =====

export async function fetchPartnerGames(token: string): Promise<AdminGame[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ games: (GameDto & { status?: string; review_reason?: string })[] }>(baseUrl, '/partner/games', { headers: adminHeaders(token) })
  return (payload.games ?? []).map(mapAdminGame)
}

export async function partnerSubmitReview(gameId: string, versionId: string | undefined, token: string): Promise<GameReviewInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<ReviewDto>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/submit`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({ version_id: versionId ?? '' }),
  })
  return mapReview(payload)
}

export async function partnerCreateVersion(gameId: string, params: { version: string; changelog?: string; packageName?: string; versionCode?: number; signFingerprint?: string; sizeBytes?: number; hash?: string; storageUrl?: string }, token: string): Promise<GameVersionInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<VersionDto>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/versions`, {
    method: 'POST',
    headers: adminHeaders(token),
    body: JSON.stringify({
      version: params.version,
      changelog: params.changelog ?? '',
      package_name: params.packageName ?? '',
      version_code: params.versionCode ?? 0,
      sign_fingerprint: params.signFingerprint ?? '',
      size_bytes: params.sizeBytes ?? 0,
      hash: params.hash ?? '',
      storage_url: params.storageUrl ?? '',
    }),
  })
  return mapVersion(payload)
}

export async function fetchPartnerVersions(gameId: string, token: string): Promise<GameVersionInfo[]> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<{ versions: VersionDto[] }>(baseUrl, `/partner/games/${encodeURIComponent(gameId)}/versions`, { headers: adminHeaders(token) })
  return (payload.versions ?? []).map(mapVersion)
}

// GP-104：发布/撤回渠道包（运营，admin 路由）
export async function publishVersion(gameId: string, versionId: string, token: string): Promise<GameVersionInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<VersionDto>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/versions/${encodeURIComponent(versionId)}/publish`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapVersion(payload)
}

export async function revokeVersion(gameId: string, versionId: string, token: string): Promise<GameVersionInfo> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const payload = await request<VersionDto>(baseUrl, `/admin/games/${encodeURIComponent(gameId)}/versions/${encodeURIComponent(versionId)}/revoke`, {
    method: 'POST',
    headers: adminHeaders(token),
  })
  return mapVersion(payload)
}

function mapPartner(dto: PartnerDto): PartnerInfo {
  return {
    id: dto.id,
    name: dto.name,
    contactEmail: dto.contact_email ?? '',
    status: dto.status ?? 'active',
    createdAt: dto.created_at ?? '',
  }
}

function mapOwnership(dto: OwnershipDto): GameOwnershipInfo {
  return { gameId: dto.game_id, partnerId: dto.partner_id, grantedBy: dto.granted_by, grantedAt: dto.granted_at, status: dto.status }
}

function mapReview(dto: ReviewDto): GameReviewInfo {
  return {
    id: dto.id,
    gameId: dto.game_id,
    versionId: dto.version_id ?? '',
    submittedBy: dto.submitted_by,
    submittedAt: dto.submitted_at,
    status: dto.status,
    reviewerId: dto.reviewer_id ?? 0,
    reviewReason: dto.review_reason ?? '',
    reviewedAt: dto.reviewed_at ?? '',
  }
}

function mapVersion(dto: VersionDto): GameVersionInfo {
  return {
    id: dto.id,
    gameId: dto.game_id,
    version: dto.version,
    changelog: dto.changelog ?? '',
    packageName: dto.package_name ?? '',
    versionCode: dto.version_code ?? 0,
    signFingerprint: dto.sign_fingerprint ?? '',
    sizeBytes: dto.size_bytes ?? 0,
    hash: dto.hash ?? '',
    storageUrl: dto.storage_url ?? '',
    status: dto.status,
    reviewReason: dto.review_reason ?? '',
    createdAt: dto.created_at,
  }
}

function mapAudit(dto: AuditDto): AuditLogInfo {
  return {
    id: dto.id,
    actorId: dto.actor_id,
    actorRole: dto.actor_role,
    action: dto.action,
    resourceType: dto.resource_type,
    resourceId: dto.resource_id,
    beforeJson: dto.before_json,
    afterJson: dto.after_json,
    ip: dto.ip,
    createdAt: dto.created_at,
  }
}

// ========== 经营分析（GP-402，data-panel 域经网关反代） ==========
// 查询面双闸：Auth 中间件 + AdminUserIds 名单（后端强校验，403 文案为
// 「无权访问经营分析」）。from/to 支持 RFC3339 或 10 位日期；空为缺省
// 窗口（now-30d ~ now）。
export interface AnalyticsTypeCount {
  eventType: string
  events: number
  users: number
  amountCents: number
}

export interface AnalyticsOverview {
  from: string
  to: string
  gameId?: string
  items: AnalyticsTypeCount[]
}

export interface AnalyticsFunnelStage {
  stage: string
  users: number
  events: number
  overallConv: number
  stepConv: number
}

export interface AnalyticsFunnel {
  from: string
  to: string
  gameId?: string
  stages: AnalyticsFunnelStage[]
}

export interface AnalyticsRetentionCell {
  retention: number
  users: number
}

export interface AnalyticsCohortRow {
  day: string
  size: number
  retention: Record<string, AnalyticsRetentionCell>
}

export interface AnalyticsRetention {
  eventType: string
  from: string
  to: string
  window: number
  cohorts: AnalyticsCohortRow[]
}

export interface AnalyticsActivityEffect {
  activityId: number
  grants: number
  participants: number
  rewardCents: number
}

export interface AnalyticsReconciliationRow {
  gameId: string
  paidCount: number
  paidCents: number
  refundCount: number
  refundCents: number
}

export interface AnalyticsChannelRow {
  channel: string
  registrations: number
  users: number
  paidCents: number
}

function analyticsParams(params?: { from?: string; to?: string; gameId?: string }): string {
  const search = new URLSearchParams()
  if (params?.from) search.set('from', params.from)
  if (params?.to) search.set('to', params.to)
  if (params?.gameId) search.set('game_id', params.gameId)
  const qs = search.toString()
  return qs ? `?${qs}` : ''
}

export async function fetchAnalyticsOverview(
  params: { from?: string; to?: string; gameId?: string } | undefined,
  token: string,
): Promise<AnalyticsOverview> {
  const payload = await request<{
    items: { event_type: string; events: number; users: number; amount_cents: number }[]
  }>(gatewayBaseUrl, `/api/v1/analytics/overview${analyticsParams(params)}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return {
    from: params?.from ?? '',
    to: params?.to ?? '',
    gameId: params?.gameId,
    items: (payload.items ?? []).map((r) => ({
      eventType: r.event_type,
      events: r.events,
      users: r.users,
      amountCents: r.amount_cents,
    })),
  }
}

export async function fetchAnalyticsFunnel(
  params: { from?: string; to?: string; gameId?: string } | undefined,
  token: string,
): Promise<AnalyticsFunnel> {
  const payload = await request<{
    from: string
    to: string
    stages: {
      stage: string
      users: number
      events: number
      overall_conv: number
      step_conv: number
    }[]
  }>(gatewayBaseUrl, `/api/v1/analytics/funnel${analyticsParams(params)}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return {
    from: payload.from,
    to: payload.to,
    stages: (payload.stages ?? []).map((s) => ({
      stage: s.stage,
      users: s.users,
      events: s.events,
      overallConv: s.overall_conv,
      stepConv: s.step_conv,
    })),
  }
}

export async function fetchAnalyticsRetention(
  params: { from?: string; to?: string; eventType?: string; window?: number } | undefined,
  token: string,
): Promise<AnalyticsRetention> {
  const search = new URLSearchParams()
  if (params?.from) search.set('from', params.from)
  if (params?.to) search.set('to', params.to)
  if (params?.eventType) search.set('event_type', params.eventType)
  if (params?.window && params.window > 0) search.set('window', String(params.window))
  const payload = await request<{
    event_type: string
    from: string
    to: string
    window: number
    cohorts: { day: string; size: number; retention: Record<string, { retention: number; users: number }> }[]
  }>(gatewayBaseUrl, `/api/v1/analytics/retention?${search.toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return {
    eventType: payload.event_type,
    from: payload.from,
    to: payload.to,
    window: payload.window,
    cohorts: (payload.cohorts ?? []).map((c) => ({
      day: c.day,
      size: c.size,
      retention: Object.fromEntries(
        Object.entries(c.retention ?? {}).map(([k, v]) => [k, { retention: v.retention, users: v.users }]),
      ),
    })),
  }
}

export async function fetchAnalyticsActivityEffect(id: string, token: string): Promise<AnalyticsActivityEffect> {
  const payload = await request<{ activity: AnalyticsActivityEffect }>(
    gatewayBaseUrl,
    `/api/v1/analytics/activities/${encodeURIComponent(id)}/effect`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return payload.activity
}

export async function fetchAnalyticsReconciliation(
  params: { from?: string; to?: string } | undefined,
  token: string,
): Promise<AnalyticsReconciliationRow[]> {
  const payload = await request<{ items: AnalyticsReconciliationRow[] }>(
    gatewayBaseUrl,
    `/api/v1/analytics/reconciliation${analyticsParams(params)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return payload.items ?? []
}

export async function fetchAnalyticsChannels(
  params: { from?: string; to?: string; gameId?: string } | undefined,
  token: string,
): Promise<AnalyticsChannelRow[]> {
  const payload = await request<{ items: AnalyticsChannelRow[] }>(
    gatewayBaseUrl,
    `/api/v1/analytics/channels${analyticsParams(params)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return payload.items ?? []
}

// ========== GP-403 单层渠道推广（game-catalog 域，经网关反代） ==========
// 渠道登记/启停与效果计数为管理路由；归因窗口改窗口不追改既有点击。
// 推广链接落地形如 /games/:id?channel=<code>&click_id=<clickId>。
export interface PromoChannel {
  id: number
  code: string
  name: string
  attributionWindowHours: number
  status: string
  createdAt: string
  updatedAt: string
}

export interface PromoChannelStats {
  channel: string
  clicks: number
  downloads: number
  attributedUsers: number
}

type PromoChannelDto = {
  id: number
  code: string
  name: string
  attribution_window_hours: number
  status: string
  created_at: string
  updated_at: string
}

function mapPromoChannel(dto: PromoChannelDto): PromoChannel {
  return {
    id: dto.id,
    code: dto.code,
    name: dto.name,
    attributionWindowHours: dto.attribution_window_hours,
    status: dto.status,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

export async function fetchPromoChannels(
  params: { limit?: number; offset?: number } | undefined,
  token: string,
): Promise<{ items: PromoChannel[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const search = new URLSearchParams()
  search.set('limit', String(params?.limit ?? 50))
  search.set('offset', String(params?.offset ?? 0))
  const payload = await request<{ items: PromoChannelDto[]; total: number }>(
    baseUrl,
    `/admin/promo/channels?${search.toString()}`,
    { headers: adminHeaders(token) },
  )
  return { items: (payload.items ?? []).map(mapPromoChannel), total: payload.total ?? 0 }
}

export async function createPromoChannel(
  params: { code: string; name: string; windowHours?: number },
  token: string,
): Promise<PromoChannel> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return mapPromoChannel(
    await request<PromoChannelDto>(baseUrl, '/admin/promo/channels', {
      method: 'POST',
      headers: adminHeaders(token),
      body: JSON.stringify({ code: params.code, name: params.name, window_hours: params.windowHours ?? 72 }),
    }),
  )
}

export async function updatePromoChannel(
  id: number,
  params: { name?: string; windowHours?: number },
  token: string,
): Promise<PromoChannel> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return mapPromoChannel(
    await request<PromoChannelDto>(baseUrl, `/admin/promo/channels/${id}`, {
      method: 'PATCH',
      headers: adminHeaders(token),
      body: JSON.stringify({ name: params.name ?? '', window_hours: params.windowHours ?? 0 }),
    }),
  )
}

export async function setPromoChannelStatus(id: number, status: string, token: string): Promise<PromoChannel> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  return mapPromoChannel(
    await request<PromoChannelDto>(baseUrl, `/admin/promo/channels/${id}/status`, {
      method: 'POST',
      headers: adminHeaders(token),
      body: JSON.stringify({ status }),
    }),
  )
}

export async function fetchPromoChannelStats(
  code: string,
  params: { from?: string; to?: string } | undefined,
  token: string,
): Promise<PromoChannelStats> {
  const baseUrl = apiGatewayUrl ?? gameServiceUrl
  const search = new URLSearchParams()
  if (params?.from) search.set('from', params.from)
  if (params?.to) search.set('to', params.to)
  const qs = search.toString()
  return request<PromoChannelStats>(baseUrl, `/admin/promo/channels/${encodeURIComponent(code)}/stats${qs ? `?${qs}` : ''}`, {
    headers: adminHeaders(token),
  })
}

// ========== GP-404 合作方自助：对账单 + 经营数据（经网关反代） ==========
// 账单域在 payments（/partner/statements*），数据域在 data-panel
// （/api/v1/analytics/partner）。角色与归属在服务端判定（game-catalog /me
// 为单一事实源），合作方只看到本人数据（行级隔离 fail-closed）。
export interface PartnerStatement {
  id: number
  vendor: string
  partnerId: number
  periodStart: string
  periodEnd: string
  paidTotal: number
  discountTotal: number
  refundTotal: number
  splitBase: number
  splitRateBp: number
  splitAmount: number
  settlement: number
  diff: number
  orderCount: number
  status: string
  confirmedBy: number
  confirmedAt: string
  disputeBy: number
  disputeAt: string
  disputeReason: string
  createdAt: string
  updatedAt: string
}

export interface PartnerAnalyticsRow {
  eventType: string
  events: number
  users: number
  amountCents: number
}

export interface PartnerAnalyticsOverview {
  from: string
  to: string
  partnerId: number
  items: PartnerAnalyticsRow[]
}

type PartnerStatementDto = {
  id: number
  vendor: string
  partner_id: number
  period_start: string
  period_end: string
  paid_total: number
  discount_total: number
  refund_total: number
  split_base: number
  split_rate_bp: number
  split_amount: number
  settlement: number
  diff: number
  order_count: number
  status: string
  confirmed_by: number
  confirmed_at: string
  dispute_by: number
  dispute_at: string
  dispute_reason: string
  created_at: string
  updated_at: string
}

function mapPartnerStatement(dto: PartnerStatementDto): PartnerStatement {
  return {
    id: dto.id,
    vendor: dto.vendor,
    partnerId: dto.partner_id,
    periodStart: dto.period_start,
    periodEnd: dto.period_end,
    paidTotal: dto.paid_total,
    discountTotal: dto.discount_total,
    refundTotal: dto.refund_total,
    splitBase: dto.split_base,
    splitRateBp: dto.split_rate_bp,
    splitAmount: dto.split_amount,
    settlement: dto.settlement,
    diff: dto.diff,
    orderCount: dto.order_count,
    status: dto.status,
    confirmedBy: dto.confirmed_by,
    confirmedAt: dto.confirmed_at,
    disputeBy: dto.dispute_by,
    disputeAt: dto.dispute_at,
    disputeReason: dto.dispute_reason,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  }
}

export async function fetchPartnerStatements(
  params: { vendor?: string; status?: string } | undefined,
  token: string,
): Promise<PartnerStatement[]> {
  const search = new URLSearchParams()
  if (params?.vendor) search.set('vendor', params.vendor)
  if (params?.status) search.set('status', params.status)
  const qs = search.toString()
  const payload = await request<{ items: PartnerStatementDto[] }>(
    gatewayBaseUrl,
    `/partner/statements${qs ? `?${qs}` : ''}`,
    { headers: adminHeaders(token) },
  )
  return (payload.items ?? []).map(mapPartnerStatement)
}

export async function fetchPartnerStatementDifferences(
  params: { vendor?: string } | undefined,
  token: string,
): Promise<PartnerStatement[]> {
  const search = new URLSearchParams()
  if (params?.vendor) search.set('vendor', params.vendor)
  const qs = search.toString()
  const payload = await request<{ items: PartnerStatementDto[] }>(
    gatewayBaseUrl,
    `/partner/statements/differences${qs ? `?${qs}` : ''}`,
    { headers: adminHeaders(token) },
  )
  return (payload.items ?? []).map(mapPartnerStatement)
}

export async function partnerConfirmStatement(id: number, token: string): Promise<PartnerStatement> {
  return mapPartnerStatement(
    await request<PartnerStatementDto>(gatewayBaseUrl, `/partner/statements/${id}/confirm`, {
      method: 'POST',
      headers: adminHeaders(token),
    }),
  )
}

export async function partnerDisputeStatement(id: number, reason: string, token: string): Promise<PartnerStatement> {
  return mapPartnerStatement(
    await request<PartnerStatementDto>(gatewayBaseUrl, `/partner/statements/${id}/dispute`, {
      method: 'POST',
      headers: adminHeaders(token),
      body: JSON.stringify({ reason }),
    }),
  )
}

export async function fetchPartnerStatementExport(id: number, token: string): Promise<Blob> {
  const resp = await fetch(`${gatewayBaseUrl}/partner/statements/${id}/export`, {
    headers: adminHeaders(token),
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(text || `导出失败（HTTP ${resp.status}）`)
  }
  return resp.blob()
}

export async function fetchPartnerAnalyticsOverview(
  params: { from?: string; to?: string } | undefined,
  token: string,
): Promise<PartnerAnalyticsOverview> {
  const search = new URLSearchParams()
  if (params?.from) search.set('from', params.from)
  if (params?.to) search.set('to', params.to)
  const qs = search.toString()
  const payload = await request<{
    from: string
    to: string
    partner_id: number
    items: { event_type: string; events: number; users: number; amount_cents: number }[]
  }>(gatewayBaseUrl, `/api/v1/analytics/partner${qs ? `?${qs}` : ''}`, {
    headers: adminHeaders(token),
  })
  return {
    from: payload.from,
    to: payload.to,
    partnerId: payload.partner_id,
    items: (payload.items ?? []).map((r) => ({
      eventType: r.event_type,
      events: r.events,
      users: r.users,
      amountCents: r.amount_cents,
    })),
  }
}
