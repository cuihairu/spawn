// 运营后台与合作方自助（game-catalog 域，经网关反代，GP-103）。
// ========== 运营后台（GP-103，game-catalog 域，经网关反代） ==========
// game-catalog 返回裸 JSON（非信封），错误经 HTTP 状态码 + {code,message}，
// request() 统一在 !response.ok 时抛 message。
import { apiGatewayUrl, gameServiceUrl, request } from './http'
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
